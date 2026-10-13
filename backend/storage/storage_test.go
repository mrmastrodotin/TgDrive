package storage

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"tgdrive/crypto"
)

func TestStorageWorkflow(t *testing.T) {
	ctx := context.Background()
	testDbFile := "test_tgdrive.db"
	defer os.Remove(testDbFile)

	db, err := New("file:"+testDbFile, "")
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Create Folder
	folder, err := db.CreateFolder(ctx, "Documents", nil)
	if err != nil {
		t.Fatalf("CreateFolder failed: %v", err)
	}
	if folder.Name != "Documents" {
		t.Fatalf("expected folder name 'Documents', got %s", folder.Name)
	}

	// Create File
	file, err := db.CreateFile(ctx, "test.txt", 1024, "text/plain", &folder.ID, "salt123", 1)
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	if file.Name != "test.txt" {
		t.Fatalf("expected file name 'test.txt', got %s", file.Name)
	}

	// Add Chunk
	chunk, err := db.CreateChunk(ctx, file.ID, 0, 1001, "tg_file_001", "tokenhash", 1024, "iv_hex", "tag_hex")
	if err != nil {
		t.Fatalf("CreateChunk failed: %v", err)
	}
	if chunk.TelegramFileID != "tg_file_001" {
		t.Fatalf("unexpected telegram file id %s", chunk.TelegramFileID)
	}

	// Test Cinema toggle
	if err := db.SetFileCinema(ctx, file.ID, true); err != nil {
		t.Fatalf("SetFileCinema failed: %v", err)
	}

	// List Files and Folders
	files, err := db.ListFiles(ctx, &folder.ID)
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if !files[0].IsCinema {
		t.Fatalf("expected file IsCinema to be true")
	}

	folders, err := db.ListFolders(ctx, nil)
	if err != nil {
		t.Fatalf("ListFolders failed: %v", err)
	}
	if len(folders) != 1 {
		t.Fatalf("expected 1 folder, got %d", len(folders))
	}

	// Get File and Chunks
	fetchedFile, err := db.GetFile(ctx, file.ID)
	if err != nil {
		t.Fatalf("GetFile failed: %v", err)
	}
	if fetchedFile.ID != file.ID {
		t.Fatalf("file ID mismatch")
	}

	chunks, err := db.GetChunks(ctx, file.ID)
	if err != nil {
		t.Fatalf("GetChunks failed: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}

	// Delete File
	if err := db.DeleteFile(ctx, file.ID); err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}

	// Delete Folder
	if err := db.DeleteFolder(ctx, folder.ID); err != nil {
		t.Fatalf("DeleteFolder failed: %v", err)
	}
}

func TestUploadSessionWorkflow(t *testing.T) {
	ctx := context.Background()
	testDbFile := "test_upload_session.db"
	defer os.Remove(testDbFile)

	db, err := New("file:"+testDbFile, "")
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	sessionID := "sess-123"
	fileID := "file-456"
	totalSize := int64(30 * 1024 * 1024) // 30 MB
	chunkSize := 20 * 1024 * 1024        // 20 MB -> 2 chunks (20MB and 10MB)

	session := &UploadSession{
		ID:             sessionID,
		FileID:         fileID,
		Name:           "large_movie.mp4",
		Size:           totalSize,
		MimeType:       "video/mp4",
		EncryptionSalt: "salt_abc",
		ChunkSize:      chunkSize,
		TotalChunks:    2,
		Status:         "uploading",
	}

	if err := db.CreateUploadSession(ctx, session); err != nil {
		t.Fatalf("CreateUploadSession failed: %v", err)
	}

	// Fetch session
	fetched, err := db.GetUploadSession(ctx, sessionID)
	if err != nil || fetched == nil {
		t.Fatalf("GetUploadSession failed: %v", err)
	}
	if fetched.TotalChunks != 2 {
		t.Fatalf("expected 2 total chunks, got %d", fetched.TotalChunks)
	}

	// Save chunk 0 (20MB)
	chunk0 := &UploadSessionChunk{
		SessionID:      sessionID,
		ChunkIndex:     0,
		TelegramMsgID:  1001,
		TelegramFileID: "tg-chunk-0",
		BotTokenHash:   "bot-hash-1",
		Size:           20 * 1024 * 1024,
		RawSize:        20 * 1024 * 1024,
		IV:             "iv0",
		AuthTag:        "tag0",
	}
	if err := db.SaveUploadSessionChunk(ctx, chunk0); err != nil {
		t.Fatalf("SaveUploadSessionChunk 0 failed: %v", err)
	}

	// Verify chunk indices
	indices, err := db.GetConfirmedChunkIndices(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetConfirmedChunkIndices failed: %v", err)
	}
	if len(indices) != 1 || indices[0] != 0 {
		t.Fatalf("expected [0], got %v", indices)
	}

	// Finalizing should fail because chunk 1 is missing
	_, err = db.FinalizeUploadSession(ctx, sessionID)
	if err == nil {
		t.Fatalf("expected FinalizeUploadSession to fail with incomplete chunks")
	}

	// Save chunk 1 (10MB)
	chunk1 := &UploadSessionChunk{
		SessionID:      sessionID,
		ChunkIndex:     1,
		TelegramMsgID:  1002,
		TelegramFileID: "tg-chunk-1",
		BotTokenHash:   "bot-hash-2",
		Size:           10 * 1024 * 1024,
		RawSize:        10 * 1024 * 1024,
		IV:             "iv1",
		AuthTag:        "tag1",
	}
	if err := db.SaveUploadSessionChunk(ctx, chunk1); err != nil {
		t.Fatalf("SaveUploadSessionChunk 1 failed: %v", err)
	}

	// Now Finalize should succeed
	finFile, err := db.FinalizeUploadSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("FinalizeUploadSession failed: %v", err)
	}
	if finFile == nil || finFile.ID != fileID || finFile.Size != totalSize {
		t.Fatalf("unexpected finalized file: %+v", finFile)
	}

	// Verify chunks moved to main chunks table
	storedChunks, err := db.GetChunks(ctx, fileID)
	if err != nil {
		t.Fatalf("GetChunks failed: %v", err)
	}
	if len(storedChunks) != 2 {
		t.Fatalf("expected 2 stored chunks, got %d", len(storedChunks))
	}

	// Finalizing again should be idempotent and return the same file
	finFile2, err := db.FinalizeUploadSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("Idempotent finalize failed: %v", err)
	}
	if finFile2.ID != fileID {
		t.Fatalf("expected same file ID, got %s", finFile2.ID)
	}
}

func TestBotCredentialEncryptionMigrationAndRotation(t *testing.T) {
	ctx := context.Background()
	testDbFile := "test_bot_creds.db"
	defer os.Remove(testDbFile)

	db, err := New("file:"+testDbFile, "")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}

	rawToken1 := "123456:ABC-DEF-RAW_TELEGRAM_TOKEN_PLAIN_1"
	tokenHash1 := crypto.HashToken(rawToken1)
	ownerID := "alice"

	// 1. Insert a legacy plaintext bot record directly via raw SQL
	_, err = db.conn.ExecContext(ctx,
		`INSERT INTO registered_bots (token_hash, owner_id, token, disabled, created_at) VALUES (?, ?, ?, 0, CURRENT_TIMESTAMP)`,
		tokenHash1, ownerID, rawToken1,
	)
	if err != nil {
		t.Fatalf("insert raw plaintext bot: %v", err)
	}

	// 2. Querying before migration returns legacy plaintext token safely
	botBefore, err := db.GetBot(ctx, tokenHash1)
	if err != nil {
		t.Fatalf("GetBot before migration: %v", err)
	}
	if botBefore.Token != rawToken1 {
		t.Fatalf("expected legacy token %s, got %s", rawToken1, botBefore.Token)
	}

	// 3. Perform Migration using Key 1
	secretKey1 := "my_test_credential_secret_key_v1"
	credKey1 := crypto.DeriveCredentialKey(secretKey1)

	if err := db.MigrateBotCredentials(ctx, credKey1); err != nil {
		t.Fatalf("MigrateBotCredentials failed: %v", err)
	}
	db.SetCredentialKey(credKey1)

	// 4. Inspect raw database storage directly to verify it is encrypted with enc:v1:
	var rawStoredToken string
	err = db.conn.QueryRowContext(ctx, `SELECT token FROM registered_bots WHERE token_hash = ?`, tokenHash1).Scan(&rawStoredToken)
	if err != nil {
		t.Fatalf("query raw token column: %v", err)
	}
	if !strings.HasPrefix(rawStoredToken, "enc:v1:") {
		t.Fatalf("expected stored token to have enc:v1: prefix at rest, got: %s", rawStoredToken)
	}
	if strings.Contains(rawStoredToken, rawToken1) {
		t.Fatalf("stored token at rest leaks raw token in plaintext!")
	}

	// 5. Query through storage API and verify decrypted token is transparently returned
	botAfter, err := db.GetBot(ctx, tokenHash1)
	if err != nil {
		t.Fatalf("GetBot after migration: %v", err)
	}
	if botAfter.Token != rawToken1 {
		t.Fatalf("expected decrypted token %s, got %s", rawToken1, botAfter.Token)
	}

	// 6. Verify json.Marshal omits the raw token completely (json:"-")
	jsonBytes, err := json.Marshal(botAfter)
	if err != nil {
		t.Fatalf("marshal bot to JSON: %v", err)
	}
	if strings.Contains(string(jsonBytes), rawToken1) {
		t.Fatalf("JSON response leaks raw bot token: %s", string(jsonBytes))
	}
	if strings.Contains(string(jsonBytes), "enc:v1:") {
		t.Fatalf("JSON response leaks encrypted bot token: %s", string(jsonBytes))
	}

	// 7. Verify SaveBot automatically encrypts a new bot at rest
	rawToken2 := "987654:GHI-JKL-ANOTHER_RAW_TOKEN_2"
	tokenHash2 := crypto.HashToken(rawToken2)
	err = db.SaveBot(ctx, &RegisteredBot{
		TokenHash: tokenHash2,
		OwnerID:   ownerID,
		Token:     rawToken2,
		Disabled:  false,
	})
	if err != nil {
		t.Fatalf("SaveBot failed: %v", err)
	}

	var rawStoredToken2 string
	_ = db.conn.QueryRowContext(ctx, `SELECT token FROM registered_bots WHERE token_hash = ?`, tokenHash2).Scan(&rawStoredToken2)
	if !strings.HasPrefix(rawStoredToken2, "enc:v1:") {
		t.Fatalf("expected new bot to be encrypted at rest with enc:v1:, got %s", rawStoredToken2)
	}

	bot2, err := db.GetBot(ctx, tokenHash2)
	if err != nil || bot2.Token != rawToken2 {
		t.Fatalf("expected decrypted token %s, got %v (err: %v)", rawToken2, bot2.Token, err)
	}

	// 8. Key Rotation: Rotate from credKey1 to credKey2
	secretKey2 := "my_new_rotated_credential_secret_v2"
	credKey2 := crypto.DeriveCredentialKey(secretKey2)

	if err := db.RotateBotCredentials(ctx, credKey1, credKey2); err != nil {
		t.Fatalf("RotateBotCredentials failed: %v", err)
	}

	// Verify all bots can be queried and decrypted under new key
	allBots, err := db.GetAllBots(ctx)
	if err != nil {
		t.Fatalf("GetAllBots after rotation: %v", err)
	}
	if len(allBots) != 2 {
		t.Fatalf("expected 2 bots, got %d", len(allBots))
	}
	for _, b := range allBots {
		if b.TokenHash == tokenHash1 && b.Token != rawToken1 {
			t.Fatalf("bot 1 token mismatch after rotation: expected %s, got %s", rawToken1, b.Token)
		}
		if b.TokenHash == tokenHash2 && b.Token != rawToken2 {
			t.Fatalf("bot 2 token mismatch after rotation: expected %s, got %s", rawToken2, b.Token)
		}
	}
}

func TestChunkMigrationPersistenceAndReconciliation(t *testing.T) {
	ctx := context.Background()
	testDbFile := "test_migration_recovery.db"
	defer os.Remove(testDbFile)

	db, err := New("file:"+testDbFile, "")
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	chunkID := "chunk-12345"
	sourceHash := "source_bot_hash_aaa"
	targetHash := "target_bot_hash_bbb"
	targetFileID := "tg_dest_file_999"
	targetMsgID := 8888

	// 1. Stage in-flight destination upload in chunk_migrations
	if err := db.RecordChunkMigration(ctx, chunkID, sourceHash, targetHash, targetFileID, targetMsgID); err != nil {
		t.Fatalf("RecordChunkMigration failed: %v", err)
	}

	// 2. Retrieve pending record (simulating restart recovery)
	pending, err := db.GetPendingChunkMigration(ctx, chunkID)
	if err != nil || pending == nil {
		t.Fatalf("expected pending migration record, got %v (err: %v)", pending, err)
	}
	if pending.TargetFileID != targetFileID || pending.TargetMsgID != targetMsgID || pending.TargetBotHash != targetHash {
		t.Fatalf("unexpected pending record: %+v", pending)
	}

	// 3. Upsert / update staged record
	targetFileID2 := "tg_dest_file_updated"
	targetMsgID2 := 9999
	if err := db.RecordChunkMigration(ctx, chunkID, sourceHash, targetHash, targetFileID2, targetMsgID2); err != nil {
		t.Fatalf("RecordChunkMigration upsert failed: %v", err)
	}

	pending2, err := db.GetPendingChunkMigration(ctx, chunkID)
	if err != nil || pending2 == nil || pending2.TargetFileID != targetFileID2 || pending2.TargetMsgID != targetMsgID2 {
		t.Fatalf("expected updated pending record, got %+v (err: %v)", pending2, err)
	}

	// 4. Delete migration staging record after commit
	if err := db.DeleteChunkMigration(ctx, chunkID); err != nil {
		t.Fatalf("DeleteChunkMigration failed: %v", err)
	}

	pendingAfterDelete, err := db.GetPendingChunkMigration(ctx, chunkID)
	if err != nil || pendingAfterDelete != nil {
		t.Fatalf("expected nil after delete, got %v (err: %v)", pendingAfterDelete, err)
	}
}


