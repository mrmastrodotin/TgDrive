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

// File represents an uploaded encrypted file.
type File struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Size           int64     `json:"size"`
	MimeType       string    `json:"mime_type"`
	FolderID       *string   `json:"folder_id,omitempty"`
	EncryptionSalt string    `json:"encryption_salt"` // Hex-encoded
	ChunkCount     int       `json:"chunk_count"`
	CreatedAt      time.Time `json:"created_at"`
}

// Chunk represents a single encrypted block stored as a Telegram message.
type Chunk struct {
	ID             string `json:"id"`
	FileID         string `json:"file_id"`
	ChunkIndex     int    `json:"chunk_index"`
	TelegramMsgID  int    `json:"telegram_msg_id"`
	TelegramFileID string `json:"telegram_file_id"`
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
