package storage

import (
	"time"
)

// Folder represents a virtual directory in the drive.
type Folder struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ParentID  *string   `json:"parent_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FileMetadata contains optional rich media information.
type FileMetadata struct {
	Duration                *float64 `json:"duration,omitempty"`
	Width                   *int     `json:"width,omitempty"`
	Height                  *int     `json:"height,omitempty"`
	ThumbnailTelegramFileID *string  `json:"thumbnail_telegram_file_id,omitempty"`
	ThumbnailIV             *string  `json:"thumbnail_iv,omitempty"`
	ThumbnailAuthTag        *string  `json:"thumbnail_auth_tag,omitempty"`
}

// File represents an uploaded encrypted file.
type File struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Size           int64         `json:"size"`
	MimeType       string        `json:"mime_type"`
	FolderID       *string       `json:"folder_id,omitempty"`
	EncryptionSalt string        `json:"encryption_salt"` // Hex-encoded
	ChunkCount     int           `json:"chunk_count"`
	ChunkSize      int           `json:"chunk_size"`      // Chunk size used for this file (0 for legacy 20MB)
	OwnerID        string        `json:"owner_id"`         // Owner/tenant identifier
	IsCinema       bool          `json:"is_cinema"`
	Progress       *float64      `json:"progress,omitempty"` // Between 0.0 and 1.0 or elapsed time
	Metadata       *FileMetadata `json:"metadata,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
}

// Chunk represents a single encrypted block stored as a Telegram message.
type Chunk struct {
	ID             string `json:"id"`
	FileID         string `json:"file_id"`
	ChunkIndex     int    `json:"chunk_index"`
	TelegramMsgID  int    `json:"telegram_msg_id"`
	TelegramFileID string `json:"telegram_file_id"`
	BotTokenHash   string `json:"bot_token_hash"`
	Size           int    `json:"size"` // Encrypted size
	IV             string `json:"iv"`   // Hex-encoded
	AuthTag        string `json:"auth_tag"` // Hex-encoded
}

// FolderContents is the response for listing a folder's children.
type FolderContents struct {
	Folder  *Folder  `json:"folder,omitempty"` // nil for root
	Folders []Folder `json:"folders"`
	Files   []File   `json:"files"`
}

// StorageStats represents the aggregate storage consumption.
type StorageStats struct {
	TotalBytes     int64 `json:"total_bytes"`
	TotalFiles     int   `json:"total_files"`
	VideoBytes     int64 `json:"video_bytes"`
	AudioBytes     int64 `json:"audio_bytes"`
	ImageBytes     int64 `json:"image_bytes"`
	DocumentBytes  int64 `json:"document_bytes"`
	ArchiveBytes   int64 `json:"archive_bytes"`
	OtherBytes     int64 `json:"other_bytes"`
}

// UploadSession represents an in-progress, resumable chunked upload session.
type UploadSession struct {
	ID                      string        `json:"id"`
	FileID                  string        `json:"file_id"`
	Name                    string        `json:"name"`
	Size                    int64         `json:"size"`
	MimeType                string        `json:"mime_type"`
	FolderID                *string       `json:"folder_id,omitempty"`
	EncryptionSalt          string        `json:"encryption_salt"`
	ChunkSize               int           `json:"chunk_size"`
	TotalChunks             int           `json:"total_chunks"`
	Status                  string        `json:"status"` // "uploading", "completed", "cancelled", "failed"
	SourceFingerprint       string        `json:"source_fingerprint"` // Validated source-file content identity
	OwnerID                 string        `json:"owner_id"`          // User/tenant account owning this session
	Duration                *float64      `json:"duration,omitempty"`
	Width                   *int          `json:"width,omitempty"`
	Height                  *int          `json:"height,omitempty"`
	ThumbnailTelegramFileID *string       `json:"thumbnail_telegram_file_id,omitempty"`
	ThumbnailIV             *string       `json:"thumbnail_iv,omitempty"`
	ThumbnailAuthTag        *string       `json:"thumbnail_auth_tag,omitempty"`
	ErrorMessage            *string       `json:"error_message,omitempty"`
	CreatedAt               time.Time     `json:"created_at"`
	UpdatedAt               time.Time     `json:"updated_at"`
}

// UploadSessionChunk represents a single confirmed chunk in an upload session.
type UploadSessionChunk struct {
	ID             string    `json:"id"`
	SessionID      string    `json:"session_id"`
	ChunkIndex     int       `json:"chunk_index"`
	TelegramMsgID  int       `json:"telegram_msg_id"`
	TelegramFileID string    `json:"telegram_file_id"`
	BotTokenHash   string    `json:"bot_token_hash"`
	Size           int       `json:"size"`     // Ciphertext size
	RawSize        int       `json:"raw_size"` // Plaintext size
	IV             string    `json:"iv"`
	AuthTag        string    `json:"auth_tag"`
	SHA256Hash     string    `json:"sha256_hash"` // Plaintext chunk SHA-256 digest
	CreatedAt      time.Time `json:"created_at"`
}

// RegisteredBot represents a user-owned Telegram bot worker record.
type RegisteredBot struct {
	TokenHash string    `json:"token_hash"`
	OwnerID   string    `json:"owner_id"`
	Token     string    `json:"-"` // Never expose token in JSON responses or logs
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"created_at"`
}

// ChunkMigration represents a staging record during different-bot chunk migration.
type ChunkMigration struct {
	ChunkID       string    `json:"chunk_id"`
	SourceBotHash string    `json:"source_bot_hash"`
	TargetBotHash string    `json:"target_bot_hash"`
	TargetFileID  string    `json:"target_file_id"`
	TargetMsgID   int       `json:"target_msg_id"`
	Status        string    `json:"status"` // 'uploaded', 'committed'
	CreatedAt     time.Time `json:"created_at"`
}

