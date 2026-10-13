package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"tgdrive/crypto"
)

// --- Folder Operations ---

// CreateFolder creates a new folder.
func (db *DB) CreateFolder(ctx context.Context, name string, parentID *string) (*Folder, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	_, err := db.conn.ExecContext(ctx,
		`INSERT INTO folders (id, name, parent_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		id, name, parentID, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create folder: %w", err)
	}

	return &Folder{
		ID:        id,
		Name:      name,
		ParentID:  parentID,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// GetFolder returns a folder by ID.
func (db *DB) GetFolder(ctx context.Context, id string) (*Folder, error) {
	var f Folder
	err := db.conn.QueryRowContext(ctx,
		`SELECT id, name, parent_id, created_at, updated_at FROM folders WHERE id = ?`, id,
	).Scan(&f.ID, &f.Name, &f.ParentID, &f.CreatedAt, &f.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get folder: %w", err)
	}
	return &f, nil
}

// ListFolders lists child folders of a parent (nil for root).
func (db *DB) ListFolders(ctx context.Context, parentID *string) ([]Folder, error) {
	var rows *sql.Rows
	var err error

	if parentID == nil {
		rows, err = db.conn.QueryContext(ctx,
			`SELECT id, name, parent_id, created_at, updated_at FROM folders WHERE parent_id IS NULL ORDER BY name`,
		)
	} else {
		rows, err = db.conn.QueryContext(ctx,
			`SELECT id, name, parent_id, created_at, updated_at FROM folders WHERE parent_id = ? ORDER BY name`,
			*parentID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	defer rows.Close()

	var folders []Folder
	for rows.Next() {
		var f Folder
		if err := rows.Scan(&f.ID, &f.Name, &f.ParentID, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan folder: %w", err)
		}
		folders = append(folders, f)
	}
	return folders, nil
}

// DeleteFolder deletes a folder and all contents (cascades via FK).
func (db *DB) DeleteFolder(ctx context.Context, id string) error {
	// Delete all files in this folder first (to clean up chunks via cascade)
	_, err := db.conn.ExecContext(ctx, `DELETE FROM files WHERE folder_id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete folder files: %w", err)
	}

	// Recursively delete subfolders
	subFolders, err := db.ListFolders(ctx, &id)
	if err != nil {
		return fmt.Errorf("list subfolders for delete: %w", err)
	}
	for _, sub := range subFolders {
		if err := db.DeleteFolder(ctx, sub.ID); err != nil {
			return err
		}
	}

	_, err = db.conn.ExecContext(ctx, `DELETE FROM folders WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete folder: %w", err)
	}
	return nil
}

// RenameFolder updates a folder's name.
func (db *DB) RenameFolder(ctx context.Context, id, newName string) error {
	_, err := db.conn.ExecContext(ctx,
		`UPDATE folders SET name = ?, updated_at = ? WHERE id = ?`,
		newName, time.Now().UTC(), id,
	)
	return err
}

// --- File Operations ---

// CreateFile creates a new file record.
func (db *DB) CreateFile(ctx context.Context, name string, size int64, mimeType string, folderID *string, salt string, chunkCount int, optionalParams ...interface{}) (*File, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	chunkSize := 0
	ownerID := "admin"

	for _, p := range optionalParams {
		switch v := p.(type) {
		case int:
			chunkSize = v
		case string:
			if v != "" {
				ownerID = v
			}
		}
	}

	_, err := db.conn.ExecContext(ctx,
		`INSERT INTO files (id, name, size, mime_type, folder_id, encryption_salt, chunk_count, chunk_size, owner_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, size, mimeType, folderID, salt, chunkCount, chunkSize, ownerID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create file: %w", err)
	}

	return &File{
		ID:             id,
		Name:           name,
		Size:           size,
		MimeType:       mimeType,
		FolderID:       folderID,
		EncryptionSalt: salt,
		ChunkCount:     chunkCount,
		ChunkSize:      chunkSize,
		OwnerID:        ownerID,
		CreatedAt:      now,
	}, nil
}

// GetFile returns a file by ID.
func (db *DB) GetFile(ctx context.Context, id string) (*File, error) {
	var f File
	var duration *float64
	var width, height *int
	var tFileID, tIV, tAuthTag *string

	err := db.conn.QueryRowContext(ctx,
		`SELECT f.id, f.name, f.size, f.mime_type, f.folder_id, f.encryption_salt, f.chunk_count, COALESCE(f.chunk_size, 0), COALESCE(f.owner_id, 'admin'), f.is_cinema, f.created_at, p.progress,
				m.duration, m.width, m.height, m.thumbnail_telegram_file_id, m.thumbnail_iv, m.thumbnail_auth_tag
		 FROM files f 
		 LEFT JOIN file_progress p ON f.id = p.file_id 
		 LEFT JOIN file_metadata m ON f.id = m.file_id
		 WHERE f.id = ?`, id,
	).Scan(&f.ID, &f.Name, &f.Size, &f.MimeType, &f.FolderID, &f.EncryptionSalt, &f.ChunkCount, &f.ChunkSize, &f.OwnerID, &f.IsCinema, &f.CreatedAt, &f.Progress,
		&duration, &width, &height, &tFileID, &tIV, &tAuthTag)
	
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get file: %w", err)
	}

	if duration != nil || width != nil || height != nil || tFileID != nil {
		f.Metadata = &FileMetadata{
			Duration:                duration,
			Width:                   width,
			Height:                  height,
			ThumbnailTelegramFileID: tFileID,
			ThumbnailIV:             tIV,
			ThumbnailAuthTag:        tAuthTag,
		}
	}

	return &f, nil
}

// ListFiles lists files in a folder (nil for root).
func (db *DB) ListFiles(ctx context.Context, folderID *string) ([]File, error) {
	var rows *sql.Rows
	var err error

	query := `SELECT f.id, f.name, f.size, f.mime_type, f.folder_id, f.encryption_salt, f.chunk_count, COALESCE(f.chunk_size, 0), COALESCE(f.owner_id, 'admin'), f.is_cinema, f.created_at, p.progress,
				     m.duration, m.width, m.height, m.thumbnail_telegram_file_id, m.thumbnail_iv, m.thumbnail_auth_tag
			  FROM files f 
			  LEFT JOIN file_progress p ON f.id = p.file_id 
			  LEFT JOIN file_metadata m ON f.id = m.file_id `

	if folderID == nil {
		rows, err = db.conn.QueryContext(ctx, query + `WHERE f.folder_id IS NULL ORDER BY f.name`)
	} else {
		rows, err = db.conn.QueryContext(ctx, query + `WHERE f.folder_id = ? ORDER BY f.name`, *folderID)
	}
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer rows.Close()

	var files []File
	for rows.Next() {
		var f File
		var duration *float64
		var width, height *int
		var tFileID, tIV, tAuthTag *string

		if err := rows.Scan(&f.ID, &f.Name, &f.Size, &f.MimeType, &f.FolderID, &f.EncryptionSalt, &f.ChunkCount, &f.ChunkSize, &f.OwnerID, &f.IsCinema, &f.CreatedAt, &f.Progress,
			&duration, &width, &height, &tFileID, &tIV, &tAuthTag); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}

		if duration != nil || width != nil || height != nil || tFileID != nil {
			f.Metadata = &FileMetadata{
				Duration:                duration,
				Width:                   width,
				Height:                  height,
				ThumbnailTelegramFileID: tFileID,
				ThumbnailIV:             tIV,
				ThumbnailAuthTag:        tAuthTag,
			}
		}

		files = append(files, f)
	}
	return files, nil
}

// SetFileCinema updates the is_cinema flag for a file.
func (db *DB) SetFileCinema(ctx context.Context, fileID string, isCinema bool) error {
	val := 0
	if isCinema {
		val = 1
	}
	_, err := db.conn.ExecContext(ctx, `UPDATE files SET is_cinema = ? WHERE id = ?`, val, fileID)
	if err != nil {
		return fmt.Errorf("set file cinema: %w", err)
	}
	return nil
}

// DeleteFile deletes a file and its chunks (cascade).
func (db *DB) DeleteFile(ctx context.Context, id string) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM chunks WHERE file_id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete file chunks: %w", err)
	}
	_, err = db.conn.ExecContext(ctx, `DELETE FROM files WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete file: %w", err)
	}
	return nil
}

// SaveFileProgress updates or inserts the progress for a file.
func (db *DB) SaveFileProgress(ctx context.Context, fileID string, progress float64) error {
	now := time.Now().UTC()
	// Using UPSERT for SQLite
	_, err := db.conn.ExecContext(ctx,
		`INSERT INTO file_progress (file_id, progress, updated_at) 
		 VALUES (?, ?, ?) 
		 ON CONFLICT(file_id) DO UPDATE SET progress = excluded.progress, updated_at = excluded.updated_at`,
		fileID, progress, now,
	)
	if err != nil {
		return fmt.Errorf("save file progress: %w", err)
	}
	return nil
}

// SaveFileMetadata saves optional metadata and thumbnail info for a file.
func (db *DB) SaveFileMetadata(ctx context.Context, fileID string, meta *FileMetadata) error {
	if meta == nil {
		return nil
	}
	_, err := db.conn.ExecContext(ctx,
		`INSERT INTO file_metadata (file_id, duration, width, height, thumbnail_telegram_file_id, thumbnail_iv, thumbnail_auth_tag)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(file_id) DO UPDATE SET
			duration=excluded.duration,
			width=excluded.width,
			height=excluded.height,
			thumbnail_telegram_file_id=excluded.thumbnail_telegram_file_id,
			thumbnail_iv=excluded.thumbnail_iv,
			thumbnail_auth_tag=excluded.thumbnail_auth_tag`,
		fileID, meta.Duration, meta.Width, meta.Height, meta.ThumbnailTelegramFileID, meta.ThumbnailIV, meta.ThumbnailAuthTag,
	)
	if err != nil {
		return fmt.Errorf("save file metadata: %w", err)
	}
	return nil
}

// --- Chunk Operations ---

// CreateChunk stores a chunk record mapping file block to Telegram message.
func (db *DB) CreateChunk(ctx context.Context, fileID string, index int, telegramMsgID int, telegramFileID, botTokenHash string, size int, iv, authTag string) (*Chunk, error) {
	id := uuid.New().String()

	_, err := db.conn.ExecContext(ctx,
		`INSERT INTO chunks (id, file_id, chunk_index, telegram_msg_id, telegram_file_id, bot_token_hash, size, iv, auth_tag)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, fileID, index, telegramMsgID, telegramFileID, botTokenHash, size, iv, authTag,
	)
	if err != nil {
		return nil, fmt.Errorf("create chunk: %w", err)
	}

	return &Chunk{
		ID:             id,
		FileID:         fileID,
		ChunkIndex:     index,
		TelegramMsgID:  telegramMsgID,
		TelegramFileID: telegramFileID,
		BotTokenHash:   botTokenHash,
		Size:           size,
		IV:             iv,
		AuthTag:        authTag,
	}, nil
}

// GetChunks returns all chunks for a file, ordered by index.
func (db *DB) GetChunks(ctx context.Context, fileID string) ([]Chunk, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, file_id, chunk_index, telegram_msg_id, telegram_file_id, bot_token_hash, size, iv, auth_tag
		 FROM chunks WHERE file_id = ? ORDER BY chunk_index`,
		fileID,
	)
	if err != nil {
		return nil, fmt.Errorf("get chunks: %w", err)
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.FileID, &c.ChunkIndex, &c.TelegramMsgID, &c.TelegramFileID, &c.BotTokenHash, &c.Size, &c.IV, &c.AuthTag); err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}
		chunks = append(chunks, c)
	}
	return chunks, nil
}

// GetChunksByRange returns chunks that overlap with the given byte range of the original file.
func (db *DB) GetChunksByRange(ctx context.Context, fileID string, startByte, endByte, rawChunkSize int64) ([]Chunk, int64, error) {
	if rawChunkSize <= 0 {
		rawChunkSize = 20 * 1024 * 1024
	}

	startChunk := int(startByte / rawChunkSize)
	endChunk := int(endByte / rawChunkSize)

	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, file_id, chunk_index, telegram_msg_id, telegram_file_id, bot_token_hash, size, iv, auth_tag
		 FROM chunks WHERE file_id = ? AND chunk_index >= ? AND chunk_index <= ? ORDER BY chunk_index`,
		fileID, startChunk, endChunk,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("get chunks by range: %w", err)
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.FileID, &c.ChunkIndex, &c.TelegramMsgID, &c.TelegramFileID, &c.BotTokenHash, &c.Size, &c.IV, &c.AuthTag); err != nil {
			return nil, 0, fmt.Errorf("scan chunk: %w", err)
		}
		chunks = append(chunks, c)
	}

	// Calculate byte offset within the first chunk
	offsetInFirstChunk := startByte - int64(startChunk)*rawChunkSize

	return chunks, offsetInFirstChunk, nil
}

// GetAllFileChunkTelegramMsgIDs returns all Telegram message IDs for a file's chunks.
func (db *DB) GetAllFileChunkTelegramMsgIDs(ctx context.Context, fileID string) ([]int, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT telegram_msg_id FROM chunks WHERE file_id = ? ORDER BY chunk_index`,
		fileID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// GetStorageStats computes the total usage stats.
func (db *DB) GetStorageStats(ctx context.Context) (StorageStats, error) {
	rows, err := db.conn.QueryContext(ctx, "SELECT size, mime_type FROM files")
	if err != nil {
		return StorageStats{}, err
	}
	defer rows.Close()

	var stats StorageStats
	for rows.Next() {
		var size int64
		var mimeType string
		if err := rows.Scan(&size, &mimeType); err != nil {
			return StorageStats{}, err
		}
		stats.TotalBytes += size
		stats.TotalFiles++

		mimeLower := strings.ToLower(mimeType)
		if strings.HasPrefix(mimeLower, "video/") {
			stats.VideoBytes += size
		} else if strings.HasPrefix(mimeLower, "audio/") {
			stats.AudioBytes += size
		} else if strings.HasPrefix(mimeLower, "image/") {
			stats.ImageBytes += size
		} else if strings.Contains(mimeLower, "pdf") || strings.Contains(mimeLower, "document") || strings.Contains(mimeLower, "text/") {
			stats.DocumentBytes += size
		} else if strings.Contains(mimeLower, "zip") || strings.Contains(mimeLower, "tar") || strings.Contains(mimeLower, "rar") || strings.Contains(mimeLower, "7z") {
			stats.ArchiveBytes += size
		} else {
			stats.OtherBytes += size
		}
	}
	return stats, nil
}

// --- Upload Session Operations ---

// CreateUploadSession creates a new upload session record.
func (db *DB) CreateUploadSession(ctx context.Context, session *UploadSession) error {
	now := time.Now().UTC()
	session.CreatedAt = now
	session.UpdatedAt = now
	if session.Status == "" {
		session.Status = "uploading"
	}
	if session.OwnerID == "" {
		session.OwnerID = "admin"
	}

	_, err := db.conn.ExecContext(ctx,
		`INSERT INTO upload_sessions (
			id, file_id, name, size, mime_type, folder_id, encryption_salt,
			chunk_size, total_chunks, status, source_fingerprint, owner_id, duration, width, height,
			thumbnail_telegram_file_id, thumbnail_iv, thumbnail_auth_tag,
			error_message, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.FileID, session.Name, session.Size, session.MimeType, session.FolderID, session.EncryptionSalt,
		session.ChunkSize, session.TotalChunks, session.Status, session.SourceFingerprint, session.OwnerID, session.Duration, session.Width, session.Height,
		session.ThumbnailTelegramFileID, session.ThumbnailIV, session.ThumbnailAuthTag,
		session.ErrorMessage, session.CreatedAt, session.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create upload session: %w", err)
	}
	return nil
}

// GetUploadSession retrieves an upload session by ID.
func (db *DB) GetUploadSession(ctx context.Context, sessionID string) (*UploadSession, error) {
	var s UploadSession
	err := db.conn.QueryRowContext(ctx,
		`SELECT id, file_id, name, size, mime_type, folder_id, encryption_salt,
		        chunk_size, total_chunks, status, COALESCE(source_fingerprint, ''), COALESCE(owner_id, 'admin'), duration, width, height,
		        thumbnail_telegram_file_id, thumbnail_iv, thumbnail_auth_tag,
		        error_message, created_at, updated_at
		 FROM upload_sessions WHERE id = ?`,
		sessionID,
	).Scan(
		&s.ID, &s.FileID, &s.Name, &s.Size, &s.MimeType, &s.FolderID, &s.EncryptionSalt,
		&s.ChunkSize, &s.TotalChunks, &s.Status, &s.SourceFingerprint, &s.OwnerID, &s.Duration, &s.Width, &s.Height,
		&s.ThumbnailTelegramFileID, &s.ThumbnailIV, &s.ThumbnailAuthTag,
		&s.ErrorMessage, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get upload session: %w", err)
	}
	return &s, nil
}

// UpdateUploadSessionStatus updates the status and optional error message of a session.
func (db *DB) UpdateUploadSessionStatus(ctx context.Context, sessionID, status string, errMsg *string) error {
	now := time.Now().UTC()
	_, err := db.conn.ExecContext(ctx,
		`UPDATE upload_sessions SET status = ?, error_message = ?, updated_at = ? WHERE id = ?`,
		status, errMsg, now, sessionID,
	)
	if err != nil {
		return fmt.Errorf("update upload session status: %w", err)
	}
	return nil
}

// SaveUploadSessionChunk saves or updates a confirmed chunk for an upload session.
func (db *DB) SaveUploadSessionChunk(ctx context.Context, chunk *UploadSessionChunk) error {
	if chunk.ID == "" {
		chunk.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	chunk.CreatedAt = now

	_, err := db.conn.ExecContext(ctx,
		`INSERT INTO upload_session_chunks (
			id, session_id, chunk_index, telegram_msg_id, telegram_file_id,
			bot_token_hash, size, raw_size, iv, auth_tag, sha256_hash, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, chunk_index) DO UPDATE SET
			telegram_msg_id = excluded.telegram_msg_id,
			telegram_file_id = excluded.telegram_file_id,
			bot_token_hash = excluded.bot_token_hash,
			size = excluded.size,
			raw_size = excluded.raw_size,
			iv = excluded.iv,
			auth_tag = excluded.auth_tag,
			sha256_hash = excluded.sha256_hash`,
		chunk.ID, chunk.SessionID, chunk.ChunkIndex, chunk.TelegramMsgID, chunk.TelegramFileID,
		chunk.BotTokenHash, chunk.Size, chunk.RawSize, chunk.IV, chunk.AuthTag, chunk.SHA256Hash, chunk.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("save upload session chunk: %w", err)
	}

	// Update session updated_at
	_, _ = db.conn.ExecContext(ctx, `UPDATE upload_sessions SET updated_at = ? WHERE id = ?`, now, chunk.SessionID)
	return nil
}

// GetUploadSessionChunks retrieves all chunks uploaded for a session, ordered by chunk_index.
func (db *DB) GetUploadSessionChunks(ctx context.Context, sessionID string) ([]UploadSessionChunk, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, session_id, chunk_index, telegram_msg_id, telegram_file_id,
		        bot_token_hash, size, raw_size, iv, auth_tag, COALESCE(sha256_hash, ''), created_at
		 FROM upload_session_chunks WHERE session_id = ? ORDER BY chunk_index ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("get upload session chunks: %w", err)
	}
	defer rows.Close()

	var chunks []UploadSessionChunk
	for rows.Next() {
		var c UploadSessionChunk
		if err := rows.Scan(
			&c.ID, &c.SessionID, &c.ChunkIndex, &c.TelegramMsgID, &c.TelegramFileID,
			&c.BotTokenHash, &c.Size, &c.RawSize, &c.IV, &c.AuthTag, &c.SHA256Hash, &c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan upload session chunk: %w", err)
		}
		chunks = append(chunks, c)
	}
	return chunks, nil
}

// GetUploadSessionChunk retrieves a single chunk by session and chunk_index.
func (db *DB) GetUploadSessionChunk(ctx context.Context, sessionID string, chunkIndex int) (*UploadSessionChunk, error) {
	var c UploadSessionChunk
	err := db.conn.QueryRowContext(ctx,
		`SELECT id, session_id, chunk_index, telegram_msg_id, telegram_file_id,
		        bot_token_hash, size, raw_size, iv, auth_tag, COALESCE(sha256_hash, ''), created_at
		 FROM upload_session_chunks WHERE session_id = ? AND chunk_index = ?`,
		sessionID, chunkIndex,
	).Scan(
		&c.ID, &c.SessionID, &c.ChunkIndex, &c.TelegramMsgID, &c.TelegramFileID,
		&c.BotTokenHash, &c.Size, &c.RawSize, &c.IV, &c.AuthTag, &c.SHA256Hash, &c.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get upload session chunk: %w", err)
	}
	return &c, nil
}

// GetConfirmedChunkIndices returns the sorted list of uploaded chunk indices for a session.
func (db *DB) GetConfirmedChunkIndices(ctx context.Context, sessionID string) ([]int, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT chunk_index FROM upload_session_chunks WHERE session_id = ? ORDER BY chunk_index ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("get confirmed chunk indices: %w", err)
	}
	defer rows.Close()

	var indices []int
	for rows.Next() {
		var idx int
		if err := rows.Scan(&idx); err != nil {
			return nil, err
		}
		indices = append(indices, idx)
	}
	return indices, nil
}

// GetConfirmedChunkHashes returns a map from chunk_index to sha256_hash for an upload session.
func (db *DB) GetConfirmedChunkHashes(ctx context.Context, sessionID string) (map[int]string, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT chunk_index, COALESCE(sha256_hash, '') FROM upload_session_chunks WHERE session_id = ?`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("get confirmed chunk hashes: %w", err)
	}
	defer rows.Close()

	hashes := make(map[int]string)
	for rows.Next() {
		var idx int
		var h string
		if err := rows.Scan(&idx, &h); err != nil {
			return nil, err
		}
		hashes[idx] = h
	}
	return hashes, nil
}

// FinalizeUploadSession atomically converts a completed upload session into a durable File and Chunks.
// It is fully idempotent: if already finalized, it returns the existing file.
func (db *DB) FinalizeUploadSession(ctx context.Context, sessionID string) (*File, error) {
	for attempt := 0; attempt < 10; attempt++ {
		file, err := db.finalizeUploadSessionInternal(ctx, sessionID)
		if err != nil && (strings.Contains(err.Error(), "locked") || strings.Contains(err.Error(), "busy") || strings.Contains(err.Error(), "SQLITE_BUSY")) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(15+attempt*25) * time.Millisecond):
				continue
			}
		}
		return file, err
	}
	return db.finalizeUploadSessionInternal(ctx, sessionID)
}

func (db *DB) finalizeUploadSessionInternal(ctx context.Context, sessionID string) (*File, error) {
	session, err := db.GetUploadSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if session == nil {
		return nil, fmt.Errorf("upload session not found")
	}

	// Idempotency: if already completed, return existing file
	if session.Status == "completed" {
		existingFile, err := db.GetFile(ctx, session.FileID)
		if err != nil {
			return nil, fmt.Errorf("get existing file: %w", err)
		}
		if existingFile != nil {
			return existingFile, nil
		}
	}

	if session.Status != "uploading" {
		return nil, fmt.Errorf("cannot finalize session with status %q", session.Status)
	}

	// Fetch all chunks
	chunks, err := db.GetUploadSessionChunks(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get chunks: %w", err)
	}

	if len(chunks) != session.TotalChunks {
		return nil, fmt.Errorf("incomplete upload: confirmed %d of %d expected chunks", len(chunks), session.TotalChunks)
	}

	// Verify chunk indices and calculate total reconstructed plaintext size
	var totalPlaintext int64
	for expectedIdx, c := range chunks {
		if c.ChunkIndex != expectedIdx {
			return nil, fmt.Errorf("missing chunk index %d (got %d)", expectedIdx, c.ChunkIndex)
		}
		totalPlaintext += int64(c.RawSize)
	}

	if totalPlaintext != session.Size {
		return nil, fmt.Errorf("reconstructed size mismatch: chunks total %d bytes, expected %d bytes", totalPlaintext, session.Size)
	}

	// Transaction to insert file and chunks
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()

	ownerID := session.OwnerID
	if ownerID == "" {
		ownerID = "admin"
	}

	// 1. Insert into files table
	_, err = tx.ExecContext(ctx,
		`INSERT INTO files (id, name, size, mime_type, folder_id, encryption_salt, chunk_count, chunk_size, owner_id, is_cinema, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   name = excluded.name,
		   size = excluded.size,
		   mime_type = excluded.mime_type,
		   folder_id = excluded.folder_id,
		   encryption_salt = excluded.encryption_salt,
		   chunk_count = excluded.chunk_count,
		   chunk_size = excluded.chunk_size,
		   owner_id = excluded.owner_id`,
		session.FileID, session.Name, session.Size, session.MimeType, session.FolderID, session.EncryptionSalt, session.TotalChunks, session.ChunkSize, ownerID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert file: %w", err)
	}

	// 2. Insert into chunks table
	for _, c := range chunks {
		chunkID := uuid.New().String()
		_, err = tx.ExecContext(ctx,
			`INSERT INTO chunks (id, file_id, chunk_index, telegram_msg_id, telegram_file_id, bot_token_hash, size, iv, auth_tag)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(file_id, chunk_index) DO UPDATE SET
			   telegram_msg_id = excluded.telegram_msg_id,
			   telegram_file_id = excluded.telegram_file_id,
			   bot_token_hash = excluded.bot_token_hash,
			   size = excluded.size,
			   iv = excluded.iv,
			   auth_tag = excluded.auth_tag`,
			chunkID, session.FileID, c.ChunkIndex, c.TelegramMsgID, c.TelegramFileID, c.BotTokenHash, c.Size, c.IV, c.AuthTag,
		)
		if err != nil {
			return nil, fmt.Errorf("insert chunk %d: %w", c.ChunkIndex, err)
		}
	}

	// 3. Insert metadata if exists
	if session.Duration != nil || session.Width != nil || session.Height != nil || session.ThumbnailTelegramFileID != nil {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO file_metadata (file_id, duration, width, height, thumbnail_telegram_file_id, thumbnail_iv, thumbnail_auth_tag)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(file_id) DO UPDATE SET
			   duration = excluded.duration,
			   width = excluded.width,
			   height = excluded.height,
			   thumbnail_telegram_file_id = excluded.thumbnail_telegram_file_id,
			   thumbnail_iv = excluded.thumbnail_iv,
			   thumbnail_auth_tag = excluded.thumbnail_auth_tag`,
			session.FileID, session.Duration, session.Width, session.Height,
			session.ThumbnailTelegramFileID, session.ThumbnailIV, session.ThumbnailAuthTag,
		)
		if err != nil {
			return nil, fmt.Errorf("insert file metadata: %w", err)
		}
	}

	// 4. Mark session completed
	_, err = tx.ExecContext(ctx,
		`UPDATE upload_sessions SET status = 'completed', updated_at = ? WHERE id = ?`,
		now, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("update session completed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return db.GetFile(ctx, session.FileID)
}

// CancelUploadSession marks a session cancelled and returns the list of Telegram message IDs to clean up.
func (db *DB) CancelUploadSession(ctx context.Context, sessionID string) ([]int, error) {
	now := time.Now().UTC()
	_, err := db.conn.ExecContext(ctx,
		`UPDATE upload_sessions SET status = 'cancelled', updated_at = ? WHERE id = ?`,
		now, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("cancel upload session: %w", err)
	}

	rows, err := db.conn.QueryContext(ctx,
		`SELECT telegram_msg_id FROM upload_session_chunks WHERE session_id = ?`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("get session chunk msg ids: %w", err)
	}
	defer rows.Close()

	var msgIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			msgIDs = append(msgIDs, id)
		}
	}
	return msgIDs, nil
}

// CountChunksForBot returns how many stored file chunks depend on the given bot token hash.
func (db *DB) CountChunksForBot(ctx context.Context, tokenHash string) (int, error) {
	var count int
	err := db.conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM chunks WHERE bot_token_hash = ?`,
		tokenHash,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count chunks for bot: %w", err)
	}
	return count, nil
}

// --- Registered Bot Operations ---

func (db *DB) decryptBotToken(token string) string {
	if db.credKey != nil && strings.HasPrefix(token, "enc:v1:") {
		decrypted, err := crypto.DecryptCredential(db.credKey, token)
		if err == nil {
			return decrypted
		}
		// Decryption failed (e.g. wrong key, corrupted ciphertext, or unrotated secret).
		// Fail safely by returning empty string instead of returning raw ciphertext.
		return ""
	}
	return token
}

// SaveBot creates or updates a registered bot worker with authenticated encryption at rest and immutable ownership.
func (db *DB) SaveBot(ctx context.Context, b *RegisteredBot) error {
	storedToken := b.Token
	if db.credKey != nil && storedToken != "" && !strings.HasPrefix(storedToken, "enc:v1:") {
		encrypted, err := crypto.EncryptCredential(db.credKey, storedToken)
		if err != nil {
			return fmt.Errorf("encrypt bot token: %w", err)
		}
		storedToken = encrypted
	}

	var existingOwner string
	err := db.conn.QueryRowContext(ctx,
		`SELECT owner_id FROM registered_bots WHERE token_hash = ?`,
		b.TokenHash,
	).Scan(&existingOwner)

	if err == nil {
		if existingOwner != b.OwnerID {
			return fmt.Errorf("bot already registered to another owner (%s)", existingOwner)
		}
		// Same owner: update disabled state
		disabledInt := 0
		if b.Disabled {
			disabledInt = 1
		}
		_, err = db.conn.ExecContext(ctx,
			`UPDATE registered_bots SET disabled = ?, token = ? WHERE token_hash = ?`,
			disabledInt, storedToken, b.TokenHash,
		)
		return err
	} else if err != sql.ErrNoRows {
		return fmt.Errorf("check bot existence: %w", err)
	}

	disabledInt := 0
	if b.Disabled {
		disabledInt = 1
	}
	now := time.Now().UTC()
	_, err = db.conn.ExecContext(ctx,
		`INSERT INTO registered_bots (token_hash, owner_id, token, disabled, created_at) VALUES (?, ?, ?, ?, ?)`,
		b.TokenHash, b.OwnerID, storedToken, disabledInt, now,
	)
	if err != nil {
		return fmt.Errorf("insert registered bot: %w", err)
	}
	return nil
}

// GetBotsByOwner returns all registered bots belonging to the specified owner, with tokens decrypted in memory.
func (db *DB) GetBotsByOwner(ctx context.Context, ownerID string) ([]RegisteredBot, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT token_hash, owner_id, token, disabled, created_at FROM registered_bots WHERE owner_id = ? ORDER BY created_at`,
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("get bots by owner: %w", err)
	}
	defer rows.Close()

	var bots []RegisteredBot
	for rows.Next() {
		var b RegisteredBot
		var disInt int
		if err := rows.Scan(&b.TokenHash, &b.OwnerID, &b.Token, &disInt, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan registered bot: %w", err)
		}
		b.Disabled = disInt == 1
		b.Token = db.decryptBotToken(b.Token)
		bots = append(bots, b)
	}
	return bots, nil
}

// GetAllBots returns all registered bots across all owners, with tokens decrypted in memory.
func (db *DB) GetAllBots(ctx context.Context) ([]RegisteredBot, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT token_hash, owner_id, token, disabled, created_at FROM registered_bots ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("get all bots: %w", err)
	}
	defer rows.Close()

	var bots []RegisteredBot
	for rows.Next() {
		var b RegisteredBot
		var disInt int
		if err := rows.Scan(&b.TokenHash, &b.OwnerID, &b.Token, &disInt, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan registered bot: %w", err)
		}
		b.Disabled = disInt == 1
		b.Token = db.decryptBotToken(b.Token)
		bots = append(bots, b)
	}
	return bots, nil
}

// GetBot returns a registered bot by its token hash, with token decrypted in memory.
func (db *DB) GetBot(ctx context.Context, tokenHash string) (*RegisteredBot, error) {
	var b RegisteredBot
	var disInt int
	err := db.conn.QueryRowContext(ctx,
		`SELECT token_hash, owner_id, token, disabled, created_at FROM registered_bots WHERE token_hash = ?`,
		tokenHash,
	).Scan(&b.TokenHash, &b.OwnerID, &b.Token, &disInt, &b.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get bot by hash: %w", err)
	}
	b.Disabled = disInt == 1
	b.Token = db.decryptBotToken(b.Token)
	return &b, nil
}

// MigrateBotCredentials upgrades all legacy unencrypted plaintext bot tokens in registered_bots
// to authenticated AES-256-GCM ciphertext at rest using the provided or configured key.
func (db *DB) MigrateBotCredentials(ctx context.Context, key []byte) error {
	if key == nil {
		key = db.credKey
	}
	if key == nil {
		return fmt.Errorf("no credential key available for migration")
	}

	rows, err := db.conn.QueryContext(ctx, `SELECT token_hash, token FROM registered_bots`)
	if err != nil {
		return fmt.Errorf("query bots for migration: %w", err)
	}
	defer rows.Close()

	type botUpdate struct {
		hash      string
		encrypted string
	}
	var updates []botUpdate

	for rows.Next() {
		var hash, token string
		if err := rows.Scan(&hash, &token); err != nil {
			return fmt.Errorf("scan bot row: %w", err)
		}
		if !strings.HasPrefix(token, "enc:v1:") && token != "" {
			enc, err := crypto.EncryptCredential(key, token)
			if err != nil {
				return fmt.Errorf("encrypt bot token (%s): %w", hash, err)
			}
			updates = append(updates, botUpdate{hash: hash, encrypted: enc})
		}
	}
	rows.Close()

	for _, u := range updates {
		_, err := db.conn.ExecContext(ctx,
			`UPDATE registered_bots SET token = ? WHERE token_hash = ?`,
			u.encrypted, u.hash,
		)
		if err != nil {
			return fmt.Errorf("update encrypted bot token (%s): %w", u.hash, err)
		}
	}
	return nil
}

// RotateBotCredentials re-encrypts all stored bot credentials from an old key to a new key.
func (db *DB) RotateBotCredentials(ctx context.Context, oldKey, newKey []byte) error {
	if oldKey == nil || newKey == nil {
		return fmt.Errorf("both oldKey and newKey are required for rotation")
	}

	rows, err := db.conn.QueryContext(ctx, `SELECT token_hash, token FROM registered_bots`)
	if err != nil {
		return fmt.Errorf("query bots for rotation: %w", err)
	}
	defer rows.Close()

	type botUpdate struct {
		hash      string
		encrypted string
	}
	var updates []botUpdate

	for rows.Next() {
		var hash, token string
		if err := rows.Scan(&hash, &token); err != nil {
			return fmt.Errorf("scan bot row: %w", err)
		}
		rawToken, err := crypto.DecryptCredential(oldKey, token)
		if err != nil {
			return fmt.Errorf("decrypt bot token with old key (%s): %w", hash, err)
		}
		newEnc, err := crypto.EncryptCredential(newKey, rawToken)
		if err != nil {
			return fmt.Errorf("encrypt bot token with new key (%s): %w", hash, err)
		}
		updates = append(updates, botUpdate{hash: hash, encrypted: newEnc})
	}
	rows.Close()

	for _, u := range updates {
		_, err := db.conn.ExecContext(ctx,
			`UPDATE registered_bots SET token = ? WHERE token_hash = ?`,
			u.encrypted, u.hash,
		)
		if err != nil {
			return fmt.Errorf("update rotated bot token (%s): %w", u.hash, err)
		}
	}
	db.SetCredentialKey(newKey)
	return nil
}

// DeleteBot removes a registered bot, strictly verifying ownership.
func (db *DB) DeleteBot(ctx context.Context, ownerID, tokenHash string) error {
	existing, err := db.GetBot(ctx, tokenHash)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("bot with hash %s not found", tokenHash)
	}
	if existing.OwnerID != ownerID && ownerID != "admin" {
		return fmt.Errorf("unauthorized: bot belongs to owner %s, not %s", existing.OwnerID, ownerID)
	}

	_, err = db.conn.ExecContext(ctx,
		`DELETE FROM registered_bots WHERE token_hash = ?`,
		tokenHash,
	)
	return err
}

// SetBotDisabled updates the disabled flag for a bot, strictly verifying ownership.
func (db *DB) SetBotDisabled(ctx context.Context, ownerID, tokenHash string, disabled bool) error {
	existing, err := db.GetBot(ctx, tokenHash)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("bot with hash %s not found", tokenHash)
	}
	if existing.OwnerID != ownerID && ownerID != "admin" {
		return fmt.Errorf("unauthorized: bot belongs to owner %s, not %s", existing.OwnerID, ownerID)
	}

	disInt := 0
	if disabled {
		disInt = 1
	}
	_, err = db.conn.ExecContext(ctx,
		`UPDATE registered_bots SET disabled = ? WHERE token_hash = ?`,
		disInt, tokenHash,
	)
	return err
}

// UpdateChunkBotTokenHash updates all chunks and upload_session_chunks mapped to oldHash to newHash.
// Used when rotating credentials for the same bot identity.
func (db *DB) UpdateChunkBotTokenHash(ctx context.Context, oldHash, newHash string) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for bot hash update: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE chunks SET bot_token_hash = ? WHERE bot_token_hash = ?`, newHash, oldHash); err != nil {
		return fmt.Errorf("update chunks bot hash: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE upload_session_chunks SET bot_token_hash = ? WHERE bot_token_hash = ?`, newHash, oldHash); err != nil {
		return fmt.Errorf("update session chunks bot hash: %w", err)
	}

	return tx.Commit()
}

// GetChunksByBotTokenHash returns all chunks currently referencing the given bot token hash.
func (db *DB) GetChunksByBotTokenHash(ctx context.Context, botHash string) ([]Chunk, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, file_id, chunk_index, telegram_msg_id, telegram_file_id, bot_token_hash, size, iv, auth_tag
		 FROM chunks WHERE bot_token_hash = ? ORDER BY file_id, chunk_index`,
		botHash,
	)
	if err != nil {
		return nil, fmt.Errorf("get chunks by bot hash: %w", err)
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.FileID, &c.ChunkIndex, &c.TelegramMsgID, &c.TelegramFileID, &c.BotTokenHash, &c.Size, &c.IV, &c.AuthTag); err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}
		chunks = append(chunks, c)
	}
	return chunks, nil
}

// UpdateChunkTelegramRef updates the Telegram reference and bot token hash for an existing chunk.
// Used during chunk migration when moving ciphertext to a replacement bot identity.
func (db *DB) UpdateChunkTelegramRef(ctx context.Context, chunkID string, newFileID string, newMsgID int, newBotHash string) error {
	_, err := db.conn.ExecContext(ctx,
		`UPDATE chunks SET telegram_file_id = ?, telegram_msg_id = ?, bot_token_hash = ? WHERE id = ?`,
		newFileID, newMsgID, newBotHash, chunkID,
	)
	if err != nil {
		return fmt.Errorf("update chunk telegram ref: %w", err)
	}
	return nil
}

// RecordChunkMigration stages or updates an in-flight destination chunk upload.
func (db *DB) RecordChunkMigration(ctx context.Context, chunkID, sourceHash, targetHash, targetFileID string, targetMsgID int) error {
	now := time.Now().UTC()
	_, err := db.conn.ExecContext(ctx,
		`INSERT INTO chunk_migrations (chunk_id, source_bot_hash, target_bot_hash, target_file_id, target_msg_id, status, created_at)
		 VALUES (?, ?, ?, ?, ?, 'uploaded', ?)
		 ON CONFLICT(chunk_id) DO UPDATE SET
		   target_bot_hash = excluded.target_bot_hash,
		   target_file_id  = excluded.target_file_id,
		   target_msg_id   = excluded.target_msg_id,
		   status          = 'uploaded',
		   created_at      = excluded.created_at`,
		chunkID, sourceHash, targetHash, targetFileID, targetMsgID, now,
	)
	if err != nil {
		return fmt.Errorf("record chunk migration: %w", err)
	}
	return nil
}

// GetPendingChunkMigration retrieves an uncommitted migration staging record for a chunk.
func (db *DB) GetPendingChunkMigration(ctx context.Context, chunkID string) (*ChunkMigration, error) {
	var m ChunkMigration
	err := db.conn.QueryRowContext(ctx,
		`SELECT chunk_id, source_bot_hash, target_bot_hash, target_file_id, target_msg_id, status, created_at
		 FROM chunk_migrations WHERE chunk_id = ?`,
		chunkID,
	).Scan(&m.ChunkID, &m.SourceBotHash, &m.TargetBotHash, &m.TargetFileID, &m.TargetMsgID, &m.Status, &m.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get pending chunk migration: %w", err)
	}
	return &m, nil
}

// DeleteChunkMigration removes a staged migration record after confirmed DB commit.
func (db *DB) DeleteChunkMigration(ctx context.Context, chunkID string) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM chunk_migrations WHERE chunk_id = ?`, chunkID)
	if err != nil {
		return fmt.Errorf("delete chunk migration: %w", err)
	}
	return nil
}



