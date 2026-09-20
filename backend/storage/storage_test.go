package storage

import (
	"context"
	"os"
	"testing"
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
	chunk, err := db.CreateChunk(ctx, file.ID, 0, 1001, "tg_file_001", 1024, "iv_hex", "tag_hex")
	if err != nil {
		t.Fatalf("CreateChunk failed: %v", err)
	}
	if chunk.TelegramFileID != "tg_file_001" {
		t.Fatalf("unexpected telegram file id %s", chunk.TelegramFileID)
	}

	// List Files and Folders
	files, err := db.ListFiles(ctx, &folder.ID)
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
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
