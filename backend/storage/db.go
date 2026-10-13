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
	conn    *sql.DB
	credKey []byte
}

// SetCredentialKey configures the key used to encrypt and decrypt bot credentials at rest.
func (db *DB) SetCredentialKey(key []byte) {
	db.credKey = key
}

// GetCredentialKey returns the configured credential encryption key.
func (db *DB) GetCredentialKey() []byte {
	return db.credKey
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

	if driverName == "sqlite" {
		conn.SetMaxOpenConns(1)
		_, _ = conn.Exec("PRAGMA journal_mode = WAL;")
		_, _ = conn.Exec("PRAGMA busy_timeout = 5000;")
		_, _ = conn.Exec("PRAGMA synchronous = NORMAL;")
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
			chunk_size      INTEGER NOT NULL DEFAULT 0,
			owner_id        TEXT NOT NULL DEFAULT 'admin',
			is_cinema       INTEGER NOT NULL DEFAULT 0,
			created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS chunks (
			id               TEXT PRIMARY KEY,
			file_id          TEXT NOT NULL,
			chunk_index      INTEGER NOT NULL,
			telegram_msg_id  INTEGER NOT NULL,
			telegram_file_id TEXT NOT NULL,
			bot_token_hash   TEXT NOT NULL,
			size             INTEGER NOT NULL,
			iv               TEXT NOT NULL,
			auth_tag         TEXT NOT NULL,
			FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE,
			UNIQUE(file_id, chunk_index)
		)`,
		`CREATE TABLE IF NOT EXISTS file_progress (
			file_id    TEXT PRIMARY KEY,
			progress   REAL NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS file_metadata (
			file_id                    TEXT PRIMARY KEY,
			duration                   REAL,
			width                      INTEGER,
			height                     INTEGER,
			thumbnail_telegram_file_id TEXT,
			thumbnail_iv               TEXT,
			thumbnail_auth_tag         TEXT,
			FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_files_folder ON files(folder_id)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_file ON chunks(file_id)`,
		`CREATE INDEX IF NOT EXISTS idx_folders_parent ON folders(parent_id)`,
		`CREATE TABLE IF NOT EXISTS upload_sessions (
			id                          TEXT PRIMARY KEY,
			file_id                     TEXT NOT NULL,
			name                        TEXT NOT NULL,
			size                        INTEGER NOT NULL,
			mime_type                   TEXT DEFAULT '',
			folder_id                   TEXT,
			encryption_salt             TEXT NOT NULL,
			chunk_size                  INTEGER NOT NULL,
			total_chunks                INTEGER NOT NULL,
			status                      TEXT NOT NULL DEFAULT 'uploading',
			source_fingerprint          TEXT NOT NULL DEFAULT '',
			owner_id                    TEXT NOT NULL DEFAULT 'admin',
			duration                    REAL,
			width                       INTEGER,
			height                      INTEGER,
			thumbnail_telegram_file_id  TEXT,
			thumbnail_iv                TEXT,
			thumbnail_auth_tag          TEXT,
			error_message               TEXT,
			created_at                  DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at                  DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS upload_session_chunks (
			id               TEXT PRIMARY KEY,
			session_id       TEXT NOT NULL,
			chunk_index      INTEGER NOT NULL,
			telegram_msg_id  INTEGER NOT NULL,
			telegram_file_id TEXT NOT NULL,
			bot_token_hash   TEXT NOT NULL,
			size             INTEGER NOT NULL,
			raw_size         INTEGER NOT NULL,
			iv               TEXT NOT NULL,
			auth_tag         TEXT NOT NULL,
			sha256_hash      TEXT NOT NULL DEFAULT '',
			created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES upload_sessions(id) ON DELETE CASCADE,
			UNIQUE(session_id, chunk_index)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_session_chunks ON upload_session_chunks(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_upload_sessions_status ON upload_sessions(status)`,
		`CREATE TABLE IF NOT EXISTS registered_bots (
			token_hash TEXT PRIMARY KEY,
			owner_id   TEXT NOT NULL,
			token      TEXT NOT NULL,
			disabled   INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_registered_bots_owner ON registered_bots(owner_id)`,
		`CREATE TABLE IF NOT EXISTS chunk_migrations (
			chunk_id        TEXT PRIMARY KEY,
			source_bot_hash TEXT NOT NULL,
			target_bot_hash TEXT NOT NULL,
			target_file_id  TEXT NOT NULL,
			target_msg_id   INTEGER NOT NULL,
			status          TEXT NOT NULL DEFAULT 'uploaded',
			created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_chunk_migrations_target ON chunk_migrations(target_bot_hash)`,
	}

	for _, q := range queries {
		if _, err := db.conn.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	// Schema migrations for existing databases
	columnMigrations := []string{
		`ALTER TABLE chunks ADD COLUMN bot_token_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE files ADD COLUMN is_cinema INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE files ADD COLUMN chunk_size INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE upload_sessions ADD COLUMN source_fingerprint TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE upload_sessions ADD COLUMN owner_id TEXT NOT NULL DEFAULT 'admin'`,
		`ALTER TABLE files ADD COLUMN owner_id TEXT NOT NULL DEFAULT 'admin'`,
		`ALTER TABLE upload_session_chunks ADD COLUMN sha256_hash TEXT NOT NULL DEFAULT ''`,
		`UPDATE files SET chunk_size = 20971520 WHERE chunk_size <= 0`,
	}
	for _, q := range columnMigrations {
		_, _ = db.conn.ExecContext(ctx, q)
	}

	return nil
}

// Exec executes a query against the database connection.
func (db *DB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return db.conn.ExecContext(ctx, query, args...)
}
