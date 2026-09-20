package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
	_ "modernc.org/sqlite"
)

// DB wraps the SQL database connection.
type DB struct {
	conn *sql.DB
}

// New creates a new database connection.
// If databaseURL starts with "libsql://" or "https://", it connects using libsql/Turso.
// Otherwise, it uses pure-Go modernc.org/sqlite for local or embedded file storage.
func New(databaseURL, authToken string) (*DB, error) {
	var driverName string
	var dsn string

	if strings.HasPrefix(databaseURL, "libsql://") || strings.HasPrefix(databaseURL, "https://") {
		driverName = "libsql"
		dsn = databaseURL
		if authToken != "" {
			dsn = fmt.Sprintf("%s?authToken=%s", databaseURL, authToken)
		}
	} else {
		driverName = "sqlite"
		dsn = strings.TrimPrefix(databaseURL, "file:")
	}

	conn, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &DB{conn: conn}, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Migrate creates tables if they don't exist.
func (db *DB) Migrate(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS folders (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			parent_id  TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (parent_id) REFERENCES folders(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS files (
			id              TEXT PRIMARY KEY,
			name            TEXT NOT NULL,
			size            INTEGER NOT NULL,
			mime_type       TEXT DEFAULT '',
			folder_id       TEXT,
			encryption_salt TEXT NOT NULL,
			chunk_count     INTEGER NOT NULL,
			created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS chunks (
			id               TEXT PRIMARY KEY,
			file_id          TEXT NOT NULL,
			chunk_index      INTEGER NOT NULL,
			telegram_msg_id  INTEGER NOT NULL,
			telegram_file_id TEXT NOT NULL,
			size             INTEGER NOT NULL,
			iv               TEXT NOT NULL,
			auth_tag         TEXT NOT NULL,
			FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE,
			UNIQUE(file_id, chunk_index)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_files_folder ON files(folder_id)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_file ON chunks(file_id)`,
		`CREATE INDEX IF NOT EXISTS idx_folders_parent ON folders(parent_id)`,
	}

	for _, q := range queries {
		if _, err := db.conn.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	return nil
}
