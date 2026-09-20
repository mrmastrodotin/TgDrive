package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
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
func (db *DB) CreateFile(ctx context.Context, name string, size int64, mimeType string, folderID *string, salt string, chunkCount int) (*File, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	_, err := db.conn.ExecContext(ctx,
		`INSERT INTO files (id, name, size, mime_type, folder_id, encryption_salt, chunk_count, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, size, mimeType, folderID, salt, chunkCount, now,
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
		CreatedAt:      now,
	}, nil
}

// GetFile returns a file by ID.
func (db *DB) GetFile(ctx context.Context, id string) (*File, error) {
	var f File
	err := db.conn.QueryRowContext(ctx,
		`SELECT id, name, size, mime_type, folder_id, encryption_salt, chunk_count, created_at
		 FROM files WHERE id = ?`, id,
	).Scan(&f.ID, &f.Name, &f.Size, &f.MimeType, &f.FolderID, &f.EncryptionSalt, &f.ChunkCount, &f.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get file: %w", err)
	}
	return &f, nil
}

// ListFiles lists files in a folder (nil for root).
func (db *DB) ListFiles(ctx context.Context, folderID *string) ([]File, error) {
	var rows *sql.Rows
	var err error

	if folderID == nil {
		rows, err = db.conn.QueryContext(ctx,
			`SELECT id, name, size, mime_type, folder_id, encryption_salt, chunk_count, created_at
			 FROM files WHERE folder_id IS NULL ORDER BY name`,
		)
	} else {
		rows, err = db.conn.QueryContext(ctx,
			`SELECT id, name, size, mime_type, folder_id, encryption_salt, chunk_count, created_at
			 FROM files WHERE folder_id = ? ORDER BY name`,
			*folderID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer rows.Close()

	var files []File
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.ID, &f.Name, &f.Size, &f.MimeType, &f.FolderID, &f.EncryptionSalt, &f.ChunkCount, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		files = append(files, f)
	}
	return files, nil
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

// --- Chunk Operations ---

// CreateChunk stores a chunk record mapping file block to Telegram message.
func (db *DB) CreateChunk(ctx context.Context, fileID string, index int, telegramMsgID int, telegramFileID string, size int, iv, authTag string) (*Chunk, error) {
	id := uuid.New().String()

	_, err := db.conn.ExecContext(ctx,
		`INSERT INTO chunks (id, file_id, chunk_index, telegram_msg_id, telegram_file_id, size, iv, auth_tag)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, fileID, index, telegramMsgID, telegramFileID, size, iv, authTag,
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
		Size:           size,
		IV:             iv,
		AuthTag:        authTag,
	}, nil
}

// GetChunks returns all chunks for a file, ordered by index.
func (db *DB) GetChunks(ctx context.Context, fileID string) ([]Chunk, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, file_id, chunk_index, telegram_msg_id, telegram_file_id, size, iv, auth_tag
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
		if err := rows.Scan(&c.ID, &c.FileID, &c.ChunkIndex, &c.TelegramMsgID, &c.TelegramFileID, &c.Size, &c.IV, &c.AuthTag); err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}
		chunks = append(chunks, c)
	}
	return chunks, nil
}

// GetChunksByRange returns chunks that overlap with the given byte range of the original file.
func (db *DB) GetChunksByRange(ctx context.Context, fileID string, startByte, endByte int64) ([]Chunk, int64, error) {
	// Each raw chunk covers ChunkSize bytes (20MB)
	const rawChunkSize = 20 * 1024 * 1024

	startChunk := int(startByte / rawChunkSize)
	endChunk := int(endByte / rawChunkSize)

	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, file_id, chunk_index, telegram_msg_id, telegram_file_id, size, iv, auth_tag
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
		if err := rows.Scan(&c.ID, &c.FileID, &c.ChunkIndex, &c.TelegramMsgID, &c.TelegramFileID, &c.Size, &c.IV, &c.AuthTag); err != nil {
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
