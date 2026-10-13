package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"tgdrive/crypto"
	"tgdrive/storage"

	"github.com/google/uuid"
)

// UploadInitRequest defines parameters sent to initialize a chunked upload session.
type UploadInitRequest struct {
	Name              string   `json:"name"`
	Size              int64    `json:"size"`
	MimeType          string   `json:"mime_type"`
	FolderID          *string  `json:"folder_id,omitempty"`
	ChunkSize         int      `json:"chunk_size"`
	SourceFingerprint string   `json:"source_fingerprint"`
	Duration          *float64 `json:"duration,omitempty"`
	Width             *int     `json:"width,omitempty"`
	Height            *int     `json:"height,omitempty"`
	ThumbnailBase64   *string  `json:"thumbnail_base64,omitempty"`
}

// UploadInitResponse returns the created upload session details.
type UploadInitResponse struct {
	UploadID        string `json:"upload_id"`
	FileID          string `json:"file_id"`
	ChunkSize       int    `json:"chunk_size"`
	TotalChunks     int    `json:"total_chunks"`
	Status          string `json:"status"`
	SafeConcurrency int    `json:"safe_concurrency"`
}

// UploadStatusResponse returns progress and confirmed chunk indices with SHA-256 integrity digests.
type UploadStatusResponse struct {
	UploadID          string         `json:"upload_id"`
	FileID            string         `json:"file_id"`
	Name              string         `json:"name"`
	Size              int64          `json:"size"`
	ChunkSize         int            `json:"chunk_size"`
	TotalChunks       int            `json:"total_chunks"`
	Status            string         `json:"status"`
	SourceFingerprint string         `json:"source_fingerprint"`
	ConfirmedChunks   []int          `json:"confirmed_chunks"`
	ConfirmedCount    int            `json:"confirmed_count"`
	ChunkHashes       map[int]string `json:"chunk_hashes"`
	Progress          float64        `json:"progress"`
	ErrorMessage      *string        `json:"error_message,omitempty"`
}

// HandleUploadInit handles POST /api/files/upload/init
func (s *Server) HandleUploadInit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req UploadInitRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json payload"})
		return
	}

	cleanName := filepath.Base(filepath.Clean(req.Name))
	cleanName = strings.ReplaceAll(cleanName, "\r", "")
	cleanName = strings.ReplaceAll(cleanName, "\n", "")
	cleanName = strings.ReplaceAll(cleanName, `"`, "")
	if cleanName == "" || cleanName == "." {
		cleanName = "unnamed_file"
	}

	if req.Size < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file size cannot be negative"})
		return
	}

	mimeType := detectMimeType(cleanName, req.MimeType)

	// Validate chunk size: default to configured DefaultChunkSize (16MiB), bounded between 1MiB and MaxChunkSize (20MiB)
	chunkSize := req.ChunkSize
	if chunkSize <= 0 || chunkSize > crypto.MaxChunkSize {
		chunkSize = s.cfg.DefaultChunkSize
	} else if chunkSize < 1024*1024 && req.Size > 1024*1024 {
		chunkSize = 1024 * 1024
	}

	// Calculate total chunks
	var totalChunks int
	if req.Size == 0 {
		totalChunks = 1
	} else {
		totalChunks = int(math.Ceil(float64(req.Size) / float64(chunkSize)))
	}

	salt, err := crypto.GenerateSalt()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate salt"})
		return
	}
	saltHex := crypto.HexEncode(salt)

	uploadID := uuid.New().String()
	fileID := uuid.New().String()

	callerOwner := GetUserFromContext(ctx)

	// Handle optional thumbnail
	var tFileID, tIV, tAuthTag *string
	if req.ThumbnailBase64 != nil && *req.ThumbnailBase64 != "" {
		thumbBytes, decErr := base64.StdEncoding.DecodeString(*req.ThumbnailBase64)
		if decErr == nil && len(thumbBytes) > 0 && len(thumbBytes) <= 5*1024*1024 {
			masterKey := []byte(s.cfg.MasterKey)
			derivedKey := crypto.DeriveKey(masterKey, salt)
			iv, ivErr := crypto.GenerateIV()
			if ivErr == nil {
				ciphertext, tag, encErr := crypto.EncryptChunk(derivedKey, iv, thumbBytes)
				if encErr == nil {
					res, upErr := s.tg.UploadChunk(ctx, callerOwner, ciphertext, fileID+".thumb")
					if upErr == nil {
						tf := res.FileID
						ivStr := crypto.HexEncode(iv)
						tagStr := crypto.HexEncode(tag)
						tFileID = &tf
						tIV = &ivStr
						tAuthTag = &tagStr
					} else {
						log.Printf("WARN: failed to upload thumbnail to telegram: %v", upErr)
					}
				}
			}
		}
	}

	session := &storage.UploadSession{
		ID:                      uploadID,
		FileID:                  fileID,
		Name:                    cleanName,
		Size:                    req.Size,
		MimeType:                mimeType,
		FolderID:                req.FolderID,
		EncryptionSalt:          saltHex,
		ChunkSize:               chunkSize,
		TotalChunks:             totalChunks,
		Status:                  "uploading",
		SourceFingerprint:       req.SourceFingerprint,
		OwnerID:                 callerOwner,
		Duration:                req.Duration,
		Width:                   req.Width,
		Height:                  req.Height,
		ThumbnailTelegramFileID: tFileID,
		ThumbnailIV:             tIV,
		ThumbnailAuthTag:        tAuthTag,
	}

	if err := s.db.CreateUploadSession(ctx, session); err != nil {
		log.Printf("ERROR: create upload session: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upload session"})
		return
	}

	writeJSON(w, http.StatusOK, UploadInitResponse{
		UploadID:        uploadID,
		FileID:          fileID,
		ChunkSize:       chunkSize,
		TotalChunks:     totalChunks,
		Status:          "uploading",
		SafeConcurrency: s.tg.GetSafeCapacityForOwner(callerOwner),
	})
}

// HandleUploadChunk handles PUT and POST /api/files/upload/{uploadId}/chunks/{chunkIndex}
func (s *Server) HandleUploadChunk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	uploadID := r.PathValue("uploadId")
	chunkIndexStr := r.PathValue("chunkIndex")
	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil || chunkIndex < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid chunk index"})
		return
	}

	// Atomic per-chunk coordination: ensures only ONE in-flight upload occurs for this exact chunk index
	unlock := s.chunkLockMgr.Acquire(uploadID, chunkIndex)
	defer unlock()

	session, err := s.db.GetUploadSession(ctx, uploadID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}
	if session == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "upload session not found"})
		return
	}
	if session.Status != "uploading" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("upload session is %s", session.Status)})
		return
	}

	// Strict session ownership check
	callerID := GetUserFromContext(ctx)
	if session.OwnerID != "" && session.OwnerID != callerID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized session access"})
		return
	}

	if chunkIndex >= session.TotalChunks {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("chunk index %d out of bounds (total %d)", chunkIndex, session.TotalChunks)})
		return
	}

	// Idempotency check: if chunk is already confirmed, return success immediately
	existingChunk, err := s.db.GetUploadSessionChunk(ctx, uploadID, chunkIndex)
	if err == nil && existingChunk != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"upload_id":   uploadID,
			"chunk_index": chunkIndex,
			"status":      "already_confirmed",
		})
		return
	}

	// Calculate expected chunk length
	var expectedSize int64
	if session.Size == 0 {
		expectedSize = 0
	} else if chunkIndex == session.TotalChunks-1 {
		expectedSize = session.Size - int64(chunkIndex*session.ChunkSize)
		if expectedSize < 0 {
			expectedSize = 0
		}
	} else {
		expectedSize = int64(session.ChunkSize)
	}

	// Strictly bound request body with MaxBytesReader to prevent unbounded body reads
	r.Body = http.MaxBytesReader(w, r.Body, expectedSize+1024*1024)

	// Enforce memory admission control with atomic in-flight reservation
	releaseMem, err := s.ReserveMemory(expectedSize * 2)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "server memory budget exceeded; please retry"})
		return
	}
	defer releaseMem()

	// Enforce global upload semaphore to bound peak memory usage
	select {
	case s.uploadSem <- struct{}{}:
		defer func() { <-s.uploadSem }()
	case <-ctx.Done():
		writeJSON(w, http.StatusRequestTimeout, map[string]string{"error": "upload timed out waiting for concurrency slot"})
		return
	}

	// Read strictly bounded chunk body into memory
	var rawData []byte
	if expectedSize > 0 {
		limitedReader := io.LimitReader(r.Body, expectedSize+1)
		buf := make([]byte, expectedSize+1)
		n, readErr := io.ReadFull(limitedReader, buf)
		if int64(n) > expectedSize {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "chunk payload exceeds expected size"})
			return
		}
		if int64(n) < expectedSize && readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("failed to read chunk body: %v", readErr)})
			return
		}
		if int64(n) != expectedSize {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("chunk size mismatch: expected %d bytes, got %d bytes", expectedSize, n)})
			return
		}
		rawData = buf[:n]
	} else {
		rawData = []byte{}
	}

	// Compute plaintext SHA-256 integrity digest
	shaSum := sha256.Sum256(rawData)
	chunkSha256 := hex.EncodeToString(shaSum[:])

	// If client provided X-Chunk-SHA256 header, enforce cryptographic match
	if clientSha := r.Header.Get("X-Chunk-SHA256"); clientSha != "" {
		if subtle.ConstantTimeCompare([]byte(strings.ToLower(clientSha)), []byte(strings.ToLower(chunkSha256))) != 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "chunk integrity error: SHA-256 checksum mismatch"})
			return
		}
	}

	// Encrypt chunk
	masterKey := []byte(s.cfg.MasterKey)
	salt, err := crypto.HexDecode(session.EncryptionSalt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid session salt"})
		return
	}

	derivedKey := crypto.DeriveKey(masterKey, salt)
	iv, err := crypto.GenerateIV()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate IV"})
		return
	}

	rawSize := len(rawData)
	ciphertext, authTag, err := crypto.EncryptChunk(derivedKey, iv, rawData)
	rawData = nil // Free plaintext buffer immediately to prevent duplicate buffer retention
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("encryption failed: %v", err)})
		return
	}

	// Upload chunk to Telegram via owner-allocated BotPool worker
	chunkFilename := fmt.Sprintf("%s.chunk.%d", session.FileID, chunkIndex)
	res, err := s.tg.UploadChunk(ctx, session.OwnerID, ciphertext, chunkFilename)
	cipherLen := len(ciphertext)
	ciphertext = nil // Free ciphertext buffer immediately
	if err != nil {
		log.Printf("ERROR: upload chunk %d for session %s: %v", chunkIndex, uploadID, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("telegram upload failed: %v", err)})
		return
	}

	// Save chunk metadata durably in DB
	sessionChunk := &storage.UploadSessionChunk{
		SessionID:      uploadID,
		ChunkIndex:     chunkIndex,
		TelegramMsgID:  res.MessageID,
		TelegramFileID: res.FileID,
		BotTokenHash:   res.BotTokenHash,
		Size:           cipherLen,
		RawSize:        rawSize,
		IV:             crypto.HexEncode(iv),
		AuthTag:        crypto.HexEncode(authTag),
		SHA256Hash:     chunkSha256,
	}

	if err := s.db.SaveUploadSessionChunk(ctx, sessionChunk); err != nil {
		log.Printf("ERROR: save upload chunk metadata: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record chunk in database"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"upload_id":   uploadID,
		"chunk_index": chunkIndex,
		"status":      "confirmed",
		"sha256":      chunkSha256,
	})
}

// HandleUploadStatus handles GET /api/files/upload/{uploadId}/status
func (s *Server) HandleUploadStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uploadID := r.PathValue("uploadId")

	session, err := s.db.GetUploadSession(ctx, uploadID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}
	if session == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "upload session not found"})
		return
	}

	callerID := GetUserFromContext(ctx)
	if session.OwnerID != "" && session.OwnerID != callerID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized session access"})
		return
	}

	confirmedIndices, err := s.db.GetConfirmedChunkIndices(ctx, uploadID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}
	if confirmedIndices == nil {
		confirmedIndices = []int{}
	}

	chunkHashes, err := s.db.GetConfirmedChunkHashes(ctx, uploadID)
	if err != nil || chunkHashes == nil {
		chunkHashes = make(map[int]string)
	}

	var progress float64
	if session.TotalChunks > 0 {
		progress = float64(len(confirmedIndices)) / float64(session.TotalChunks)
	}

	writeJSON(w, http.StatusOK, UploadStatusResponse{
		UploadID:          session.ID,
		FileID:            session.FileID,
		Name:              session.Name,
		Size:              session.Size,
		ChunkSize:         session.ChunkSize,
		TotalChunks:       session.TotalChunks,
		Status:            session.Status,
		SourceFingerprint: session.SourceFingerprint,
		ConfirmedChunks:   confirmedIndices,
		ConfirmedCount:    len(confirmedIndices),
		ChunkHashes:       chunkHashes,
		Progress:          progress,
		ErrorMessage:      session.ErrorMessage,
	})
}

// HandleUploadFinalize handles POST /api/files/upload/{uploadId}/finalize
func (s *Server) HandleUploadFinalize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uploadID := r.PathValue("uploadId")

	defer s.chunkLockMgr.ClearSession(uploadID)

	session, err := s.db.GetUploadSession(ctx, uploadID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}
	if session == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "upload session not found"})
		return
	}
	callerID := GetUserFromContext(ctx)
	if session.OwnerID != "" && session.OwnerID != callerID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized session access"})
		return
	}

	file, err := s.db.FinalizeUploadSession(ctx, uploadID)
	if err != nil {
		log.Printf("WARN: finalize session %s failed: %v", uploadID, err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, file)
}

// HandleUploadCancel handles POST /api/files/upload/{uploadId}/cancel and DELETE /api/files/upload/{uploadId}
func (s *Server) HandleUploadCancel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uploadID := r.PathValue("uploadId")

	defer s.chunkLockMgr.ClearSession(uploadID)

	session, err := s.db.GetUploadSession(ctx, uploadID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}
	if session == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "upload session not found"})
		return
	}
	callerID := GetUserFromContext(ctx)
	if session.OwnerID != "" && session.OwnerID != callerID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized session access"})
		return
	}

	msgIDs, err := s.db.CancelUploadSession(ctx, uploadID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to cancel session"})
		return
	}

	// Delete uploaded Telegram chunks asynchronously in background
	if len(msgIDs) > 0 {
		go func(ids []int) {
			for _, id := range ids {
				_ = s.tg.DeleteMessage(context.Background(), id)
			}
		}(msgIDs)
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"upload_id": uploadID,
		"status":    "cancelled",
	})
}

// HandleListBots handles GET /api/bots, returning only bots belonging to the caller.
func (s *Server) HandleListBots(w http.ResponseWriter, r *http.Request) {
	callerID := GetUserFromContext(r.Context())
	statuses := s.tg.GetBotStatusesForOwner(callerID)
	total, online := s.tg.GetStatsForOwner(callerID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total":   total,
		"online":  online,
		"workers": statuses,
	})
}

// HandleAddBot handles POST /api/bots, binding the newly registered bot to the authenticated caller.
func (s *Server) HandleAddBot(w http.ResponseWriter, r *http.Request) {
	callerID := GetUserFromContext(r.Context())
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&req); err != nil || strings.TrimSpace(req.Token) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bot token required"})
		return
	}

	if err := s.tg.AddBot(callerID, req.Token); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	hash := crypto.HashToken(req.Token)
	if err := s.db.SaveBot(r.Context(), &storage.RegisteredBot{
		TokenHash: hash,
		OwnerID:   callerID,
		Token:     req.Token,
		Disabled:  false,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to persist registered bot"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "bot registered successfully",
		"token_hash": hash,
	})
}

// HandleRemoveBot handles DELETE /api/bots/{hash}, strictly enforcing caller ownership.
func (s *Server) HandleRemoveBot(w http.ResponseWriter, r *http.Request) {
	callerID := GetUserFromContext(r.Context())
	tokenHash := r.PathValue("hash")
	if tokenHash == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token hash required"})
		return
	}

	// Safety check: Prevent bot removal from breaking access to existing file chunks
	chunkCount, err := s.db.CountChunksForBot(r.Context(), tokenHash)
	if err == nil && chunkCount > 0 && r.URL.Query().Get("force") != "true" {
		displayHash := tokenHash
		if len(displayHash) > 8 {
			displayHash = displayHash[:8]
		}
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"error":                 fmt.Sprintf("bot %s owns %d stored file chunks; removing it will break access to those files. Pass ?force=true to acknowledge data loss or disable the bot instead", displayHash, chunkCount),
			"affected_chunks":       chunkCount,
			"requires_confirmation": true,
		})
		return
	}

	if err := s.tg.RemoveBot(callerID, tokenHash); err != nil {
		if strings.Contains(err.Error(), "unauthorized") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	_ = s.db.DeleteBot(r.Context(), callerID, tokenHash)

	writeJSON(w, http.StatusOK, map[string]string{"status": "bot removed successfully"})
}

// HandleDisableBot handles POST /api/bots/{hash}/disable, strictly enforcing caller ownership.
func (s *Server) HandleDisableBot(w http.ResponseWriter, r *http.Request) {
	callerID := GetUserFromContext(r.Context())
	tokenHash := r.PathValue("hash")
	if tokenHash == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token hash required"})
		return
	}

	// Safety check: verify if disabling the only download-capable credential for stored chunks
	chunkCount, err := s.db.CountChunksForBot(r.Context(), tokenHash)
	if err == nil && chunkCount > 0 && r.URL.Query().Get("force") != "true" {
		displayHash := tokenHash
		if len(displayHash) > 8 {
			displayHash = displayHash[:8]
		}
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"error":                 fmt.Sprintf("cannot disable bot %s: it is the only download-capable credential for %d stored chunks. Migrate chunks to another active bot with POST /api/bots/{hash}/migrate first, or pass ?force=true to disable uploads while preserving download capability in the bot pool", displayHash, chunkCount),
			"affected_chunks":       chunkCount,
			"requires_confirmation": true,
		})
		return
	}

	if err := s.tg.DisableBot(callerID, tokenHash); err != nil {
		if strings.Contains(err.Error(), "unauthorized") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	_ = s.db.SetBotDisabled(r.Context(), callerID, tokenHash, true)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":           "bot disabled successfully",
		"download_capable": chunkCount > 0,
		"note":             "bot will no longer be selected for new uploads, but existing chunks remain downloadable",
	})
}

// HandleRotateBot handles POST /api/bots/{hash}/rotate, rotating bot credentials for the authenticated caller.
func (s *Server) HandleRotateBot(w http.ResponseWriter, r *http.Request) {
	callerID := GetUserFromContext(r.Context())
	oldHash := r.PathValue("hash")
	if oldHash == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token hash required"})
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&req); err != nil || strings.TrimSpace(req.Token) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "new bot token required"})
		return
	}

	isSameBot, err := s.tg.RotateBot(callerID, oldHash, req.Token)
	if err != nil {
		if strings.Contains(err.Error(), "unauthorized") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	newHash := crypto.HashToken(req.Token)

	if isSameBot {
		// Same bot identity:
		// 1. Save new bot credential
		_ = s.db.SaveBot(r.Context(), &storage.RegisteredBot{
			TokenHash: newHash,
			OwnerID:   callerID,
			Token:     req.Token,
			Disabled:  false,
		})
		// 2. Remove old bot row if hash changed
		if oldHash != newHash {
			_ = s.db.DeleteBot(r.Context(), callerID, oldHash)
			// 3. Atomically update chunk references to new hash since Telegram file_ids remain valid for same bot identity
			_ = s.db.UpdateChunkBotTokenHash(r.Context(), oldHash, newHash)
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":         "bot token rotated in-place (same bot identity)",
			"same_bot":       true,
			"new_token_hash": newHash,
		})
		return
	}

	// Different bot identity:
	// 1. Save new bot credential
	_ = s.db.SaveBot(r.Context(), &storage.RegisteredBot{
		TokenHash: newHash,
		OwnerID:   callerID,
		Token:     req.Token,
		Disabled:  false,
	})
	// 2. Preserve old bot credentials in database (marked disabled) so existing chunks remain downloadable!
	_ = s.db.SetBotDisabled(r.Context(), callerID, oldHash, true)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":         "bot replaced with new identity; previous bot retained for existing chunks",
		"same_bot":       false,
		"new_token_hash": newHash,
		"migration_tip":  "call POST /api/bots/{hash}/migrate to re-upload chunks to the new bot if you wish to decommission the old bot",
	})
}

// HandleMigrateBotChunks handles POST /api/bots/{hash}/migrate.
// Resumably and idempotently migrates stored chunks from the source bot to a target bot:
// 1. Validates caller ownership of source bot and target bot.
// 2. Finds all chunks in the database referencing the source bot.
// 3. For each chunk:
//    a. Downloads raw ciphertext using source bot.
//    b. Uploads ciphertext using target bot.
//    c. Updates the chunk's telegram_file_id, telegram_msg_id, and bot_token_hash in SQLite only AFTER upload confirms.
//    d. Deletes old Telegram message only after DB update succeeds.
func (s *Server) HandleMigrateBotChunks(w http.ResponseWriter, r *http.Request) {
	callerID := GetUserFromContext(r.Context())
	sourceHash := r.PathValue("hash")
	if sourceHash == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source bot token hash required"})
		return
	}

	var req struct {
		TargetBotHash string `json:"target_bot_hash"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&req)

	// Verify source bot exists and belongs to caller
	sourceBot, err := s.db.GetBot(r.Context(), sourceHash)
	if err != nil || sourceBot == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "source bot not found"})
		return
	}
	if sourceBot.OwnerID != callerID && callerID != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized: source bot belongs to another user"})
		return
	}

	// Determine target bot
	targetHash := strings.TrimSpace(req.TargetBotHash)
	if targetHash == "" {
		// Find first active bot for caller that is not the source bot
		callerBots, err := s.db.GetBotsByOwner(r.Context(), callerID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		for _, b := range callerBots {
			if b.TokenHash != sourceHash && !b.Disabled {
				targetHash = b.TokenHash
				break
			}
		}
		if targetHash == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no active target bot found for migration; please provide target_bot_hash"})
			return
		}
	} else {
		targetBot, err := s.db.GetBot(r.Context(), targetHash)
		if err != nil || targetBot == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target bot not found"})
			return
		}
		if targetBot.OwnerID != callerID && callerID != "admin" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized: target bot belongs to another user"})
			return
		}
		if targetBot.Disabled {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target bot is disabled"})
			return
		}
	}

	if sourceHash == targetHash {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source and target bots must be different"})
		return
	}

	// Fetch all chunks referencing source bot
	chunks, err := s.db.GetChunksByBotTokenHash(r.Context(), sourceHash)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("fetch chunks: %v", err)})
		return
	}

	if len(chunks) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":   "no chunks pending migration for this bot",
			"migrated": 0,
		})
		return
	}

	ctx := r.Context()
	migratedCount := 0
	failedCount := 0

	for _, chunk := range chunks {
		if ctx.Err() != nil {
			break
		}

		// Reconciliation check: check if destination upload succeeded previously but DB update/commit timed out or crashed
		pending, err := s.db.GetPendingChunkMigration(ctx, chunk.ID)
		if err == nil && pending != nil && pending.TargetBotHash == targetHash {
			// A previous destination upload succeeded! Reconcile by attempting to commit the existing destination reference.
			err = s.db.UpdateChunkTelegramRef(ctx, chunk.ID, pending.TargetFileID, pending.TargetMsgID, targetHash)
			if err == nil {
				_ = s.db.DeleteChunkMigration(ctx, chunk.ID)
				migratedCount++
				// Never delete source message until destination reference is committed and verified
				_ = s.tg.DeleteChunkMessage(ctx, sourceHash, chunk.TelegramMsgID)
				continue
			}
			log.Printf("ERROR: migrate chunk %s DB reconciliation failed: %v", chunk.ID, err)
			failedCount++
			continue
		}

		// If an obsolete pending record for a different target bot exists, delete the stale destination Telegram message first
		if pending != nil && pending.TargetBotHash != targetHash {
			_ = s.tg.DeleteChunkMessage(ctx, pending.TargetBotHash, pending.TargetMsgID)
			_ = s.db.DeleteChunkMigration(ctx, chunk.ID)
		}

		// 1. Download ciphertext with source bot
		ciphertext, err := s.tg.DownloadChunk(ctx, chunk.TelegramFileID, chunk.BotTokenHash)
		if err != nil {
			log.Printf("ERROR: migrate chunk %s download failed: %v", chunk.ID, err)
			failedCount++
			continue
		}

		// 2. Upload ciphertext with target bot
		filename := fmt.Sprintf("%s.chunk.%d", chunk.FileID, chunk.ChunkIndex)
		uploadRes, err := s.tg.UploadChunkWithBot(ctx, targetHash, ciphertext, filename)
		if err != nil {
			log.Printf("ERROR: migrate chunk %s upload failed: %v", chunk.ID, err)
			failedCount++
			continue
		}

		// Persist staging migration state immediately so retries do not orphan this message if DB update times out/fails
		if recErr := s.db.RecordChunkMigration(ctx, chunk.ID, sourceHash, targetHash, uploadRes.FileID, uploadRes.MessageID); recErr != nil {
			log.Printf("WARN: record chunk migration %s: %v", chunk.ID, recErr)
		}

		// 3. Atomically update chunk in DB ONLY AFTER verified upload success
		err = s.db.UpdateChunkTelegramRef(ctx, chunk.ID, uploadRes.FileID, uploadRes.MessageID, targetHash)
		if err != nil {
			log.Printf("ERROR: migrate chunk %s DB update failed: %v", chunk.ID, err)
			failedCount++
			continue
		}

		// DB update confirmed! Remove migration staging record
		_ = s.db.DeleteChunkMigration(ctx, chunk.ID)
		migratedCount++

		// 4. Delete old source Telegram message ONLY after destination reference is verified and committed
		_ = s.tg.DeleteChunkMessage(ctx, sourceHash, chunk.TelegramMsgID)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":           "migration finished",
		"migrated_chunks":  migratedCount,
		"failed_chunks":    failedCount,
		"remaining_chunks": len(chunks) - migratedCount,
		"target_bot_hash":  targetHash,
	})
}

