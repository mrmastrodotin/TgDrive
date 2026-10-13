package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"tgdrive/config"
	"tgdrive/crypto"
	"tgdrive/storage"
	"tgdrive/telegram"
)

// setupMockTelegramServer creates a mock Telegram Bot API server that stores uploaded chunks in memory
// and serves them back for getFile / file download requests, enabling true end-to-end integration tests.
func setupMockTelegramServer() *httptest.Server {
	var mu sync.Mutex
	files := make(map[string][]byte)
	fileSeq := 100

	mux := http.NewServeMux()

	// Handle all bot requests
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if len(path) > 0 && path[len(path)-1] == '/' {
			path = path[:len(path)-1]
		}

		if strings.Contains(path, "sendDocument") {
			_ = r.ParseMultipartForm(32 << 20)

			mu.Lock()
			fileSeq++
			fileID := fmt.Sprintf("mock_file_%d", fileSeq)
			mu.Unlock()

			var data []byte
			if r.MultipartForm != nil && r.MultipartForm.File != nil {
				if fHeaders := r.MultipartForm.File["document"]; len(fHeaders) > 0 {
					f, err := fHeaders[0].Open()
					if err == nil {
						data, _ = io.ReadAll(f)
						f.Close()
					}
				}
			}

			mu.Lock()
			files[fileID] = data
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"ok": true,
				"result": {
					"message_id": %d,
					"document": {
						"file_id": "%s",
						"file_name": "chunk.data",
						"file_size": %d
					}
				}
			}`, fileSeq, fileID, len(data))))
			return
		}

		if strings.Contains(path, "getFile") {
			fileID := ""
			_ = r.ParseMultipartForm(10 << 20)
			if r.MultipartForm != nil && len(r.MultipartForm.Value["file_id"]) > 0 {
				fileID = r.MultipartForm.Value["file_id"][0]
			}
			if fileID == "" {
				fileID = r.FormValue("file_id")
			}
			if fileID == "" {
				bodyBytes, _ := io.ReadAll(r.Body)
				var bodyMap map[string]interface{}
				if err := json.Unmarshal(bodyBytes, &bodyMap); err == nil {
					if id, ok := bodyMap["file_id"].(string); ok {
						fileID = id
					}
				}
				if fileID == "" {
					fileID = r.URL.Query().Get("file_id")
				}
			}
			if fileID == "" {
				fileID = "mock_file_101"
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"ok": true,
				"result": {
					"file_id": "%s",
					"file_path": "documents/%s"
				}
			}`, fileID, fileID)))
			return
		}

		if strings.Contains(path, "/documents/") {
			parts := strings.Split(path, "/documents/")
			targetID := parts[len(parts)-1]

			mu.Lock()
			data, exists := files[targetID]
			mu.Unlock()

			if !exists {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("mock_document_fallback_bytes"))
				return
			}

			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}

		if strings.Contains(path, "deleteMessage") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok": true, "result": true}`))
			return
		}

		// Fallback ok
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true, "result": true}`))
	})

	return httptest.NewServer(mux)
}

func setupTestServer(t *testing.T, dbPath string, tgServerURL string) (*Server, *storage.DB, func()) {
	cfg := &config.Config{
		Port:        8080,
		MasterKey:   "test_master_key_for_tgdrive_32chars!!",
		TGBotTokens: []string{"test_bot_token_12345"},
		TGChannelID: -100123456789,
		TGServerURL: tgServerURL,
	}

	db, err := storage.New("file:"+dbPath, "")
	if err != nil {
		t.Fatalf("setup test db failed: %v", err)
	}

	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("db migrate failed: %v", err)
	}

	tgPool, err := telegram.NewBotPool(cfg.TGBotTokens, cfg.TGChannelID, cfg.TGServerURL)
	if err != nil {
		t.Fatalf("setup tg pool failed: %v", err)
	}

	server := NewServer(cfg, db, tgPool)

	cleanup := func() {
		_ = db.Close()
		_ = os.Remove(dbPath)
	}

	return server, db, cleanup
}

func setupTestServerWithBots(t *testing.T, dbPath string, tgServerURL string, tokens []string) (*Server, *storage.DB, func()) {
	cfg := &config.Config{
		Port:        8080,
		MasterKey:   "test_master_key_for_tgdrive_32chars!!",
		TGBotTokens: tokens,
		TGChannelID: -100123456789,
		TGServerURL: tgServerURL,
	}

	db, err := storage.New("file:"+dbPath, "")
	if err != nil {
		t.Fatalf("setup test db failed: %v", err)
	}

	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("db migrate failed: %v", err)
	}

	tgPool, err := telegram.NewBotPool(cfg.TGBotTokens, cfg.TGChannelID, cfg.TGServerURL)
	if err != nil {
		t.Fatalf("setup tg pool failed: %v", err)
	}

	server := NewServer(cfg, db, tgPool)

	cleanup := func() {
		_ = db.Close()
		_ = os.Remove(dbPath)
	}

	return server, db, cleanup
}


func TestChunkedUploadInitAndStatus(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, _, cleanup := setupTestServer(t, "test_api_init.db", tgServer.URL)
	defer cleanup()

	// 1. POST /api/files/upload/init
	initBody := UploadInitRequest{
		Name:      "test_movie.mp4",
		Size:      45 * 1024 * 1024, // 45 MB -> 3 chunks (20MB, 20MB, 5MB)
		MimeType:  "video/mp4",
		ChunkSize: 20 * 1024 * 1024,
	}
	bodyBytes, _ := json.Marshal(initBody)

	req := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(bodyBytes))
	rec := httptest.NewRecorder()
	server.HandleUploadInit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("HandleUploadInit expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var initResp UploadInitResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &initResp); err != nil {
		t.Fatalf("unmarshal init response: %v", err)
	}

	if initResp.TotalChunks != 3 {
		t.Fatalf("expected 3 total chunks, got %d", initResp.TotalChunks)
	}

	// 2. GET /api/files/upload/{uploadId}/status
	statusReq := httptest.NewRequest(http.MethodGet, "/api/files/upload/"+initResp.UploadID+"/status", nil)
	statusReq.SetPathValue("uploadId", initResp.UploadID)
	statusRec := httptest.NewRecorder()
	server.HandleUploadStatus(statusRec, statusReq)

	if statusRec.Code != http.StatusOK {
		t.Fatalf("HandleUploadStatus expected 200, got %d", statusRec.Code)
	}

	var statusResp UploadStatusResponse
	_ = json.Unmarshal(statusRec.Body.Bytes(), &statusResp)
	if statusResp.ConfirmedCount != 0 {
		t.Fatalf("expected 0 confirmed chunks initially, got %d", statusResp.ConfirmedCount)
	}
}

func TestChunkedUploadExecutionAndFinalize(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, _, cleanup := setupTestServer(t, "test_api_exec.db", tgServer.URL)
	defer cleanup()

	// 1. Initialize upload: 5 MB file with 2 MB chunks -> 3 chunks (2MB, 2MB, 1MB)
	chunkSize := 2 * 1024 * 1024
	totalSize := int64(5 * 1024 * 1024)

	initBody := UploadInitRequest{
		Name:      "test_document.pdf",
		Size:      totalSize,
		MimeType:  "application/pdf",
		ChunkSize: chunkSize,
	}
	b, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(b))
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)
	uploadID := initResp.UploadID

	// 2. Upload Chunk 0 (2 MB)
	chunk0Data := make([]byte, chunkSize)
	chunk0Req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", uploadID), bytes.NewReader(chunk0Data))
	chunk0Req.SetPathValue("uploadId", uploadID)
	chunk0Req.SetPathValue("chunkIndex", "0")
	chunk0Rec := httptest.NewRecorder()
	server.HandleUploadChunk(chunk0Rec, chunk0Req)
	if chunk0Rec.Code != http.StatusOK {
		t.Fatalf("upload chunk 0 failed (%d): %s", chunk0Rec.Code, chunk0Rec.Body.String())
	}

	// 3. Duplicate Chunk 0 submission should be idempotent (return 200 immediately)
	chunk0DupRec := httptest.NewRecorder()
	server.HandleUploadChunk(chunk0DupRec, chunk0Req)
	if chunk0DupRec.Code != http.StatusOK {
		t.Fatalf("duplicate chunk 0 upload failed: %d", chunk0DupRec.Code)
	}

	// 4. Try finalize before remaining chunks are uploaded (should fail with 400)
	finFailReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/finalize", uploadID), nil)
	finFailReq.SetPathValue("uploadId", uploadID)
	finFailRec := httptest.NewRecorder()
	server.HandleUploadFinalize(finFailRec, finFailReq)
	if finFailRec.Code != http.StatusBadRequest {
		t.Fatalf("expected premature finalize to fail with 400, got %d", finFailRec.Code)
	}

	// 5. Upload Chunk 1 out-of-order or in-order (2 MB)
	chunk1Data := make([]byte, chunkSize)
	chunk1Req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/1", uploadID), bytes.NewReader(chunk1Data))
	chunk1Req.SetPathValue("uploadId", uploadID)
	chunk1Req.SetPathValue("chunkIndex", "1")
	chunk1Rec := httptest.NewRecorder()
	server.HandleUploadChunk(chunk1Rec, chunk1Req)
	if chunk1Rec.Code != http.StatusOK {
		t.Fatalf("upload chunk 1 failed (%d): %s", chunk1Rec.Code, chunk1Rec.Body.String())
	}

	// 6. Upload Chunk 2 (final partial chunk: 1 MB)
	chunk2Data := make([]byte, 1*1024*1024)
	chunk2Req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/2", uploadID), bytes.NewReader(chunk2Data))
	chunk2Req.SetPathValue("uploadId", uploadID)
	chunk2Req.SetPathValue("chunkIndex", "2")
	chunk2Rec := httptest.NewRecorder()
	server.HandleUploadChunk(chunk2Rec, chunk2Req)
	if chunk2Rec.Code != http.StatusOK {
		t.Fatalf("upload chunk 2 failed (%d): %s", chunk2Rec.Code, chunk2Rec.Body.String())
	}

	// 7. Finalize now that all 3 chunks are confirmed
	finReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/finalize", uploadID), nil)
	finReq.SetPathValue("uploadId", uploadID)
	finRec := httptest.NewRecorder()
	server.HandleUploadFinalize(finRec, finReq)
	if finRec.Code != http.StatusOK {
		t.Fatalf("finalize failed (%d): %s", finRec.Code, finRec.Body.String())
	}

	var finalizedFile storage.File
	if err := json.Unmarshal(finRec.Body.Bytes(), &finalizedFile); err != nil {
		t.Fatalf("unmarshal finalized file: %v", err)
	}
	if finalizedFile.Size != totalSize || finalizedFile.ChunkCount != 3 {
		t.Fatalf("unexpected finalized file attributes: size=%d, chunks=%d", finalizedFile.Size, finalizedFile.ChunkCount)
	}
}

func TestLargeLogicalFileStreamingWithoutDiskAllocation(t *testing.T) {
	// Tests a logical 3 GB file upload using bounded streaming chunks without allocating a 3 GB fixture on disk!
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, _, cleanup := setupTestServer(t, "test_large_logical.db", tgServer.URL)
	defer cleanup()

	chunkSize := crypto.ChunkSize // 20 MiB
	logical3GB := int64(3 * 1024 * 1024 * 1024) // 3 GB

	initBody := UploadInitRequest{
		Name:      "3gb_large_movie.mkv",
		Size:      logical3GB,
		MimeType:  "video/x-matroska",
		ChunkSize: chunkSize,
	}
	b, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(b))
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)

	expectedChunks := int(math.Ceil(float64(logical3GB) / float64(chunkSize)))
	if initResp.TotalChunks != expectedChunks {
		t.Fatalf("expected %d total chunks for 3GB logical file, got %d", expectedChunks, initResp.TotalChunks)
	}

	// Upload Chunk 0 (20 MiB bounded reader without writing to disk)
	// We use io.LimitReader over a zero-reader to stream 20MB of bytes on the fly
	chunk0Reader := io.LimitReader(zeroReader{}, int64(chunkSize))
	chunkReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", initResp.UploadID), chunk0Reader)
	chunkReq.SetPathValue("uploadId", initResp.UploadID)
	chunkReq.SetPathValue("chunkIndex", "0")
	chunkRec := httptest.NewRecorder()
	server.HandleUploadChunk(chunkRec, chunkReq)

	if chunkRec.Code != http.StatusOK {
		t.Fatalf("upload chunk 0 for 3GB file failed (%d): %s", chunkRec.Code, chunkRec.Body.String())
	}

	// Verify status reports chunk 0 confirmed
	statusReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/upload/%s/status", initResp.UploadID), nil)
	statusReq.SetPathValue("uploadId", initResp.UploadID)
	statusRec := httptest.NewRecorder()
	server.HandleUploadStatus(statusRec, statusReq)

	var statusResp UploadStatusResponse
	_ = json.Unmarshal(statusRec.Body.Bytes(), &statusResp)
	if statusResp.ConfirmedCount != 1 || statusResp.ConfirmedChunks[0] != 0 {
		t.Fatalf("expected chunk 0 confirmed, got count=%d, list=%v", statusResp.ConfirmedCount, statusResp.ConfirmedChunks)
	}
}

func TestBackendRestartPreservesConfirmedChunks(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	dbFile := "test_restart_durability.db"
	server1, db1, cleanup1 := setupTestServer(t, dbFile, tgServer.URL)
	_ = cleanup1

	// 1. Initialize session with 2 chunks
	chunkSize := 1024 * 1024
	totalSize := int64(2 * 1024 * 1024)

	initBody := UploadInitRequest{
		Name:      "survive_restart.bin",
		Size:      totalSize,
		MimeType:  "application/octet-stream",
		ChunkSize: chunkSize,
	}
	b, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(b))
	initRec := httptest.NewRecorder()
	server1.HandleUploadInit(initRec, initReq)

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)
	uploadID := initResp.UploadID

	// 2. Upload Chunk 0
	chunk0Data := make([]byte, chunkSize)
	chunk0Req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", uploadID), bytes.NewReader(chunk0Data))
	chunk0Req.SetPathValue("uploadId", uploadID)
	chunk0Req.SetPathValue("chunkIndex", "0")
	chunk0Rec := httptest.NewRecorder()
	server1.HandleUploadChunk(chunk0Rec, chunk0Req)
	if chunk0Rec.Code != http.StatusOK {
		t.Fatalf("chunk 0 upload failed: %d", chunk0Rec.Code)
	}

	// 3. Simulate backend shutdown / restart by closing db1 and creating server2 on the same dbFile
	_ = db1.Close()

	server2, _, cleanup2 := setupTestServer(t, dbFile, tgServer.URL)
	defer cleanup2()

	// 4. Verify server2 recognizes chunk 0 is still confirmed!
	statusReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/upload/%s/status", uploadID), nil)
	statusReq.SetPathValue("uploadId", uploadID)
	statusRec := httptest.NewRecorder()
	server2.HandleUploadStatus(statusRec, statusReq)

	var statusResp UploadStatusResponse
	_ = json.Unmarshal(statusRec.Body.Bytes(), &statusResp)
	if statusResp.ConfirmedCount != 1 || statusResp.ConfirmedChunks[0] != 0 {
		t.Fatalf("expected chunk 0 to survive restart, got count=%d", statusResp.ConfirmedCount)
	}

	// 5. Upload Chunk 1 to server2
	chunk1Data := make([]byte, chunkSize)
	chunk1Req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/1", uploadID), bytes.NewReader(chunk1Data))
	chunk1Req.SetPathValue("uploadId", uploadID)
	chunk1Req.SetPathValue("chunkIndex", "1")
	chunk1Rec := httptest.NewRecorder()
	server2.HandleUploadChunk(chunk1Rec, chunk1Req)
	if chunk1Rec.Code != http.StatusOK {
		t.Fatalf("chunk 1 upload to server2 failed: %d", chunk1Rec.Code)
	}

	// 6. Finalize on server2
	finReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/finalize", uploadID), nil)
	finReq.SetPathValue("uploadId", uploadID)
	finRec := httptest.NewRecorder()
	server2.HandleUploadFinalize(finRec, finReq)
	if finRec.Code != http.StatusOK {
		t.Fatalf("finalize after restart failed: %d", finRec.Code)
	}
}

func TestUploadCancellation(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, _, cleanup := setupTestServer(t, "test_api_cancel.db", tgServer.URL)
	defer cleanup()

	initBody := UploadInitRequest{
		Name:      "cancel_me.mp4",
		Size:      10 * 1024 * 1024,
		ChunkSize: 5 * 1024 * 1024,
	}
	b, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(b))
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)

	cancelReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/cancel", initResp.UploadID), nil)
	cancelReq.SetPathValue("uploadId", initResp.UploadID)
	cancelRec := httptest.NewRecorder()
	server.HandleUploadCancel(cancelRec, cancelReq)

	if cancelRec.Code != http.StatusOK {
		t.Fatalf("cancel failed (%d)", cancelRec.Code)
	}

	// Status should now be cancelled
	statusReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/upload/%s/status", initResp.UploadID), nil)
	statusReq.SetPathValue("uploadId", initResp.UploadID)
	statusRec := httptest.NewRecorder()
	server.HandleUploadStatus(statusRec, statusReq)

	var statusResp UploadStatusResponse
	_ = json.Unmarshal(statusRec.Body.Bytes(), &statusResp)
	if statusResp.Status != "cancelled" {
		t.Fatalf("expected status 'cancelled', got %s", statusResp.Status)
	}
}

func TestEmptyFileUploadAndFinalize(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, _, cleanup := setupTestServer(t, "test_api_empty.db", tgServer.URL)
	defer cleanup()

	initBody := UploadInitRequest{
		Name:      "empty.txt",
		Size:      0,
		MimeType:  "text/plain",
		ChunkSize: 20 * 1024 * 1024,
	}
	b, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(b))
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)
	if initResp.TotalChunks != 1 {
		t.Fatalf("expected 1 chunk for 0-byte file, got %d", initResp.TotalChunks)
	}

	// Upload empty chunk 0
	chunkReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", initResp.UploadID), bytes.NewReader([]byte{}))
	chunkReq.SetPathValue("uploadId", initResp.UploadID)
	chunkReq.SetPathValue("chunkIndex", "0")
	chunkRec := httptest.NewRecorder()
	server.HandleUploadChunk(chunkRec, chunkReq)
	if chunkRec.Code != http.StatusOK {
		t.Fatalf("empty chunk upload failed: %d", chunkRec.Code)
	}

	// Finalize empty file
	finReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/finalize", initResp.UploadID), nil)
	finReq.SetPathValue("uploadId", initResp.UploadID)
	finRec := httptest.NewRecorder()
	server.HandleUploadFinalize(finRec, finReq)
	if finRec.Code != http.StatusOK {
		t.Fatalf("empty file finalize failed (%d): %s", finRec.Code, finRec.Body.String())
	}

	var finalized storage.File
	_ = json.Unmarshal(finRec.Body.Bytes(), &finalized)
	if finalized.Size != 0 {
		t.Fatalf("expected 0 bytes size, got %d", finalized.Size)
	}
}

func TestInvalidChunkIndicesAndSizes(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, _, cleanup := setupTestServer(t, "test_api_invalids.db", tgServer.URL)
	defer cleanup()

	initBody := UploadInitRequest{
		Name:      "strict.bin",
		Size:      1000,
		ChunkSize: 1000,
	}
	b, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(b))
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)

	// 1. Chunk index out of bounds (index 5 when totalChunks is 1)
	req1 := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/5", initResp.UploadID), bytes.NewReader(make([]byte, 1000)))
	req1.SetPathValue("uploadId", initResp.UploadID)
	req1.SetPathValue("chunkIndex", "5")
	rec1 := httptest.NewRecorder()
	server.HandleUploadChunk(rec1, req1)
	if rec1.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for out of bounds chunk index, got %d", rec1.Code)
	}

	// 2. Chunk size mismatch: expected 1000 bytes, provided 500 bytes
	req2 := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", initResp.UploadID), bytes.NewReader(make([]byte, 500)))
	req2.SetPathValue("uploadId", initResp.UploadID)
	req2.SetPathValue("chunkIndex", "0")
	rec2 := httptest.NewRecorder()
	server.HandleUploadChunk(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for truncated chunk body, got %d", rec2.Code)
	}

	// 3. Chunk size mismatch: expected 1000 bytes, provided 2000 bytes
	req3 := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", initResp.UploadID), bytes.NewReader(make([]byte, 2000)))
	req3.SetPathValue("uploadId", initResp.UploadID)
	req3.SetPathValue("chunkIndex", "0")
	rec3 := httptest.NewRecorder()
	server.HandleUploadChunk(rec3, req3)
	if rec3.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized chunk body, got %d", rec3.Code)
	}
}

func TestConcurrentFinalization(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, _, cleanup := setupTestServer(t, "test_api_concurrent_fin.db", tgServer.URL)
	defer cleanup()

	initBody := UploadInitRequest{
		Name:      "concurrent.bin",
		Size:      100,
		ChunkSize: 100,
	}
	b, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(b))
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)

	// Upload chunk 0
	chunkReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", initResp.UploadID), bytes.NewReader(make([]byte, 100)))
	chunkReq.SetPathValue("uploadId", initResp.UploadID)
	chunkReq.SetPathValue("chunkIndex", "0")
	chunkRec := httptest.NewRecorder()
	server.HandleUploadChunk(chunkRec, chunkReq)
	if chunkRec.Code != http.StatusOK {
		t.Fatalf("upload chunk failed: %d", chunkRec.Code)
	}

	// Run 5 concurrent finalize requests
	const concurrency = 5
	results := make(chan int, concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/finalize", initResp.UploadID), nil)
			req.SetPathValue("uploadId", initResp.UploadID)
			rec := httptest.NewRecorder()
			server.HandleUploadFinalize(rec, req)
			results <- rec.Code
		}()
	}

	for i := 0; i < concurrency; i++ {
		code := <-results
		if code != http.StatusOK {
			t.Fatalf("concurrent finalize attempt returned status %d", code)
		}
	}
}

// zeroReader is a zero-allocation io.Reader that returns infinite zeros.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (n int, err error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func TestConcurrentDuplicateChunkUpload(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, db, cleanup := setupTestServer(t, "test_api_dup_chunk.db", tgServer.URL)
	defer cleanup()

	initBody := UploadInitRequest{
		Name:      "duplicate_chunk_race.bin",
		Size:      1024,
		ChunkSize: 1024,
	}
	b, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(b))
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)

	// Launch two concurrent uploads for chunk 0
	var wg sync.WaitGroup
	wg.Add(2)
	chunkData := make([]byte, 1024)
	codes := make([]int, 2)

	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", initResp.UploadID), bytes.NewReader(chunkData))
			req.SetPathValue("uploadId", initResp.UploadID)
			req.SetPathValue("chunkIndex", "0")
			rec := httptest.NewRecorder()
			server.HandleUploadChunk(rec, req)
			codes[idx] = rec.Code
		}(i)
	}

	wg.Wait()

	for _, code := range codes {
		if code != http.StatusOK {
			t.Fatalf("expected both concurrent chunk upload requests to succeed (one upload, one already_confirmed), got status %d", code)
		}
	}

	// Verify only 1 chunk record exists for this upload session
	indices, err := db.GetConfirmedChunkIndices(context.Background(), initResp.UploadID)
	if err != nil {
		t.Fatalf("get confirmed chunk indices failed: %v", err)
	}
	if len(indices) != 1 {
		t.Fatalf("expected exactly 1 chunk stored, got %d", len(indices))
	}
}

func TestSourceFingerprintVerification(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, _, cleanup := setupTestServer(t, "test_api_fingerprint.db", tgServer.URL)
	defer cleanup()

	fingerprint := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	initBody := UploadInitRequest{
		Name:              "fingerprinted.bin",
		Size:              2048,
		ChunkSize:         1024,
		SourceFingerprint: fingerprint,
	}
	b, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(b))
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)

	statusReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/upload/%s/status", initResp.UploadID), nil)
	statusReq.SetPathValue("uploadId", initResp.UploadID)
	statusRec := httptest.NewRecorder()
	server.HandleUploadStatus(statusRec, statusReq)

	var statusResp UploadStatusResponse
	_ = json.Unmarshal(statusRec.Body.Bytes(), &statusResp)
	if statusResp.SourceFingerprint != fingerprint {
		t.Fatalf("expected source fingerprint %s, got %s", fingerprint, statusResp.SourceFingerprint)
	}
}

func TestUploadSessionOwnershipEnforcement(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, _, cleanup := setupTestServer(t, "test_api_ownership.db", tgServer.URL)
	defer cleanup()

	// Register a dedicated bot for Alice
	if err := server.tg.AddBot("alice", "alice_bot_token_secret_12345"); err != nil {
		t.Fatalf("failed to add bot for alice: %v", err)
	}

	// Alice creates upload session
	initBody := UploadInitRequest{
		Name:      "alice_private.bin",
		Size:      1024,
		ChunkSize: 1024,
	}
	b, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(b))
	aliceCtx := WithUserContext(context.Background(), "alice")
	initReq = initReq.WithContext(aliceCtx)
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)
	uploadID := initResp.UploadID

	bobCtx := WithUserContext(context.Background(), "bob")

	// Bob tries to upload a chunk -> 403 Forbidden
	chunkReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", uploadID), bytes.NewReader(make([]byte, 1024)))
	chunkReq.SetPathValue("uploadId", uploadID)
	chunkReq.SetPathValue("chunkIndex", "0")
	chunkReq = chunkReq.WithContext(bobCtx)
	chunkRec := httptest.NewRecorder()
	server.HandleUploadChunk(chunkRec, chunkReq)
	if chunkRec.Code != http.StatusForbidden {
		t.Fatalf("expected Bob's chunk upload to return 403, got %d", chunkRec.Code)
	}

	// Bob tries to check status -> 403 Forbidden
	statusReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/upload/%s/status", uploadID), nil)
	statusReq.SetPathValue("uploadId", uploadID)
	statusReq = statusReq.WithContext(bobCtx)
	statusRec := httptest.NewRecorder()
	server.HandleUploadStatus(statusRec, statusReq)
	if statusRec.Code != http.StatusForbidden {
		t.Fatalf("expected Bob's status check to return 403, got %d", statusRec.Code)
	}

	// Bob tries to finalize -> 403 Forbidden
	finReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/finalize", uploadID), nil)
	finReq.SetPathValue("uploadId", uploadID)
	finReq = finReq.WithContext(bobCtx)
	finRec := httptest.NewRecorder()
	server.HandleUploadFinalize(finRec, finReq)
	if finRec.Code != http.StatusForbidden {
		t.Fatalf("expected Bob's finalize to return 403, got %d", finRec.Code)
	}

	// Bob tries to cancel -> 403 Forbidden
	cancelReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/cancel", uploadID), nil)
	cancelReq.SetPathValue("uploadId", uploadID)
	cancelReq = cancelReq.WithContext(bobCtx)
	cancelRec := httptest.NewRecorder()
	server.HandleUploadCancel(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusForbidden {
		t.Fatalf("expected Bob's cancel to return 403, got %d", cancelRec.Code)
	}

	// Alice uploads chunk 0 -> 200 OK
	aliceChunkReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", uploadID), bytes.NewReader(make([]byte, 1024)))
	aliceChunkReq.SetPathValue("uploadId", uploadID)
	aliceChunkReq.SetPathValue("chunkIndex", "0")
	aliceChunkReq = aliceChunkReq.WithContext(aliceCtx)
	aliceChunkRec := httptest.NewRecorder()
	server.HandleUploadChunk(aliceChunkRec, aliceChunkReq)
	if aliceChunkRec.Code != http.StatusOK {
		t.Fatalf("expected Alice's chunk upload to return 200, got %d: %s", aliceChunkRec.Code, aliceChunkRec.Body.String())
	}
}

func TestRangeStreamingAndDownloadE2E(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, _, cleanup := setupTestServer(t, "test_api_streaming_e2e.db", tgServer.URL)
	defer cleanup()

	// Upload a 500-byte file across 4 chunks: 150, 150, 150, 50 bytes
	totalSize := int64(500)
	chunkSize := 150

	originalData := make([]byte, totalSize)
	for i := range originalData {
		originalData[i] = byte(i % 256)
	}

	initBody := UploadInitRequest{
		Name:      "test_stream.bin",
		Size:      totalSize,
		MimeType:  "application/octet-stream",
		ChunkSize: chunkSize,
	}
	b, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(b))
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)
	uploadID := initResp.UploadID

	for i := 0; i < initResp.TotalChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if int64(end) > totalSize {
			end = int(totalSize)
		}
		cData := originalData[start:end]

		cReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/%d", uploadID, i), bytes.NewReader(cData))
		cReq.SetPathValue("uploadId", uploadID)
		cReq.SetPathValue("chunkIndex", strconv.Itoa(i))
		cRec := httptest.NewRecorder()
		server.HandleUploadChunk(cRec, cReq)
		if cRec.Code != http.StatusOK {
			t.Fatalf("chunk %d upload failed: %d (%s)", i, cRec.Code, cRec.Body.String())
		}
	}

	finReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/finalize", uploadID), nil)
	finReq.SetPathValue("uploadId", uploadID)
	finRec := httptest.NewRecorder()
	server.HandleUploadFinalize(finRec, finReq)
	if finRec.Code != http.StatusOK {
		t.Fatalf("finalize failed: %d (%s)", finRec.Code, finRec.Body.String())
	}

	var finalizedFile storage.File
	_ = json.Unmarshal(finRec.Body.Bytes(), &finalizedFile)
	fileID := finalizedFile.ID

	// 1. Test full download endpoint GET /api/files/{id}/download
	downReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/download", fileID), nil)
	downReq.SetPathValue("id", fileID)
	downRec := httptest.NewRecorder()
	server.HandleDownloadFile(downRec, downReq)

	if downRec.Code != http.StatusOK {
		t.Fatalf("full download failed (%d): %s", downRec.Code, downRec.Body.String())
	}
	if !bytes.Equal(downRec.Body.Bytes(), originalData) {
		t.Fatalf("downloaded data mismatch: got %d bytes, want %d bytes", len(downRec.Body.Bytes()), len(originalData))
	}
	if cd := downRec.Header().Get("Content-Disposition"); !strings.Contains(cd, "test_stream.bin") {
		t.Fatalf("unexpected Content-Disposition: %s", cd)
	}

	// 2. Test HEAD on download
	headDownReq := httptest.NewRequest(http.MethodHead, fmt.Sprintf("/api/files/%s/download", fileID), nil)
	headDownReq.SetPathValue("id", fileID)
	headDownRec := httptest.NewRecorder()
	server.HandleDownloadFile(headDownRec, headDownReq)
	if headDownRec.Code != http.StatusOK {
		t.Fatalf("head download failed: %d", headDownRec.Code)
	}
	if headDownRec.Body.Len() != 0 {
		t.Fatalf("expected empty body on HEAD download, got %d bytes", headDownRec.Body.Len())
	}

	// 3. Test Range: bytes=10-40 (single chunk within chunk 0)
	r1Req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/stream", fileID), nil)
	r1Req.SetPathValue("id", fileID)
	r1Req.Header.Set("Range", "bytes=10-40")
	r1Rec := httptest.NewRecorder()
	server.HandleStreamFile(r1Rec, r1Req)

	if r1Rec.Code != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content, got %d: %s", r1Rec.Code, r1Rec.Body.String())
	}
	if r1Rec.Header().Get("Content-Range") != "bytes 10-40/500" {
		t.Fatalf("expected Content-Range 'bytes 10-40/500', got %s", r1Rec.Header().Get("Content-Range"))
	}
	if !bytes.Equal(r1Rec.Body.Bytes(), originalData[10:41]) {
		t.Fatalf("range data mismatch for bytes=10-40")
	}

	// 4. Test Range: bytes=100-350 (spans chunk 0, 1, and 2)
	r2Req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/stream", fileID), nil)
	r2Req.SetPathValue("id", fileID)
	r2Req.Header.Set("Range", "bytes=100-350")
	r2Rec := httptest.NewRecorder()
	server.HandleStreamFile(r2Rec, r2Req)

	if r2Rec.Code != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content, got %d", r2Rec.Code)
	}
	if r2Rec.Header().Get("Content-Range") != "bytes 100-350/500" {
		t.Fatalf("expected Content-Range 'bytes 100-350/500', got %s", r2Rec.Header().Get("Content-Range"))
	}
	if !bytes.Equal(r2Rec.Body.Bytes(), originalData[100:351]) {
		t.Fatalf("multi-chunk range data mismatch")
	}

	// 5. Test Suffix Range: bytes=-50 (last 50 bytes)
	r3Req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/stream", fileID), nil)
	r3Req.SetPathValue("id", fileID)
	r3Req.Header.Set("Range", "bytes=-50")
	r3Rec := httptest.NewRecorder()
	server.HandleStreamFile(r3Rec, r3Req)

	if r3Rec.Code != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content for suffix range, got %d", r3Rec.Code)
	}
	if r3Rec.Header().Get("Content-Range") != "bytes 450-499/500" {
		t.Fatalf("expected Content-Range 'bytes 450-499/500', got %s", r3Rec.Header().Get("Content-Range"))
	}
	if !bytes.Equal(r3Rec.Body.Bytes(), originalData[450:500]) {
		t.Fatalf("suffix range data mismatch")
	}

	// 6. Test Open-Ended Range: bytes=400-
	r4Req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/stream", fileID), nil)
	r4Req.SetPathValue("id", fileID)
	r4Req.Header.Set("Range", "bytes=400-")
	r4Rec := httptest.NewRecorder()
	server.HandleStreamFile(r4Rec, r4Req)

	if r4Rec.Code != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content for open-ended range, got %d", r4Rec.Code)
	}
	if r4Rec.Header().Get("Content-Range") != "bytes 400-499/500" {
		t.Fatalf("expected Content-Range 'bytes 400-499/500', got %s", r4Rec.Header().Get("Content-Range"))
	}
	if !bytes.Equal(r4Rec.Body.Bytes(), originalData[400:500]) {
		t.Fatalf("open-ended range data mismatch")
	}

	// 7. Test Invalid Range: bytes=700-800 -> 416
	r5Req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/stream", fileID), nil)
	r5Req.SetPathValue("id", fileID)
	r5Req.Header.Set("Range", "bytes=700-800")
	r5Rec := httptest.NewRecorder()
	server.HandleStreamFile(r5Rec, r5Req)

	if r5Rec.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("expected 416 Requested Range Not Satisfiable, got %d", r5Rec.Code)
	}
	if r5Rec.Header().Get("Content-Range") != "bytes */500" {
		t.Fatalf("expected Content-Range 'bytes */500', got %s", r5Rec.Header().Get("Content-Range"))
	}

	// 8. Test HEAD stream request
	headStreamReq := httptest.NewRequest(http.MethodHead, fmt.Sprintf("/api/files/%s/stream", fileID), nil)
	headStreamReq.SetPathValue("id", fileID)
	headStreamReq.Header.Set("Range", "bytes=10-20")
	headStreamRec := httptest.NewRecorder()
	server.HandleStreamFile(headStreamRec, headStreamReq)

	if headStreamRec.Code != http.StatusPartialContent {
		t.Fatalf("head stream expected 206, got %d", headStreamRec.Code)
	}
	if headStreamRec.Body.Len() != 0 {
		t.Fatalf("head stream expected empty body, got %d bytes", headStreamRec.Body.Len())
	}
}

func TestAESGCMAuthenticationFailure(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, db, cleanup := setupTestServer(t, "test_api_auth_fail.db", tgServer.URL)
	defer cleanup()

	initBody := UploadInitRequest{
		Name:      "tampered.bin",
		Size:      100,
		ChunkSize: 100,
	}
	b, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(b))
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)

	// Upload chunk 0
	cReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", initResp.UploadID), bytes.NewReader(make([]byte, 100)))
	cReq.SetPathValue("uploadId", initResp.UploadID)
	cReq.SetPathValue("chunkIndex", "0")
	cRec := httptest.NewRecorder()
	server.HandleUploadChunk(cRec, cReq)
	if cRec.Code != http.StatusOK {
		t.Fatalf("chunk upload failed: %d", cRec.Code)
	}

	// Finalize
	finReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/finalize", initResp.UploadID), nil)
	finReq.SetPathValue("uploadId", initResp.UploadID)
	finRec := httptest.NewRecorder()
	server.HandleUploadFinalize(finRec, finReq)
	if finRec.Code != http.StatusOK {
		t.Fatalf("finalize failed: %d", finRec.Code)
	}

	// Tamper with chunk 0 auth tag in database!
	_, err := db.Exec(context.Background(), "UPDATE chunks SET auth_tag = '00000000000000000000000000000000' WHERE file_id = ? AND chunk_index = 0", initResp.FileID)
	if err != nil {
		t.Fatalf("tampering chunk tag failed: %v", err)
	}

	// Clear LRU chunk cache so it has to decrypt from storage
	server.chunkCache.Clear()

	// Attempt stream — should fail authentication and emit no plaintext bytes
	streamReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/stream", initResp.FileID), nil)
	streamReq.SetPathValue("id", initResp.FileID)
	streamRec := httptest.NewRecorder()
	server.HandleStreamFile(streamRec, streamReq)

	if streamRec.Body.Len() > 0 {
		t.Fatalf("tampered chunk must not emit unauthenticated plaintext, emitted %d bytes", streamRec.Body.Len())
	}
}

func TestBotRemovalSafeguards(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, _, cleanup := setupTestServer(t, "test_api_bot_removal.db", tgServer.URL)
	defer cleanup()

	// Add second bot
	bot2Token := "test_bot_token_second"
	if err := server.tg.AddBot(bot2Token); err != nil {
		t.Fatalf("add bot 2 failed: %v", err)
	}

	// Upload a file (chunks will be stored under bot 1 or 2)
	initBody := UploadInitRequest{
		Name:      "bot_bound.bin",
		Size:      100,
		ChunkSize: 100,
	}
	b, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(b))
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)

	cReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", initResp.UploadID), bytes.NewReader(make([]byte, 100)))
	cReq.SetPathValue("uploadId", initResp.UploadID)
	cReq.SetPathValue("chunkIndex", "0")
	cRec := httptest.NewRecorder()
	server.HandleUploadChunk(cRec, cReq)
	if cRec.Code != http.StatusOK {
		t.Fatalf("upload chunk failed: %d", cRec.Code)
	}

	finReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/finalize", initResp.UploadID), nil)
	finReq.SetPathValue("uploadId", initResp.UploadID)
	finRec := httptest.NewRecorder()
	server.HandleUploadFinalize(finRec, finReq)
	if finRec.Code != http.StatusOK {
		t.Fatalf("finalize failed: %d", finRec.Code)
	}

	// Find which bot owns chunk 0
	chunks, err := server.db.GetChunks(context.Background(), initResp.FileID)
	if err != nil || len(chunks) == 0 {
		t.Fatalf("get chunks failed: %v", err)
	}
	owningBotHash := chunks[0].BotTokenHash

	// Attempt removing the owning bot without ?force=true -> 409 Conflict
	delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/bots/%s", owningBotHash), nil)
	delReq.SetPathValue("hash", owningBotHash)
	delRec := httptest.NewRecorder()
	server.HandleRemoveBot(delRec, delReq)

	if delRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when deleting bot with stored chunks, got %d: %s", delRec.Code, delRec.Body.String())
	}

	// Attempt removing with ?force=true -> 200 OK
	forceReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/bots/%s?force=true", owningBotHash), nil)
	forceReq.SetPathValue("hash", owningBotHash)
	forceRec := httptest.NewRecorder()
	server.HandleRemoveBot(forceRec, forceReq)

	if forceRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK when force deleting bot, got %d: %s", forceRec.Code, forceRec.Body.String())
	}
}

func TestBotPoolSafeCapacity(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, _, cleanup := setupTestServer(t, "test_api_capacity.db", tgServer.URL)
	defer cleanup()

	// Initial single bot -> capacity should be 1
	cap1 := server.tg.GetSafeCapacity()
	if cap1 != 1 {
		t.Fatalf("expected initial capacity 1, got %d", cap1)
	}

	// Add two more bots -> capacity should grow to 3
	_ = server.tg.AddBot("token_bot_extra_1")
	_ = server.tg.AddBot("token_bot_extra_2")

	cap3 := server.tg.GetSafeCapacity()
	if cap3 != 3 {
		t.Fatalf("expected capacity 3 with 3 online bots, got %d", cap3)
	}
}

func TestLegacyChunkBoundaryStreaming(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, db, cleanup := setupTestServer(t, "test_legacy_streaming.db", tgServer.URL)
	defer cleanup()

	// Legacy file layout:
	// Chunk size was implicitly 20 MiB (20971520 bytes).
	// Let's create a file of 22,020,096 bytes:
	// Chunk 0: 20,971,520 bytes
	// Chunk 1: 1,048,576 bytes
	totalSize := int64(22020096)
	chunk0Size := int64(20 * 1024 * 1024)
	chunk1Size := int64(1024 * 1024)

	salt, _ := crypto.GenerateSalt()
	key := crypto.DeriveKey([]byte(server.cfg.MasterKey), salt)

	// Build deterministic plaintext data generator
	getExpectedByte := func(offset int64) byte {
		return byte(offset % 251)
	}

	rawChunk0 := make([]byte, chunk0Size)
	for i := range rawChunk0 {
		rawChunk0[i] = getExpectedByte(int64(i))
	}
	iv0, _ := crypto.GenerateIV()
	ct0, tag0, err := crypto.EncryptChunk(key, iv0, rawChunk0)
	if err != nil {
		t.Fatalf("encrypt chunk 0: %v", err)
	}

	rawChunk1 := make([]byte, chunk1Size)
	for i := range rawChunk1 {
		rawChunk1[i] = getExpectedByte(chunk0Size + int64(i))
	}
	iv1, _ := crypto.GenerateIV()
	ct1, tag1, err := crypto.EncryptChunk(key, iv1, rawChunk1)
	if err != nil {
		t.Fatalf("encrypt chunk 1: %v", err)
	}

	// Upload to mock Telegram via the mock server's bot client
	res0, err := server.tg.UploadChunk(context.Background(), ct0, "chunk0.enc")
	if err != nil {
		t.Fatalf("tg upload chunk 0: %v", err)
	}
	res1, err := server.tg.UploadChunk(context.Background(), ct1, "chunk1.enc")
	if err != nil {
		t.Fatalf("tg upload chunk 1: %v", err)
	}

	// Insert legacy file into database with ChunkSize = 0 (legacy!)
	legacyFile, err := db.CreateFile(context.Background(), "legacy_movie.mp4", totalSize, "video/mp4", nil, crypto.HexEncode(salt), 2, 0, "admin")
	if err != nil {
		t.Fatalf("insert legacy file: %v", err)
	}
	legacyFileID := legacyFile.ID

	// Insert chunks
	_, err = db.CreateChunk(context.Background(), legacyFileID, 0, res0.MessageID, res0.FileID, res0.BotTokenHash, len(ct0), crypto.HexEncode(iv0), crypto.HexEncode(tag0))
	if err != nil {
		t.Fatalf("insert legacy chunk 0: %v", err)
	}
	_, err = db.CreateChunk(context.Background(), legacyFileID, 1, res1.MessageID, res1.FileID, res1.BotTokenHash, len(ct1), crypto.HexEncode(iv1), crypto.HexEncode(tag1))
	if err != nil {
		t.Fatalf("insert legacy chunk 1: %v", err)
	}

	type rangeTestCase struct {
		name         string
		rangeHeader  string
		expectedCode int
		expectedLen  int
		startOffset  int64
	}

	tests := []rangeTestCase{
		{
			name:         "Chunk 0 last byte (exact boundary)",
			rangeHeader:  "bytes=20971519-20971519",
			expectedCode: http.StatusPartialContent,
			expectedLen:  1,
			startOffset:  20971519,
		},
		{
			name:         "Chunk 1 first byte (exact boundary)",
			rangeHeader:  "bytes=20971520-20971520",
			expectedCode: http.StatusPartialContent,
			expectedLen:  1,
			startOffset:  20971520,
		},
		{
			name:         "Crossing boundary (Chunk 0 last 10 bytes + Chunk 1 first 11 bytes)",
			rangeHeader:  "bytes=20971510-20971530",
			expectedCode: http.StatusPartialContent,
			expectedLen:  21,
			startOffset:  20971510,
		},
		{
			name:         "Immediately before boundary",
			rangeHeader:  "bytes=20971515-20971519",
			expectedCode: http.StatusPartialContent,
			expectedLen:  5,
			startOffset:  20971515,
		},
		{
			name:         "Immediately after boundary",
			rangeHeader:  "bytes=20971520-20971525",
			expectedCode: http.StatusPartialContent,
			expectedLen:  6,
			startOffset:  20971520,
		},
		{
			name:         "Near EOF (last 6 bytes)",
			rangeHeader:  "bytes=22020090-22020095",
			expectedCode: http.StatusPartialContent,
			expectedLen:  6,
			startOffset:  22020090,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/stream", legacyFileID), nil)
			req.SetPathValue("id", legacyFileID)
			req.Header.Set("Range", tc.rangeHeader)
			rec := httptest.NewRecorder()

			server.HandleStreamFile(rec, req)

			if rec.Code != tc.expectedCode {
				t.Fatalf("expected code %d, got %d: %s", tc.expectedCode, rec.Code, rec.Body.String())
			}

			body := rec.Body.Bytes()
			if len(body) != tc.expectedLen {
				t.Fatalf("expected len %d, got %d", tc.expectedLen, len(body))
			}

			for i := 0; i < tc.expectedLen; i++ {
				expectedByte := getExpectedByte(tc.startOffset + int64(i))
				if body[i] != expectedByte {
					t.Fatalf("byte mismatch at offset %d: expected %d, got %d", tc.startOffset+int64(i), expectedByte, body[i])
				}
			}
		})
	}
}

func TestScopedStreamTicketAuthentication(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, db, cleanup := setupTestServer(t, "test_tickets.db", tgServer.URL)
	defer cleanup()

	// Insert test files
	fA, err := db.CreateFile(context.Background(), "testA.mp4", 100, "video/mp4", nil, "salt", 1, 100, "admin")
	if err != nil {
		t.Fatalf("insert file A: %v", err)
	}
	fB, err := db.CreateFile(context.Background(), "testB.mp4", 100, "video/mp4", nil, "salt", 1, 100, "admin")
	if err != nil {
		t.Fatalf("insert file B: %v", err)
	}
	fileA := fA.ID
	fileB := fB.ID

	// 1. Request ticket for fileA
	ticketReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/%s/ticket", fileA), nil)
	ticketReq.SetPathValue("id", fileA)
	ticketRec := httptest.NewRecorder()
	server.HandleCreateStreamTicket(ticketRec, ticketReq)

	if ticketRec.Code != http.StatusOK {
		t.Fatalf("create ticket expected 200, got %d", ticketRec.Code)
	}

	var ticketResp struct {
		Ticket    string `json:"ticket"`
		ExpiresAt int64  `json:"expires_at"`
		FileID    string `json:"file_id"`
	}
	_ = json.Unmarshal(ticketRec.Body.Bytes(), &ticketResp)
	if ticketResp.Ticket == "" {
		t.Fatal("empty ticket returned")
	}

	if ticketRec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("expected Referrer-Policy: no-referrer, got %s", ticketRec.Header().Get("Referrer-Policy"))
	}

	// Build handler wrapped with AuthMiddleware
	streamHandler := server.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("stream-ok"))
	}))

	// 2. Ticket used on File A stream -> 200 OK
	reqA := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/stream?ticket=%s", fileA, ticketResp.Ticket), nil)
	reqA.SetPathValue("id", fileA)
	recA := httptest.NewRecorder()
	streamHandler.ServeHTTP(recA, reqA)
	if recA.Code != http.StatusOK {
		t.Fatalf("expected 200 for fileA with ticket, got %d", recA.Code)
	}

	// 3. Ticket for File A used on File B stream -> 403 Forbidden!
	reqB := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/stream?ticket=%s", fileB, ticketResp.Ticket), nil)
	reqB.SetPathValue("id", fileB)
	recB := httptest.NewRecorder()
	streamHandler.ServeHTTP(recB, reqB)
	if recB.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for fileB with fileA ticket, got %d", recB.Code)
	}

	// 4. Ticket scoped to 'stream' cannot access '/api/files/A/download' -> 403 Forbidden!
	downloadHandler := server.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("download-ok"))
	}))
	reqDown := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/download?ticket=%s", fileA, ticketResp.Ticket), nil)
	reqDown.SetPathValue("id", fileA)
	recDown := httptest.NewRecorder()
	downloadHandler.ServeHTTP(recDown, reqDown)
	if recDown.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when using stream ticket for download endpoint, got %d", recDown.Code)
	}

	// 5. Create ticket scoped strictly to 'download' and verify it works on download but fails on stream
	dlTicketReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/%s/ticket?action=download", fileA), nil)
	dlTicketReq.SetPathValue("id", fileA)
	dlTicketRec := httptest.NewRecorder()
	server.HandleCreateStreamTicket(dlTicketRec, dlTicketReq)
	if dlTicketRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for download ticket, got %d", dlTicketRec.Code)
	}
	var dlTicketResp struct {
		Ticket string `json:"ticket"`
		Action string `json:"action"`
	}
	_ = json.Unmarshal(dlTicketRec.Body.Bytes(), &dlTicketResp)
	if dlTicketResp.Action != "download" {
		t.Fatalf("expected action 'download', got %s", dlTicketResp.Action)
	}

	// Download ticket on download -> 200 OK
	reqDlOk := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/download?ticket=%s", fileA, dlTicketResp.Ticket), nil)
	reqDlOk.SetPathValue("id", fileA)
	recDlOk := httptest.NewRecorder()
	downloadHandler.ServeHTTP(recDlOk, reqDlOk)
	if recDlOk.Code != http.StatusOK {
		t.Fatalf("expected 200 for download ticket on download endpoint, got %d", recDlOk.Code)
	}

	// Download ticket on stream -> 403 Forbidden!
	reqDlOnStream := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/stream?ticket=%s", fileA, dlTicketResp.Ticket), nil)
	reqDlOnStream.SetPathValue("id", fileA)
	recDlOnStream := httptest.NewRecorder()
	streamHandler.ServeHTTP(recDlOnStream, reqDlOnStream)
	if recDlOnStream.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when using download ticket on stream endpoint, got %d", recDlOnStream.Code)
	}

	// 6. Ticket used on unauthorized endpoint (e.g., /api/bots) -> 403 Forbidden!
	botHandler := server.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	reqBot := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/bots?ticket=%s", ticketResp.Ticket), nil)
	recBot := httptest.NewRecorder()
	botHandler.ServeHTTP(recBot, reqBot)
	if recBot.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for bot endpoint with stream ticket, got %d", recBot.Code)
	}

	// 7. Expired ticket -> 401 Unauthorized
	expiredClaims := AuthClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-10 * time.Minute)),
			Issuer:    "tgdrive",
			Subject:   "admin",
			Audience:  jwt.ClaimStrings{"stream:" + fileA},
		},
	}
	expiredToken := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims)
	expiredStr, _ := expiredToken.SignedString([]byte(server.cfg.JWTSecret))

	reqExp := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/stream?ticket=%s", fileA, expiredStr), nil)
	reqExp.SetPathValue("id", fileA)
	recExp := httptest.NewRecorder()
	streamHandler.ServeHTTP(recExp, reqExp)
	if recExp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for expired ticket, got %d", recExp.Code)
	}

	// 8. Test RedactSensitiveURL redacts ticket and token query params from logs
	rawURL := "https://tgdrive.app/api/files/xyz/stream?ticket=sensitive_ticket_jwt_token_123&quality=high"
	redacted := RedactSensitiveURL(rawURL)
	if strings.Contains(redacted, "sensitive_ticket_jwt_token_123") {
		t.Fatalf("RedactSensitiveURL failed to redact ticket: %s", redacted)
	}
	if !strings.Contains(redacted, "ticket=[REDACTED]") || !strings.Contains(redacted, "quality=high") {
		t.Fatalf("unexpected redacted URL format: %s", redacted)
	}
}

func TestTenantIsolation(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, db, cleanup := setupTestServer(t, "test_tenant.db", tgServer.URL)
	defer cleanup()

	// Insert file owned by Alice
	aliceFile, err := db.CreateFile(context.Background(), "secret.pdf", 100, "application/pdf", nil, "salt", 1, 100, "alice")
	if err != nil {
		t.Fatalf("insert alice file: %v", err)
	}
	aliceFileID := aliceFile.ID

	// Caller Bob tries to access Alice's file
	bobCtx := WithUserContext(context.Background(), "bob")

	// Download
	reqDl := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/download", aliceFileID), nil).WithContext(bobCtx)
	reqDl.SetPathValue("id", aliceFileID)
	recDl := httptest.NewRecorder()
	server.HandleDownloadFile(recDl, reqDl)
	if recDl.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for Bob downloading Alice's file, got %d", recDl.Code)
	}

	// Stream
	reqSt := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/stream", aliceFileID), nil).WithContext(bobCtx)
	reqSt.SetPathValue("id", aliceFileID)
	recSt := httptest.NewRecorder()
	server.HandleStreamFile(recSt, reqSt)
	if recSt.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for Bob streaming Alice's file, got %d", recSt.Code)
	}

	// Info
	reqInf := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s", aliceFileID), nil).WithContext(bobCtx)
	reqInf.SetPathValue("id", aliceFileID)
	recInf := httptest.NewRecorder()
	server.HandleGetFileInfo(recInf, reqInf)
	if recInf.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for Bob getting Alice's file info, got %d", recInf.Code)
	}

	// Delete
	reqDel := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/files/%s", aliceFileID), nil).WithContext(bobCtx)
	reqDel.SetPathValue("id", aliceFileID)
	recDel := httptest.NewRecorder()
	server.HandleDeleteFile(recDel, reqDel)
	if recDel.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for Bob deleting Alice's file, got %d", recDel.Code)
	}

	// Alice herself can access
	aliceCtx := WithUserContext(context.Background(), "alice")
	reqAlice := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s", aliceFileID), nil).WithContext(aliceCtx)
	reqAlice.SetPathValue("id", aliceFileID)
	recAlice := httptest.NewRecorder()
	server.HandleGetFileInfo(recAlice, reqAlice)
	if recAlice.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for Alice getting her own file, got %d", recAlice.Code)
	}
}

func TestPerChunkSHA256IntegrityAndResume(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, _, cleanup := setupTestServer(t, "test_sha256_resume.db", tgServer.URL)
	defer cleanup()

	initBody := UploadInitRequest{
		Name:      "integrity_test.bin",
		Size:      200,
		MimeType:  "application/octet-stream",
		ChunkSize: 100,
	}
	b, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(b))
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)
	uploadID := initResp.UploadID

	chunk0Data := bytes.Repeat([]byte{0x42}, 100)
	correctHash := fmt.Sprintf("%x", sha256.Sum256(chunk0Data))
	badHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	// 1. Upload chunk 0 with bad hash -> 400 Bad Request
	badReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", uploadID), bytes.NewReader(chunk0Data))
	badReq.SetPathValue("uploadId", uploadID)
	badReq.SetPathValue("chunkIndex", "0")
	badReq.Header.Set("X-Chunk-SHA256", badHash)
	badRec := httptest.NewRecorder()
	server.HandleUploadChunk(badRec, badReq)

	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for corrupted hash, got %d: %s", badRec.Code, badRec.Body.String())
	}

	// 2. Upload chunk 0 with correct hash -> 200 OK
	okReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", uploadID), bytes.NewReader(chunk0Data))
	okReq.SetPathValue("uploadId", uploadID)
	okReq.SetPathValue("chunkIndex", "0")
	okReq.Header.Set("X-Chunk-SHA256", correctHash)
	okRec := httptest.NewRecorder()
	server.HandleUploadChunk(okRec, okReq)

	if okRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for correct hash, got %d: %s", okRec.Code, okRec.Body.String())
	}

	// 3. Status returns confirmed chunk_hashes
	stReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/upload/%s/status", uploadID), nil)
	stReq.SetPathValue("uploadId", uploadID)
	stRec := httptest.NewRecorder()
	server.HandleUploadStatus(stRec, stReq)

	if stRec.Code != http.StatusOK {
		t.Fatalf("status failed: %d", stRec.Code)
	}

	var statusResp UploadStatusResponse
	_ = json.Unmarshal(stRec.Body.Bytes(), &statusResp)

	if statusResp.ChunkHashes == nil || statusResp.ChunkHashes[0] != correctHash {
		t.Fatalf("expected chunk hash %s in status response, got %v", correctHash, statusResp.ChunkHashes)
	}
}

func TestMemoryLimitsAndLockCleanup(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, _, cleanup := setupTestServer(t, "test_mem_cleanup.db", tgServer.URL)
	defer cleanup()

	// Verify defaults
	if server.cfg.CacheMaxBytes != 64*1024*1024 {
		t.Fatalf("expected default 64MB cache, got %d", server.cfg.CacheMaxBytes)
	}
	if server.cfg.MaxConcurrentUploads != 4 {
		t.Fatalf("expected default 4 concurrent uploads, got %d", server.cfg.MaxConcurrentUploads)
	}
	if server.cfg.MaxConcurrentDownloads != 4 {
		t.Fatalf("expected default 4 concurrent downloads, got %d", server.cfg.MaxConcurrentDownloads)
	}

	// Test memory stats endpoint
	memReq := httptest.NewRequest(http.MethodGet, "/api/system/memory", nil)
	memRec := httptest.NewRecorder()
	server.HandleGetMemoryStats(memRec, memReq)
	if memRec.Code != http.StatusOK {
		t.Fatalf("memory stats endpoint failed: %d", memRec.Code)
	}

	var memStats map[string]interface{}
	_ = json.Unmarshal(memRec.Body.Bytes(), &memStats)
	if _, ok := memStats["heap_alloc_bytes"]; !ok {
		t.Fatal("missing heap_alloc_bytes in memory stats")
	}

	// Test per-chunk lock cleanup on finalize
	initBody := UploadInitRequest{
		Name:      "lock_test.bin",
		Size:      100,
		MimeType:  "application/octet-stream",
		ChunkSize: 100,
	}
	b, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(b))
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)
	uploadID := initResp.UploadID

	cReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", uploadID), bytes.NewReader(make([]byte, 100)))
	cReq.SetPathValue("uploadId", uploadID)
	cReq.SetPathValue("chunkIndex", "0")
	cRec := httptest.NewRecorder()
	server.HandleUploadChunk(cRec, cReq)

	// Lock should exist before finalize
	lockKey := uploadID + ":0"
	if !server.chunkLockMgr.HasLock(lockKey) {
		t.Fatalf("expected lock %s to exist in chunkLockMgr", lockKey)
	}

	// Finalize should clean up locks
	finReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/finalize", uploadID), nil)
	finReq.SetPathValue("uploadId", uploadID)
	finRec := httptest.NewRecorder()
	server.HandleUploadFinalize(finRec, finReq)

	if server.chunkLockMgr.HasLock(lockKey) {
		t.Fatalf("expected lock %s to be cleaned up after finalize", lockKey)
	}

	// Test lock cleanup on cancel
	initReq2 := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(b))
	initRec2 := httptest.NewRecorder()
	server.HandleUploadInit(initRec2, initReq2)
	var initResp2 UploadInitResponse
	_ = json.Unmarshal(initRec2.Body.Bytes(), &initResp2)
	uploadID2 := initResp2.UploadID

	cReq2 := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", uploadID2), bytes.NewReader(make([]byte, 100)))
	cReq2.SetPathValue("uploadId", uploadID2)
	cReq2.SetPathValue("chunkIndex", "0")
	cRec2 := httptest.NewRecorder()
	server.HandleUploadChunk(cRec2, cReq2)

	lockKey2 := uploadID2 + ":0"
	if !server.chunkLockMgr.HasLock(lockKey2) {
		t.Fatalf("expected lock %s to exist in chunkLockMgr", lockKey2)
	}

	cancelReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/files/upload/%s", uploadID2), nil)
	cancelReq.SetPathValue("uploadId", uploadID2)
	cancelRec := httptest.NewRecorder()
	server.HandleUploadCancel(cancelRec, cancelReq)

	if server.chunkLockMgr.HasLock(lockKey2) {
		t.Fatalf("expected lock %s to be cleaned up after cancel", lockKey2)
	}
}

func TestTwoUserAPILevelBotIsolation(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, _, cleanup := setupTestServer(t, "test_api_two_user_isolation.db", tgServer.URL)
	defer cleanup()

	aliceCtx := WithUserContext(context.Background(), "alice")
	bobCtx := WithUserContext(context.Background(), "bob")
	charlieCtx := WithUserContext(context.Background(), "charlie")

	// 1. Alice registers 2 bots
	aliceToken1 := "11111:ALICE_BOT_1"
	aliceToken2 := "22222:ALICE_BOT_2"
	for _, tok := range []string{aliceToken1, aliceToken2} {
		body, _ := json.Marshal(map[string]string{"token": tok})
		req := httptest.NewRequest(http.MethodPost, "/api/bots", bytes.NewReader(body)).WithContext(aliceCtx)
		rec := httptest.NewRecorder()
		server.HandleAddBot(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("Alice add bot failed: %d %s", rec.Code, rec.Body.String())
		}
	}

	// 2. Bob registers 1 bot
	bobToken1 := "33333:BOB_BOT_1"
	body, _ := json.Marshal(map[string]string{"token": bobToken1})
	req := httptest.NewRequest(http.MethodPost, "/api/bots", bytes.NewReader(body)).WithContext(bobCtx)
	rec := httptest.NewRecorder()
	server.HandleAddBot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Bob add bot failed: %d %s", rec.Code, rec.Body.String())
	}

	// 3. Verify GET /api/bots isolation
	reqA := httptest.NewRequest(http.MethodGet, "/api/bots", nil).WithContext(aliceCtx)
	recA := httptest.NewRecorder()
	server.HandleListBots(recA, reqA)
	var listRespA struct {
		Total   int                  `json:"total"`
		Online  int                  `json:"online"`
		Workers []telegram.BotStatus `json:"workers"`
	}
	_ = json.Unmarshal(recA.Body.Bytes(), &listRespA)
	if listRespA.Total != 2 || len(listRespA.Workers) != 2 {
		t.Fatalf("expected Alice to have 2 bots, got total=%d, workers=%d", listRespA.Total, len(listRespA.Workers))
	}
	for _, w := range listRespA.Workers {
		if w.OwnerID != "alice" {
			t.Fatalf("expected Alice's worker to have owner alice, got %s", w.OwnerID)
		}
	}

	reqB := httptest.NewRequest(http.MethodGet, "/api/bots", nil).WithContext(bobCtx)
	recB := httptest.NewRecorder()
	server.HandleListBots(recB, reqB)
	var listRespB struct {
		Total   int                  `json:"total"`
		Online  int                  `json:"online"`
		Workers []telegram.BotStatus `json:"workers"`
	}
	_ = json.Unmarshal(recB.Body.Bytes(), &listRespB)
	if listRespB.Total != 1 || len(listRespB.Workers) != 1 {
		t.Fatalf("expected Bob to have 1 bot, got total=%d, workers=%d", listRespB.Total, len(listRespB.Workers))
	}
	if listRespB.Workers[0].OwnerID != "bob" {
		t.Fatalf("expected Bob's worker to have owner bob, got %s", listRespB.Workers[0].OwnerID)
	}

	// Charlie has 0 bots
	reqC := httptest.NewRequest(http.MethodGet, "/api/bots", nil).WithContext(charlieCtx)
	recC := httptest.NewRecorder()
	server.HandleListBots(recC, reqC)
	var listRespC struct {
		Total   int                  `json:"total"`
		Workers []telegram.BotStatus `json:"workers"`
	}
	_ = json.Unmarshal(recC.Body.Bytes(), &listRespC)
	if listRespC.Total != 0 || len(listRespC.Workers) != 0 {
		t.Fatalf("expected Charlie to have 0 bots, got total=%d", listRespC.Total)
	}

	// 4. Bob tries to remove Alice's bot -> 403 Forbidden
	aliceBotHash := listRespA.Workers[0].TokenHash
	delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/bots/%s", aliceBotHash), nil).WithContext(bobCtx)
	delReq.SetPathValue("hash", aliceBotHash)
	delRec := httptest.NewRecorder()
	server.HandleRemoveBot(delRec, delReq)
	if delRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when Bob tries to remove Alice's bot, got %d", delRec.Code)
	}

	// 5. Bob tries to disable Alice's bot -> 403 Forbidden
	disReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bots/%s/disable", aliceBotHash), nil).WithContext(bobCtx)
	disReq.SetPathValue("hash", aliceBotHash)
	disRec := httptest.NewRecorder()
	server.HandleDisableBot(disRec, disReq)
	if disRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when Bob tries to disable Alice's bot, got %d", disRec.Code)
	}

	// 6. Bob tries to rotate Alice's bot -> 403 Forbidden
	rotBody, _ := json.Marshal(map[string]string{"token": "99999:ATTACK_TOKEN"})
	rotReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bots/%s/rotate", aliceBotHash), bytes.NewReader(rotBody)).WithContext(bobCtx)
	rotReq.SetPathValue("hash", aliceBotHash)
	rotRec := httptest.NewRecorder()
	server.HandleRotateBot(rotRec, rotReq)
	if rotRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when Bob tries to rotate Alice's bot, got %d", rotRec.Code)
	}

	// 7. Charlie attempts to upload a chunk without registering any bots -> fails cleanly
	initBodyC := UploadInitRequest{
		Name:      "charlie_test.bin",
		Size:      100,
		ChunkSize: 100,
	}
	bC, _ := json.Marshal(initBodyC)
	cInitReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(bC)).WithContext(charlieCtx)
	cInitRec := httptest.NewRecorder()
	server.HandleUploadInit(cInitRec, cInitReq)
	var cInitResp UploadInitResponse
	_ = json.Unmarshal(cInitRec.Body.Bytes(), &cInitResp)

	cChunkReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", cInitResp.UploadID), bytes.NewReader(make([]byte, 100))).WithContext(charlieCtx)
	cChunkReq.SetPathValue("uploadId", cInitResp.UploadID)
	cChunkReq.SetPathValue("chunkIndex", "0")
	cChunkRec := httptest.NewRecorder()
	server.HandleUploadChunk(cChunkRec, cChunkReq)
	if cChunkRec.Code != http.StatusBadGateway || !strings.Contains(cChunkRec.Body.String(), "no active bot workers") {
		t.Fatalf("expected 502 with no active bot workers for Charlie, got %d: %s", cChunkRec.Code, cChunkRec.Body.String())
	}

	// 8. Alice uploads successfully using her bots
	initBodyA := UploadInitRequest{
		Name:      "alice_doc.bin",
		Size:      200,
		ChunkSize: 100,
	}
	bA, _ := json.Marshal(initBodyA)
	aInitReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(bA)).WithContext(aliceCtx)
	aInitRec := httptest.NewRecorder()
	server.HandleUploadInit(aInitRec, aInitReq)
	var aInitResp UploadInitResponse
	_ = json.Unmarshal(aInitRec.Body.Bytes(), &aInitResp)

	aChunkReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", aInitResp.UploadID), bytes.NewReader(make([]byte, 100))).WithContext(aliceCtx)
	aChunkReq.SetPathValue("uploadId", aInitResp.UploadID)
	aChunkReq.SetPathValue("chunkIndex", "0")
	aChunkRec := httptest.NewRecorder()
	server.HandleUploadChunk(aChunkRec, aChunkReq)
	if aChunkRec.Code != http.StatusOK {
		t.Fatalf("expected Alice chunk upload 200, got %d: %s", aChunkRec.Code, aChunkRec.Body.String())
	}

	// 9. Bob uploads successfully using his bot
	initBodyB := UploadInitRequest{
		Name:      "bob_doc.bin",
		Size:      100,
		ChunkSize: 100,
	}
	bB, _ := json.Marshal(initBodyB)
	bInitReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(bB)).WithContext(bobCtx)
	bInitRec := httptest.NewRecorder()
	server.HandleUploadInit(bInitRec, bInitReq)
	var bInitResp UploadInitResponse
	_ = json.Unmarshal(bInitRec.Body.Bytes(), &bInitResp)

	bChunkReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", bInitResp.UploadID), bytes.NewReader(make([]byte, 100))).WithContext(bobCtx)
	bChunkReq.SetPathValue("uploadId", bInitResp.UploadID)
	bChunkReq.SetPathValue("chunkIndex", "0")
	bChunkRec := httptest.NewRecorder()
	server.HandleUploadChunk(bChunkRec, bChunkReq)
	if bChunkRec.Code != http.StatusOK {
		t.Fatalf("expected Bob chunk upload 200, got %d: %s", bChunkRec.Code, bChunkRec.Body.String())
	}
}

func TestChunkLockManagerNoSimultaneousMutexes(t *testing.T) {
	mgr := NewChunkLockManager()
	uploadID := "upload-session-race-proof"
	chunkIndex := 0

	const concurrency = 25
	var startedWg sync.WaitGroup
	var doneWg sync.WaitGroup
	startedWg.Add(concurrency)
	doneWg.Add(concurrency)

	counter := 0
	activeInside := int32(0)
	maxActiveInside := int32(0)

	for i := 0; i < concurrency; i++ {
		go func() {
			startedWg.Done()
			unlock := mgr.Acquire(uploadID, chunkIndex)

			curr := atomic.AddInt32(&activeInside, 1)
			for {
				old := atomic.LoadInt32(&maxActiveInside)
				if curr <= old || atomic.CompareAndSwapInt32(&maxActiveInside, old, curr) {
					break
				}
			}

			time.Sleep(10 * time.Millisecond)
			counter++

			atomic.AddInt32(&activeInside, -1)
			unlock()
			doneWg.Done()
		}()
	}

	startedWg.Wait()
	// While workers are actively in flight, call ClearSession
	time.Sleep(3 * time.Millisecond)
	mgr.ClearSession(uploadID)

	key := fmt.Sprintf("%s:%d", uploadID, chunkIndex)
	if !mgr.HasLock(key) {
		t.Fatalf("expected lock %s to remain present while in-flight operations exist", key)
	}

	doneExtra := make(chan struct{})
	go func() {
		extraUnlock := mgr.Acquire(uploadID, chunkIndex)
		curr := atomic.AddInt32(&activeInside, 1)
		if curr > 1 {
			t.Errorf("critical section violation: activeInside = %d", curr)
		}
		time.Sleep(5 * time.Millisecond)
		counter++
		atomic.AddInt32(&activeInside, -1)
		extraUnlock()
		close(doneExtra)
	}()

	doneWg.Wait()
	<-doneExtra

	// After all in-flight workers release, the markedDeleted entry is cleaned up!
	mgr.ClearSession(uploadID)
	if mgr.HasLock(key) {
		t.Fatalf("expected lock %s to be cleaned up once all operations completed", key)
	}

	if maxActiveInside > 1 {
		t.Fatalf("expected mutual exclusion (maxActiveInside <= 1), got %d", maxActiveInside)
	}
	if counter != concurrency+1 {
		t.Fatalf("expected %d operations serialized, got %d", concurrency+1, counter)
	}
}

func TestMemoryBudgetAndConcurrentStress(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	// 1. Verify /api/system/memory admin authorization
	server, _, cleanup := setupTestServer(t, "test_api_memory_stress.db", tgServer.URL)
	defer cleanup()

	// Non-admin request rejected
	bobCtx := WithUserContext(context.Background(), "bob")
	nonAdminReq := httptest.NewRequest(http.MethodGet, "/api/system/memory", nil).WithContext(bobCtx)
	nonAdminRec := httptest.NewRecorder()
	server.HandleGetMemoryStats(nonAdminRec, nonAdminReq)
	if nonAdminRec.Code != http.StatusForbidden {
		t.Fatalf("expected non-admin to get 403 Forbidden for memory stats, got %d", nonAdminRec.Code)
	}

	// Admin request allowed
	adminCtx := WithUserContext(context.Background(), "admin")
	adminReq := httptest.NewRequest(http.MethodGet, "/api/system/memory", nil).WithContext(adminCtx)
	adminRec := httptest.NewRecorder()
	server.HandleGetMemoryStats(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("expected admin to get 200 OK for memory stats, got %d", adminRec.Code)
	}

	// 2. Admission control rejection test under tight budget
	server.cfg.MaxMemoryBudgetBytes = 1024 // 1 KB budget
	tightInitBody := UploadInitRequest{
		Name:      "large_payload.bin",
		Size:      1024 * 1024,
		ChunkSize: 1024 * 1024,
	}
	bT, _ := json.Marshal(tightInitBody)
	tInitReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(bT)).WithContext(adminCtx)
	tInitRec := httptest.NewRecorder()
	server.HandleUploadInit(tInitRec, tInitReq)
	var tInitResp UploadInitResponse
	_ = json.Unmarshal(tInitRec.Body.Bytes(), &tInitResp)

	tChunkReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", tInitResp.UploadID), bytes.NewReader(make([]byte, 1024*1024))).WithContext(adminCtx)
	tChunkReq.SetPathValue("uploadId", tInitResp.UploadID)
	tChunkReq.SetPathValue("chunkIndex", "0")
	tChunkRec := httptest.NewRecorder()
	server.HandleUploadChunk(tChunkRec, tChunkReq)
	if tChunkRec.Code != http.StatusServiceUnavailable || !strings.Contains(tChunkRec.Body.String(), "memory budget exceeded") {
		t.Fatalf("expected 503 memory budget exceeded, got %d: %s", tChunkRec.Code, tChunkRec.Body.String())
	}

	// 3. Repeatable concurrent stress test under normal budget
	server.cfg.MaxMemoryBudgetBytes = 384 * 1024 * 1024 // 384 MiB
	var startMem, endMem runtime.MemStats
	runtime.ReadMemStats(&startMem)

	const stressUploads = 6
	const stressStreams = 6
	var wg sync.WaitGroup
	wg.Add(stressUploads + stressStreams)

	// Pre-create an uploaded file for streaming
	streamData := make([]byte, 2048)
	for i := range streamData {
		streamData[i] = byte(i % 127)
	}
	sInitBody := UploadInitRequest{
		Name:      "stream_stress_target.bin",
		Size:      2048,
		ChunkSize: 1024,
	}
	bS, _ := json.Marshal(sInitBody)
	sInitReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(bS)).WithContext(adminCtx)
	sInitRec := httptest.NewRecorder()
	server.HandleUploadInit(sInitRec, sInitReq)
	var sInitResp UploadInitResponse
	_ = json.Unmarshal(sInitRec.Body.Bytes(), &sInitResp)

	for cIdx := 0; cIdx < 2; cIdx++ {
		cReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/%d", sInitResp.UploadID, cIdx), bytes.NewReader(streamData[cIdx*1024:(cIdx+1)*1024])).WithContext(adminCtx)
		cReq.SetPathValue("uploadId", sInitResp.UploadID)
		cReq.SetPathValue("chunkIndex", strconv.Itoa(cIdx))
		cRec := httptest.NewRecorder()
		server.HandleUploadChunk(cRec, cReq)
	}
	sFinReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/finalize", sInitResp.UploadID), nil).WithContext(adminCtx)
	sFinReq.SetPathValue("uploadId", sInitResp.UploadID)
	sFinRec := httptest.NewRecorder()
	server.HandleUploadFinalize(sFinRec, sFinReq)
	var sFile storage.File
	_ = json.Unmarshal(sFinRec.Body.Bytes(), &sFile)

	// Concurrently perform uploads and range streams
	for i := 0; i < stressUploads; i++ {
		go func(idx int) {
			defer wg.Done()
			uBody := UploadInitRequest{
				Name:      fmt.Sprintf("stress_up_%d.bin", idx),
				Size:      512,
				ChunkSize: 512,
			}
			bU, _ := json.Marshal(uBody)
			uReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(bU)).WithContext(adminCtx)
			uRec := httptest.NewRecorder()
			server.HandleUploadInit(uRec, uReq)
			var uResp UploadInitResponse
			_ = json.Unmarshal(uRec.Body.Bytes(), &uResp)

			cReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", uResp.UploadID), bytes.NewReader(make([]byte, 512))).WithContext(adminCtx)
			cReq.SetPathValue("uploadId", uResp.UploadID)
			cReq.SetPathValue("chunkIndex", "0")
			cRec := httptest.NewRecorder()
			server.HandleUploadChunk(cRec, cReq)

			fReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/finalize", uResp.UploadID), nil).WithContext(adminCtx)
			fReq.SetPathValue("uploadId", uResp.UploadID)
			fRec := httptest.NewRecorder()
			server.HandleUploadFinalize(fRec, fReq)
		}(i)
	}

	for j := 0; j < stressStreams; j++ {
		go func(idx int) {
			defer wg.Done()
			strReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/stream", sFile.ID), nil).WithContext(adminCtx)
			strReq.SetPathValue("id", sFile.ID)
			strReq.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", idx*50, idx*50+100))
			strRec := httptest.NewRecorder()
			server.HandleStreamFile(strRec, strReq)
		}(j)
	}

	wg.Wait()

	runtime.ReadMemStats(&endMem)
	cachedEntries, cachedBytes, _ := server.chunkCache.Stats()

	t.Logf("=== STRESS TEST TELEMETRY REPORT ===")
	t.Logf("Concurrent Operations: %d (uploads: %d, streams: %d)", stressUploads+stressStreams, stressUploads, stressStreams)
	t.Logf("Go Heap Alloc (start -> end): %d KB -> %d KB", startMem.HeapAlloc/1024, endMem.HeapAlloc/1024)
	t.Logf("Go Sys Memory: %d KB", endMem.Sys/1024)
	t.Logf("Decrypted LRU Cache Entries: %d, Bytes Occupied: %d KB", cachedEntries, cachedBytes/1024)
	t.Logf("Active Upload Semaphore: %d / %d", len(server.uploadSem), cap(server.uploadSem))
	t.Logf("Active Download Semaphore: %d / %d", len(server.downloadSem), cap(server.downloadSem))
}

func TestRealistic16MiBAtomicMemoryReservationAndStress(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	server, _, cleanup := setupTestServer(t, "test_api_16mib_mem.db", tgServer.URL)
	defer cleanup()

	adminCtx := WithUserContext(context.Background(), "admin")

	// 1. Verify Atomic Reservation prevents concurrent requests from exceeding budget
	const chunkSize16MiB = 16 * 1024 * 1024 // 16 MiB
	const estimatedBuffer = int64(chunkSize16MiB * 2) // 32 MiB per chunk in-flight
	// Set budget to allow exactly 2 simultaneous 16 MiB chunks (~64 MiB + current heap)
	var currentMem runtime.MemStats
	runtime.ReadMemStats(&currentMem)
	budget := int64(currentMem.HeapAlloc) + (estimatedBuffer * 2) + (4 * 1024 * 1024)
	server.cfg.MaxMemoryBudgetBytes = budget

	// Test direct Server.ReserveMemory concurrency safety
	rel1, err := server.ReserveMemory(estimatedBuffer)
	if err != nil {
		t.Fatalf("first reservation should succeed, got %v", err)
	}
	if inFlight := server.inFlightMemoryBytes.Load(); inFlight != estimatedBuffer {
		t.Fatalf("expected inFlightMemoryBytes=%d, got %d", estimatedBuffer, inFlight)
	}

	rel2, err := server.ReserveMemory(estimatedBuffer)
	if err != nil {
		t.Fatalf("second reservation should succeed, got %v", err)
	}
	if inFlight := server.inFlightMemoryBytes.Load(); inFlight != estimatedBuffer*2 {
		t.Fatalf("expected inFlightMemoryBytes=%d, got %d", estimatedBuffer*2, inFlight)
	}

	// Third simultaneous request MUST be rejected due to budget saturation
	_, err = server.ReserveMemory(estimatedBuffer)
	if err == nil {
		t.Fatalf("third reservation should be rejected when budget saturated")
	}
	if !strings.Contains(err.Error(), "memory budget exceeded") {
		t.Fatalf("expected memory budget exceeded error, got %v", err)
	}

	// Release first reservation: now a new request can be admitted
	rel1()
	if inFlight := server.inFlightMemoryBytes.Load(); inFlight != estimatedBuffer {
		t.Fatalf("expected inFlightMemoryBytes=%d after rel1, got %d", estimatedBuffer, inFlight)
	}

	rel3, err := server.ReserveMemory(estimatedBuffer)
	if err != nil {
		t.Fatalf("reservation after release should succeed, got %v", err)
	}
	rel2()
	rel3()
	if inFlight := server.inFlightMemoryBytes.Load(); inFlight != 0 {
		t.Fatalf("expected inFlightMemoryBytes=0 after all releases, got %d", inFlight)
	}

	// 2. Realistic 16 MiB Chunk Upload & Range Streaming under production-scale budget (384 MiB)
	server.cfg.MaxMemoryBudgetBytes = 384 * 1024 * 1024 // 384 MiB container budget
	server.cfg.DefaultChunkSize = chunkSize16MiB

	initBody := UploadInitRequest{
		Name:      "video_16mib_test.mp4",
		Size:      chunkSize16MiB,
		ChunkSize: chunkSize16MiB,
	}
	bInit, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(bInit)).WithContext(adminCtx)
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)
	if initRec.Code != http.StatusOK {
		t.Fatalf("init 16 MiB upload failed: %d - %s", initRec.Code, initRec.Body.String())
	}
	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)

	// Build deterministic 16 MiB chunk
	chunkPlaintext := make([]byte, chunkSize16MiB)
	for i := 0; i < chunkSize16MiB; i += 4096 {
		chunkPlaintext[i] = byte(i % 251)
	}

	chunkReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", initResp.UploadID), bytes.NewReader(chunkPlaintext)).WithContext(adminCtx)
	chunkReq.SetPathValue("uploadId", initResp.UploadID)
	chunkReq.SetPathValue("chunkIndex", "0")
	chunkRec := httptest.NewRecorder()
	server.HandleUploadChunk(chunkRec, chunkReq)
	if chunkRec.Code != http.StatusOK {
		t.Fatalf("upload 16 MiB chunk failed: %d - %s", chunkRec.Code, chunkRec.Body.String())
	}

	// Verify in-flight memory returned to 0 after upload chunk handler returned
	if inFlight := server.inFlightMemoryBytes.Load(); inFlight != 0 {
		t.Fatalf("expected inFlightMemoryBytes=0 after chunk upload completed, got %d", inFlight)
	}

	// Finalize 16 MiB upload session
	finReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/finalize", initResp.UploadID), nil).WithContext(adminCtx)
	finReq.SetPathValue("uploadId", initResp.UploadID)
	finRec := httptest.NewRecorder()
	server.HandleUploadFinalize(finRec, finReq)
	if finRec.Code != http.StatusOK {
		t.Fatalf("finalize 16 MiB upload failed: %d - %s", finRec.Code, finRec.Body.String())
	}
	var finFile storage.File
	_ = json.Unmarshal(finRec.Body.Bytes(), &finFile)

	// 3. Concurrent Range Streams on 16 MiB file
	const concurrentStreams = 4
	var streamWg sync.WaitGroup
	streamWg.Add(concurrentStreams)
	var rejectedCount int64

	for sIdx := 0; sIdx < concurrentStreams; sIdx++ {
		go func(idx int) {
			defer streamWg.Done()
			offsetStart := int64(idx * 1024 * 1024)
			offsetEnd := offsetStart + (1024 * 1024) - 1 // 1 MiB slice

			stReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/files/%s/stream", finFile.ID), nil).WithContext(adminCtx)
			stReq.SetPathValue("id", finFile.ID)
			stReq.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offsetStart, offsetEnd))
			stRec := httptest.NewRecorder()
			server.HandleStreamFile(stRec, stReq)

			if stRec.Code == http.StatusServiceUnavailable {
				atomic.AddInt64(&rejectedCount, 1)
			} else if stRec.Code != http.StatusPartialContent {
				t.Errorf("unexpected status %d for range stream %d", stRec.Code, idx)
			}
		}(sIdx)
	}
	streamWg.Wait()

	// 4. Memory Telemetry Inspection
	var finalMem runtime.MemStats
	runtime.ReadMemStats(&finalMem)
	cachedEntries, cachedBytes, maxCacheBytes := server.chunkCache.Stats()

	t.Logf("=== 16 MiB WORKLOAD TELEMETRY REPORT ===")
	t.Logf("Chunk Size: 16 MiB (%d bytes)", chunkSize16MiB)
	t.Logf("Configured Memory Budget: %d MiB", server.cfg.MaxMemoryBudgetBytes/(1024*1024))
	t.Logf("Peak Go Heap Alloc: %d MiB", finalMem.HeapAlloc/(1024*1024))
	t.Logf("Go Sys Memory: %d MiB", finalMem.Sys/(1024*1024))
	t.Logf("Decrypted LRU Cache: %d entries, %d MiB / %d MiB max", cachedEntries, cachedBytes/(1024*1024), maxCacheBytes/(1024*1024))
	t.Logf("In-Flight Reserved Bytes at rest: %d bytes", server.inFlightMemoryBytes.Load())
	t.Logf("Total Garbage Collections: %d", finalMem.NumGC)
	t.Logf("Rejected Requests: %d", atomic.LoadInt64(&rejectedCount))

	if inFlight := server.inFlightMemoryBytes.Load(); inFlight != 0 {
		t.Fatalf("memory reservation leak: in-flight bytes=%d", inFlight)
	}
}

func TestSameBotTokenRotationSurvival(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	dbFile := filepath.Join(t.TempDir(), "same_bot_rotation.db")
	server, db, cleanup := setupTestServerWithBots(t, dbFile, tgServer.URL, nil)
	defer cleanup()

	// Register Bot A with ID 11111
	botToken := "11111:ORIGINAL_TOKEN_SECRET"
	addReq := httptest.NewRequest("POST", "/api/bots", strings.NewReader(fmt.Sprintf(`{"token":"%s"}`, botToken)))
	addReq.Header.Set("Content-Type", "application/json")
	addReq = addReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	addRec := httptest.NewRecorder()
	server.HandleAddBot(addRec, addReq)
	if addRec.Code != http.StatusOK {
		t.Fatalf("AddBot failed: %s", addRec.Body.String())
	}
	origHash := crypto.HashToken(botToken)

	// Upload a file of 200 KB
	fileData := bytes.Repeat([]byte("SameBotDataBlock!"), 12000)
	initBody := UploadInitRequest{
		Name:      "test_same_bot.bin",
		Size:      int64(len(fileData)),
		MimeType:  "application/octet-stream",
		ChunkSize: len(fileData),
	}
	initBytes, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(initBytes))
	initReq = initReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)
	if initRec.Code != http.StatusOK {
		t.Fatalf("HandleUploadInit failed: %s", initRec.Body.String())
	}

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)
	uploadID := initResp.UploadID
	fileID := initResp.FileID

	// Upload chunk 0
	chunk0Req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", uploadID), bytes.NewReader(fileData))
	chunk0Req.SetPathValue("uploadId", uploadID)
	chunk0Req.SetPathValue("chunkIndex", "0")
	chunk0Req = chunk0Req.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	chunk0Rec := httptest.NewRecorder()
	server.HandleUploadChunk(chunk0Rec, chunk0Req)
	if chunk0Rec.Code != http.StatusOK {
		t.Fatalf("UploadChunk failed: %s", chunk0Rec.Body.String())
	}

	// Finalize upload
	finReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/finalize", uploadID), nil)
	finReq.SetPathValue("uploadId", uploadID)
	finReq = finReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	finRec := httptest.NewRecorder()
	server.HandleUploadFinalize(finRec, finReq)
	if finRec.Code != http.StatusOK {
		t.Fatalf("FinalizeUpload failed: %s", finRec.Body.String())
	}

	// Verify chunk in DB initially has origHash
	chunks, err := db.GetChunks(context.Background(), fileID)
	if err != nil || len(chunks) != 1 {
		t.Fatalf("failed to retrieve chunk from DB: %v", err)
	}
	if chunks[0].BotTokenHash != origHash {
		t.Fatalf("expected chunk bot hash %s, got %s", origHash, chunks[0].BotTokenHash)
	}

	// Rotate Bot A with a NEW token for the SAME bot ID (11111)
	rotatedToken := "11111:ROTATED_TOKEN_SECRET"
	rotReq := httptest.NewRequest("POST", fmt.Sprintf("/api/bots/%s/rotate", origHash), strings.NewReader(fmt.Sprintf(`{"token":"%s"}`, rotatedToken)))
	rotReq.SetPathValue("hash", origHash)
	rotReq.Header.Set("Content-Type", "application/json")
	rotReq = rotReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	rotRec := httptest.NewRecorder()
	server.HandleRotateBot(rotRec, rotReq)
	if rotRec.Code != http.StatusOK {
		t.Fatalf("RotateBot failed: %s", rotRec.Body.String())
	}

	var rotResp struct {
		Status       string `json:"status"`
		SameBot      bool   `json:"same_bot"`
		NewTokenHash string `json:"new_token_hash"`
	}
	_ = json.Unmarshal(rotRec.Body.Bytes(), &rotResp)
	if !rotResp.SameBot {
		t.Fatalf("expected same_bot = true for matching bot ID 11111")
	}

	// Verify chunk record in DB was updated in-place to new token hash
	rotatedHash := crypto.HashToken(rotatedToken)
	chunksAfter, err := db.GetChunks(context.Background(), fileID)
	if err != nil || len(chunksAfter) != 1 {
		t.Fatalf("failed to retrieve chunk after rotation: %v", err)
	}
	if chunksAfter[0].BotTokenHash != rotatedHash {
		t.Fatalf("expected chunk bot hash to be updated to %s, got %s", rotatedHash, chunksAfter[0].BotTokenHash)
	}

	// Verify file is still completely downloadable and decrypted without error
	dlReq := httptest.NewRequest("GET", fmt.Sprintf("/api/files/%s/download", fileID), nil)
	dlReq.SetPathValue("id", fileID)
	dlReq = dlReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	dlRec := httptest.NewRecorder()
	server.HandleDownloadFile(dlRec, dlReq)
	if dlRec.Code != http.StatusOK {
		t.Fatalf("Download after same-bot rotation failed (%d): %s", dlRec.Code, dlRec.Body.String())
	}
	if !bytes.Equal(dlRec.Body.Bytes(), fileData) {
		t.Fatalf("downloaded data mismatch after same-bot rotation")
	}
}

func TestDifferentBotReplacementAndResumableMigration(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	dbFile := filepath.Join(t.TempDir(), "diff_bot_migration.db")
	server, db, cleanup := setupTestServerWithBots(t, dbFile, tgServer.URL, nil)
	defer cleanup()

	// Register Bot A (ID 11111)
	botA := "11111:BOT_ALPHA_TOKEN"
	addReq := httptest.NewRequest("POST", "/api/bots", strings.NewReader(fmt.Sprintf(`{"token":"%s"}`, botA)))
	addReq.Header.Set("Content-Type", "application/json")
	addReq = addReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	addRec := httptest.NewRecorder()
	server.HandleAddBot(addRec, addReq)
	if addRec.Code != http.StatusOK {
		t.Fatalf("AddBot Alpha failed: %s", addRec.Body.String())
	}
	hashA := crypto.HashToken(botA)

	// Upload a file through Bot A
	fileData := bytes.Repeat([]byte("MigrationTestDataPayload!"), 8000)
	initBody := UploadInitRequest{
		Name:      "migration_test.bin",
		Size:      int64(len(fileData)),
		MimeType:  "application/octet-stream",
		ChunkSize: len(fileData),
	}
	initBytes, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(initBytes))
	initReq = initReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)
	if initRec.Code != http.StatusOK {
		t.Fatalf("HandleUploadInit failed: %s", initRec.Body.String())
	}

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)
	uploadID := initResp.UploadID
	fileID := initResp.FileID

	chunk0Req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", uploadID), bytes.NewReader(fileData))
	chunk0Req.SetPathValue("uploadId", uploadID)
	chunk0Req.SetPathValue("chunkIndex", "0")
	chunk0Req = chunk0Req.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	chunk0Rec := httptest.NewRecorder()
	server.HandleUploadChunk(chunk0Rec, chunk0Req)
	if chunk0Rec.Code != http.StatusOK {
		t.Fatalf("UploadChunk failed: %s", chunk0Rec.Body.String())
	}

	finReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/finalize", uploadID), nil)
	finReq.SetPathValue("uploadId", uploadID)
	finReq = finReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	finRec := httptest.NewRecorder()
	server.HandleUploadFinalize(finRec, finReq)
	if finRec.Code != http.StatusOK {
		t.Fatalf("FinalizeUpload failed: %s", finRec.Body.String())
	}

	// Replace Bot A with Bot B (different bot identity ID 22222)
	botB := "22222:BOT_BETA_TOKEN"
	rotReq := httptest.NewRequest("POST", fmt.Sprintf("/api/bots/%s/rotate", hashA), strings.NewReader(fmt.Sprintf(`{"token":"%s"}`, botB)))
	rotReq.SetPathValue("hash", hashA)
	rotReq.Header.Set("Content-Type", "application/json")
	rotReq = rotReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	rotRec := httptest.NewRecorder()
	server.HandleRotateBot(rotRec, rotReq)
	if rotRec.Code != http.StatusOK {
		t.Fatalf("RotateBot failed: %s", rotRec.Body.String())
	}

	var rotResp struct {
		Status       string `json:"status"`
		SameBot      bool   `json:"same_bot"`
		NewTokenHash string `json:"new_token_hash"`
	}
	_ = json.Unmarshal(rotRec.Body.Bytes(), &rotResp)
	if rotResp.SameBot {
		t.Fatalf("expected same_bot = false for different bot ID 22222")
	}
	hashB := rotResp.NewTokenHash


	// 1. Verify Bot A cannot be disabled without force=true because chunks depend on it
	disReq := httptest.NewRequest("POST", fmt.Sprintf("/api/bots/%s/disable", hashA), nil)
	disReq.SetPathValue("hash", hashA)
	disReq = disReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	disRec := httptest.NewRecorder()
	server.HandleDisableBot(disRec, disReq)
	if disRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when disabling bot with stored chunks, got %d", disRec.Code)
	}

	// 2. Verify Bot A cannot be removed without force=true because chunks depend on it
	remReq := httptest.NewRequest("DELETE", fmt.Sprintf("/api/bots/%s", hashA), nil)
	remReq.SetPathValue("hash", hashA)
	remReq = remReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	remRec := httptest.NewRecorder()
	server.HandleRemoveBot(remRec, remReq)
	if remRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when removing bot with stored chunks, got %d", remRec.Code)
	}

	// 3. Verify file can still be downloaded through preserved Bot A credentials
	dlReq := httptest.NewRequest("GET", fmt.Sprintf("/api/files/%s/download", fileID), nil)
	dlReq.SetPathValue("id", fileID)
	dlReq = dlReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	dlRec := httptest.NewRecorder()
	server.HandleDownloadFile(dlRec, dlReq)
	if dlRec.Code != http.StatusOK {
		t.Fatalf("download with preserved old bot failed (%d): %s", dlRec.Code, dlRec.Body.String())
	}
	if !bytes.Equal(dlRec.Body.Bytes(), fileData) {
		t.Fatalf("downloaded data mismatch")
	}

	// 4. Perform resumable chunk migration from Bot A to Bot B
	migReq := httptest.NewRequest("POST", fmt.Sprintf("/api/bots/%s/migrate", hashA), strings.NewReader(fmt.Sprintf(`{"target_bot_hash":"%s"}`, hashB)))
	migReq.SetPathValue("hash", hashA)
	migReq.Header.Set("Content-Type", "application/json")
	migReq = migReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	migRec := httptest.NewRecorder()
	server.HandleMigrateBotChunks(migRec, migReq)
	if migRec.Code != http.StatusOK {
		t.Fatalf("MigrateBotChunks failed: %s", migRec.Body.String())
	}

	var migResp struct {
		MigratedChunks int `json:"migrated_chunks"`
		FailedChunks   int `json:"failed_chunks"`
	}
	_ = json.Unmarshal(migRec.Body.Bytes(), &migResp)
	if migResp.MigratedChunks != 1 || migResp.FailedChunks != 0 {
		t.Fatalf("expected 1 migrated and 0 failed, got %d and %d", migResp.MigratedChunks, migResp.FailedChunks)
	}

	// Verify chunk in DB now references Bot B
	chunksMigrated, err := db.GetChunks(context.Background(), fileID)
	if err != nil || len(chunksMigrated) != 1 {
		t.Fatalf("failed to retrieve migrated chunk: %v", err)
	}
	if chunksMigrated[0].BotTokenHash != hashB {
		t.Fatalf("expected chunk bot hash to be %s after migration, got %s", hashB, chunksMigrated[0].BotTokenHash)
	}

	// 5. Test idempotency: second migration call reports 0 chunks pending
	migReq2 := httptest.NewRequest("POST", fmt.Sprintf("/api/bots/%s/migrate", hashA), strings.NewReader(fmt.Sprintf(`{"target_bot_hash":"%s"}`, hashB)))
	migReq2.SetPathValue("hash", hashA)
	migReq2.Header.Set("Content-Type", "application/json")
	migReq2 = migReq2.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	migRec2 := httptest.NewRecorder()
	server.HandleMigrateBotChunks(migRec2, migReq2)
	if migRec2.Code != http.StatusOK {
		t.Fatalf("Second migration failed: %s", migRec2.Body.String())
	}
	var migResp2 struct {
		Migrated int `json:"migrated"`
	}
	_ = json.Unmarshal(migRec2.Body.Bytes(), &migResp2)
	if migResp2.Migrated != 0 {
		t.Fatalf("expected 0 migrated on second run, got %d", migResp2.Migrated)
	}

	// 6. Now Bot A can be safely removed because 0 chunks depend on it
	remReq2 := httptest.NewRequest("DELETE", fmt.Sprintf("/api/bots/%s", hashA), nil)
	remReq2.SetPathValue("hash", hashA)
	remReq2 = remReq2.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	remRec2 := httptest.NewRecorder()
	server.HandleRemoveBot(remRec2, remReq2)
	if remRec2.Code != http.StatusOK {
		t.Fatalf("Removal of decommissioned bot failed: %s", remRec2.Body.String())
	}

	// 7. Verify file downloads cleanly using Bot B
	dlReq2 := httptest.NewRequest("GET", fmt.Sprintf("/api/files/%s/download", fileID), nil)
	dlReq2.SetPathValue("id", fileID)
	dlReq2 = dlReq2.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	dlRec2 := httptest.NewRecorder()
	server.HandleDownloadFile(dlRec2, dlReq2)
	if dlRec2.Code != http.StatusOK {
		t.Fatalf("Download via Bot B failed: %s", dlRec2.Body.String())
	}
	if !bytes.Equal(dlRec2.Body.Bytes(), fileData) {
		t.Fatalf("downloaded data mismatch after migration to Bot B")
	}
}

func TestContainerMemoryStatsEndpoint(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()


	dbFile := filepath.Join(t.TempDir(), "container_memory_test.db")
	server, _, cleanup := setupTestServer(t, dbFile, tgServer.URL)
	defer cleanup()

	// Non-admin request should receive 403 Forbidden
	nonAdminReq := httptest.NewRequest("GET", "/api/system/memory", nil)
	nonAdminReq = nonAdminReq.WithContext(context.WithValue(context.Background(), userContextKey, "student_user"))
	nonAdminRec := httptest.NewRecorder()
	server.HandleGetMemoryStats(nonAdminRec, nonAdminReq)
	if nonAdminRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for non-admin on memory stats, got %d", nonAdminRec.Code)
	}

	// Admin request returns detailed container and Go memory telemetry
	adminReq := httptest.NewRequest("GET", "/api/system/memory", nil)
	adminReq = adminReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	adminRec := httptest.NewRecorder()
	server.HandleGetMemoryStats(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin memory stats, got %d: %s", adminRec.Code, adminRec.Body.String())
	}

	var stats map[string]interface{}
	if err := json.Unmarshal(adminRec.Body.Bytes(), &stats); err != nil {
		t.Fatalf("unmarshal memory stats failed: %v", err)
	}

	// Validate required memory fields
	requiredKeys := []string{
		"process_rss_bytes",
		"peak_process_rss_bytes",
		"container_memory_bytes",
		"peak_container_memory_bytes",
		"cgroup_version",
		"cgroup_available",
		"rss_available",
		"go_alloc_bytes",
		"go_sys_bytes",
		"go_heap_alloc_bytes",
		"in_flight_bytes",
		"memory_budget_bytes",
		"rejected_requests",
		"cache_entries",
		"cache_used_bytes",
		"upload_sem_capacity",
		"download_sem_capacity",
	}

	for _, k := range requiredKeys {
		if _, ok := stats[k]; !ok {
			t.Fatalf("missing required key %q in memory stats response", k)
		}
	}
	t.Logf("Memory Stats response: %+v", stats)
}

func TestSecretManagementProductionRequirement(t *testing.T) {
	// 1. In production mode (ENV=production), BOT_CREDENTIAL_SECRET is strictly required
	t.Setenv("ENV", "production")
	t.Setenv("ALLOW_LOCAL_SQLITE", "true")
	t.Setenv("TG_BOT_TOKENS", "11111:TOKEN_A")
	t.Setenv("TG_CHANNEL_ID", "-1001234567890")
	t.Setenv("ENCRYPTION_SECRET", "super_master_encryption_secret_32bytes!")
	t.Setenv("ADMIN_PASSWORD", "strong_admin_password_123")
	t.Setenv("BOT_CREDENTIAL_SECRET", "")

	_, err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "BOT_CREDENTIAL_SECRET is required in production") {
		t.Fatalf("expected error requiring BOT_CREDENTIAL_SECRET in production, got: %v", err)
	}

	// 2. When BOT_CREDENTIAL_SECRET is provided in production, config loads successfully
	t.Setenv("BOT_CREDENTIAL_SECRET", "separate_bot_secret_32bytes_value!!")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load failed with valid BOT_CREDENTIAL_SECRET: %v", err)
	}
	if cfg.BotCredentialSecretIsFallback {
		t.Fatalf("expected BotCredentialSecretIsFallback = false when explicitly provided")
	}

	// 3. In non-production mode, fallback to ENCRYPTION_SECRET is allowed with documented fallback flag
	t.Setenv("ENV", "development")
	t.Setenv("REQUIRE_BOT_CREDENTIAL_SECRET", "false")
	t.Setenv("BOT_CREDENTIAL_SECRET", "")
	cfgDev, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load dev failed: %v", err)
	}
	if !cfgDev.BotCredentialSecretIsFallback {
		t.Fatalf("expected BotCredentialSecretIsFallback = true when falling back in dev")
	}
}

func TestDatabaseDurabilityProductionRequirement(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("TG_BOT_TOKENS", "11111:TOKEN_A")
	t.Setenv("TG_CHANNEL_ID", "-1001234567890")
	t.Setenv("ENCRYPTION_SECRET", "super_master_encryption_secret_32bytes!")
	t.Setenv("BOT_CREDENTIAL_SECRET", "separate_bot_secret_32bytes_value!!")
	t.Setenv("ADMIN_PASSWORD", "strong_admin_password_123")
	t.Setenv("TURSO_DATABASE_URL", "")
	t.Setenv("ALLOW_LOCAL_SQLITE", "")

	// 1. In production, missing Turso connection fails startup
	_, err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "production startup requires a persistent Turso database") {
		t.Fatalf("expected error requiring persistent Turso database in production, got: %v", err)
	}

	// 2. Setting ALLOW_LOCAL_SQLITE=true allows production startup with local SQLite (e.g. dedicated VM with volume)
	t.Setenv("ALLOW_LOCAL_SQLITE", "true")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected config.Load to succeed with ALLOW_LOCAL_SQLITE=true, got: %v", err)
	}
	if cfg.TursoDatabaseURL != "tgdrive.db" {
		t.Fatalf("unexpected db url: %s", cfg.TursoDatabaseURL)
	}

	// 3. Configuring Turso cloud URL without TURSO_AUTH_TOKEN fails
	t.Setenv("ALLOW_LOCAL_SQLITE", "")
	t.Setenv("TURSO_DATABASE_URL", "libsql://my-cluster.turso.io")
	t.Setenv("TURSO_AUTH_TOKEN", "")
	_, err = config.Load()
	if err == nil || !strings.Contains(err.Error(), "TURSO_AUTH_TOKEN is required") {
		t.Fatalf("expected error requiring TURSO_AUTH_TOKEN, got: %v", err)
	}

	// 4. Configuring both Turso URL and auth token succeeds
	t.Setenv("TURSO_AUTH_TOKEN", "my_secret_turso_jwt_auth_token")
	cfgTurso, err := config.Load()
	if err != nil {
		t.Fatalf("expected config.Load to succeed with Turso cloud config: %v", err)
	}
	if cfgTurso.TursoDatabaseURL != "libsql://my-cluster.turso.io" {
		t.Fatalf("unexpected turso db url: %s", cfgTurso.TursoDatabaseURL)
	}
}

func TestMigrationCrashRecoveryAndReconciliationInAPI(t *testing.T) {
	tgServer := setupMockTelegramServer()
	defer tgServer.Close()

	dbFile := filepath.Join(t.TempDir(), "migration_recovery_test.db")
	server, db, cleanup := setupTestServer(t, dbFile, tgServer.URL)
	defer cleanup()

	botTokenA := "12345:TOKEN_AAA_MIGRATION"
	hashA := crypto.HashToken(botTokenA)
	botTokenB := "99999:TOKEN_BBB_REPLACEMENT"
	hashB := crypto.HashToken(botTokenB)

	// Register bot A
	if err := db.SaveBot(context.Background(), &storage.RegisteredBot{
		TokenHash: hashA,
		OwnerID:   "admin",
		Token:     botTokenA,
	}); err != nil {
		t.Fatalf("SaveBot A: %v", err)
	}

	// Register bot B
	if err := db.SaveBot(context.Background(), &storage.RegisteredBot{
		TokenHash: hashB,
		OwnerID:   "admin",
		Token:     botTokenB,
	}); err != nil {
		t.Fatalf("SaveBot B: %v", err)
	}

	// Upload a file through Bot A
	fileData := []byte("critical persistent content to test migration crash reconciliation")
	initBody := UploadInitRequest{
		Name:      "recovery_doc.txt",
		Size:      int64(len(fileData)),
		MimeType:  "text/plain",
		ChunkSize: len(fileData),
	}
	initBytes, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/init", bytes.NewReader(initBytes))
	initReq = initReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	initRec := httptest.NewRecorder()
	server.HandleUploadInit(initRec, initReq)
	if initRec.Code != http.StatusOK {
		t.Fatalf("HandleUploadInit failed: %s", initRec.Body.String())
	}

	var initResp UploadInitResponse
	_ = json.Unmarshal(initRec.Body.Bytes(), &initResp)
	uploadID := initResp.UploadID
	fileID := initResp.FileID

	chunk0Req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/files/upload/%s/chunks/0", uploadID), bytes.NewReader(fileData))
	chunk0Req.SetPathValue("uploadId", uploadID)
	chunk0Req.SetPathValue("chunkIndex", "0")
	chunk0Req = chunk0Req.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	chunk0Rec := httptest.NewRecorder()
	server.HandleUploadChunk(chunk0Rec, chunk0Req)
	if chunk0Rec.Code != http.StatusOK {
		t.Fatalf("UploadChunk failed: %s", chunk0Rec.Body.String())
	}

	finReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/files/upload/%s/finalize", uploadID), nil)
	finReq.SetPathValue("uploadId", uploadID)
	finReq = finReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	finRec := httptest.NewRecorder()
	server.HandleUploadFinalize(finRec, finReq)
	if finRec.Code != http.StatusOK {
		t.Fatalf("FinalizeUpload failed: %s", finRec.Body.String())
	}

	chunksBefore, err := db.GetChunks(context.Background(), fileID)
	if err != nil || len(chunksBefore) != 1 {
		t.Fatalf("failed to retrieve chunks before migration: %v", err)
	}
	chunk := chunksBefore[0]
	sourceHash := chunk.BotTokenHash

	// Register source bot in registered_bots
	if err := db.SaveBot(context.Background(), &storage.RegisteredBot{
		TokenHash: sourceHash,
		OwnerID:   "admin",
		Token:     "test_bot_token_12345",
	}); err != nil {
		t.Fatalf("SaveBot source: %v", err)
	}

	// Simulate crash state: Bot B already uploaded destination ciphertext to Telegram,
	// staged in chunk_migrations, but DB chunks commit had not completed before crash/restart
	stagedFileID := "tg_staged_destination_file_id_1234"
	stagedMsgID := 77777
	if err := db.RecordChunkMigration(context.Background(), chunk.ID, sourceHash, hashB, stagedFileID, stagedMsgID); err != nil {
		t.Fatalf("RecordChunkMigration failed: %v", err)
	}

	// Now trigger HandleMigrateBotChunks. It should detect the staged record, reconcile
	// and commit stagedFileID and stagedMsgID into chunks without downloading/re-uploading!
	migReq := httptest.NewRequest("POST", fmt.Sprintf("/api/bots/%s/migrate", sourceHash), strings.NewReader(fmt.Sprintf(`{"target_bot_hash":"%s"}`, hashB)))
	migReq.SetPathValue("hash", sourceHash)
	migReq.Header.Set("Content-Type", "application/json")
	migReq = migReq.WithContext(context.WithValue(context.Background(), userContextKey, "admin"))
	migRec := httptest.NewRecorder()
	server.HandleMigrateBotChunks(migRec, migReq)

	if migRec.Code != http.StatusOK {
		t.Fatalf("MigrateBotChunks reconciliation failed: %s", migRec.Body.String())
	}

	// Verify chunk in DB was updated to the staged references
	chunksAfter, err := db.GetChunks(context.Background(), fileID)
	if err != nil || len(chunksAfter) != 1 {
		t.Fatalf("failed to retrieve chunks after migration: %v", err)
	}
	if chunksAfter[0].TelegramFileID != stagedFileID || chunksAfter[0].TelegramMsgID != stagedMsgID || chunksAfter[0].BotTokenHash != hashB {
		t.Fatalf("expected chunk to reconcile with staged destination values, got: %+v", chunksAfter[0])
	}

	// Verify staging record was cleaned up
	pending, err := db.GetPendingChunkMigration(context.Background(), chunk.ID)
	if err != nil || pending != nil {
		t.Fatalf("expected pending migration record to be cleaned up after reconciliation, got: %+v", pending)
	}
}




