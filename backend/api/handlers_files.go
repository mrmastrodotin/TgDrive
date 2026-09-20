package api

import (
	"context"
	"fmt"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"tgdrive/crypto"
)

// HandleUploadFile handles POST /api/files/upload
func (s *Server) HandleUploadFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse multipart form"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file field required"})
		return
	}
	defer file.Close()

	cleanName := filepath.Base(filepath.Clean(header.Filename))
	cleanName = strings.ReplaceAll(cleanName, "\r", "")
	cleanName = strings.ReplaceAll(cleanName, "\n", "")
	cleanName = strings.ReplaceAll(cleanName, `"`, "")
	if cleanName == "" || cleanName == "." {
		cleanName = "unnamed_file"
	}

	folderID := r.FormValue("folder_id")
	var folderIDPtr *string
	if folderID != "" {
		folderIDPtr = &folderID
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		ext := filepath.Ext(header.Filename)
		mimeType = mime.TypeByExtension(ext)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
	}

	salt, err := crypto.GenerateSalt()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate salt"})
		return
	}

	masterKey := []byte(s.cfg.MasterKey)

	encChunks, err := crypto.ChunkAndEncrypt(file, masterKey, salt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("encryption failed: %v", err)})
		return
	}

	saltHex := crypto.HexEncode(salt)
	dbFile, err := s.db.CreateFile(ctx, cleanName, header.Size, mimeType, folderIDPtr, saltHex, len(encChunks))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("database error: %v", err)})
		return
	}

	for _, chunk := range encChunks {
		chunkFilename := fmt.Sprintf("%s.chunk.%d", dbFile.ID, chunk.Index)

		result, err := s.tg.UploadChunk(ctx, chunk.Data, chunkFilename)
		if err != nil {
			log.Printf("ERROR: upload chunk %d for file %s: %v", chunk.Index, dbFile.ID, err)
			s.cleanupFailedUpload(dbFile.ID)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("telegram upload failed at chunk %d", chunk.Index)})
			return
		}

		_, err = s.db.CreateChunk(ctx,
			dbFile.ID,
			chunk.Index,
			result.MessageID,
			result.FileID,
			len(chunk.Data),
			crypto.HexEncode(chunk.IV),
			crypto.HexEncode(chunk.AuthTag),
		)
		if err != nil {
			log.Printf("ERROR: store chunk %d metadata: %v", chunk.Index, err)
			s.cleanupFailedUpload(dbFile.ID)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store chunk metadata"})
			return
		}
	}

	writeJSON(w, http.StatusOK, dbFile)
}

// HandleDownloadFile handles GET /api/files/:id/download
func (s *Server) HandleDownloadFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fileID := extractPathParam(r.URL.Path, "files", "download")

	dbFile, err := s.db.GetFile(ctx, fileID)
	if err != nil || dbFile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	chunks, err := s.db.GetChunks(ctx, fileID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load chunks"})
		return
	}

	salt, err := crypto.HexDecode(dbFile.EncryptionSalt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid encryption salt"})
		return
	}

	masterKey := []byte(s.cfg.MasterKey)

	chunkMetas := make([]crypto.ChunkMeta, len(chunks))
	chunkMap := make(map[int]string)
	for i, c := range chunks {
		chunkMetas[i] = crypto.ChunkMeta{
			Index:   c.ChunkIndex,
			IV:      c.IV,
			AuthTag: c.AuthTag,
		}
		chunkMap[c.ChunkIndex] = c.TelegramFileID
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, dbFile.Name))
	w.Header().Set("Content-Type", dbFile.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(dbFile.Size, 10))

	err = crypto.DecryptAndStream(w, masterKey, salt, chunkMetas, func(index int) ([]byte, error) {
		tgFileID, ok := chunkMap[index]
		if !ok {
			return nil, fmt.Errorf("chunk %d not found", index)
		}
		return s.tg.DownloadChunk(ctx, tgFileID)
	})
	if err != nil {
		log.Printf("ERROR: stream download for file %s: %v", fileID, err)
	}
}

// HandleStreamFile handles GET /api/files/:id/stream with HTTP Range support.
func (s *Server) HandleStreamFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fileID := extractPathParam(r.URL.Path, "files", "stream")

	dbFile, err := s.db.GetFile(ctx, fileID)
	if err != nil || dbFile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	salt, err := crypto.HexDecode(dbFile.EncryptionSalt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid salt"})
		return
	}

	masterKey := []byte(s.cfg.MasterKey)

	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", dbFile.MimeType)

	rangeHeader := r.Header.Get("Range")
	if rangeHeader == "" {
		chunks, err := s.db.GetChunks(ctx, fileID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load chunks"})
			return
		}

		chunkMetas := make([]crypto.ChunkMeta, len(chunks))
		chunkMap := make(map[int]string)
		for i, c := range chunks {
			chunkMetas[i] = crypto.ChunkMeta{Index: c.ChunkIndex, IV: c.IV, AuthTag: c.AuthTag}
			chunkMap[c.ChunkIndex] = c.TelegramFileID
		}

		w.Header().Set("Content-Length", strconv.FormatInt(dbFile.Size, 10))
		w.WriteHeader(http.StatusOK)

		_ = crypto.DecryptAndStream(w, masterKey, salt, chunkMetas, func(index int) ([]byte, error) {
			return s.tg.DownloadChunk(ctx, chunkMap[index])
		})
		return
	}

	startByte, endByte, err := parseRangeHeader(rangeHeader, dbFile.Size)
	if err != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", dbFile.Size))
		writeJSON(w, http.StatusRequestedRangeNotSatisfiable, map[string]string{"error": "invalid range"})
		return
	}

	contentLength := endByte - startByte + 1

	chunks, offsetInFirst, err := s.db.GetChunksByRange(ctx, fileID, startByte, endByte)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load chunks"})
		return
	}

	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", startByte, endByte, dbFile.Size))
	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	w.WriteHeader(http.StatusPartialContent)

	var bytesWritten int64
	for _, chunk := range chunks {
		iv, _ := crypto.HexDecode(chunk.IV)
		authTag, _ := crypto.HexDecode(chunk.AuthTag)

		encData, err := s.tg.DownloadChunk(ctx, chunk.TelegramFileID)
		if err != nil {
			log.Printf("ERROR: stream chunk %d: %v", chunk.ChunkIndex, err)
			return
		}

		derivedKey := crypto.DeriveKey(masterKey, salt)
		plaintext, err := crypto.DecryptChunk(derivedKey, iv, encData, authTag)
		if err != nil {
			log.Printf("ERROR: decrypt chunk %d: %v", chunk.ChunkIndex, err)
			return
		}

		chunkStart := int64(0)
		chunkEnd := int64(len(plaintext))

		if chunk.ChunkIndex == chunks[0].ChunkIndex {
			chunkStart = offsetInFirst
		}

		remaining := contentLength - bytesWritten
		if chunkEnd-chunkStart > remaining {
			chunkEnd = chunkStart + remaining
		}

		n, err := w.Write(plaintext[chunkStart:chunkEnd])
		if err != nil {
			return
		}
		bytesWritten += int64(n)

		if bytesWritten >= contentLength {
			break
		}
	}
}

// HandleDeleteFile handles DELETE /api/files/:id
func (s *Server) HandleDeleteFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fileID := extractPathParam(r.URL.Path, "files", "")

	dbFile, err := s.db.GetFile(ctx, fileID)
	if err != nil || dbFile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	msgIDs, err := s.db.GetAllFileChunkTelegramMsgIDs(ctx, fileID)
	if err == nil {
		for _, msgID := range msgIDs {
			_ = s.tg.DeleteMessage(ctx, msgID)
		}
	}

	if err := s.db.DeleteFile(ctx, fileID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete file"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// HandleGetFileInfo handles GET /api/files/:id/info
func (s *Server) HandleGetFileInfo(w http.ResponseWriter, r *http.Request) {
	fileID := extractPathParam(r.URL.Path, "files", "info")

	dbFile, err := s.db.GetFile(r.Context(), fileID)
	if err != nil || dbFile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	writeJSON(w, http.StatusOK, dbFile)
}

// HandlePreviewFile handles GET /api/files/:id/preview — inline content for viewing.
func (s *Server) HandlePreviewFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fileID := extractPathParam(r.URL.Path, "files", "preview")

	dbFile, err := s.db.GetFile(ctx, fileID)
	if err != nil || dbFile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	salt, err := crypto.HexDecode(dbFile.EncryptionSalt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid salt"})
		return
	}

	masterKey := []byte(s.cfg.MasterKey)

	chunks, err := s.db.GetChunks(ctx, fileID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load chunks"})
		return
	}

	chunkMetas := make([]crypto.ChunkMeta, len(chunks))
	chunkMap := make(map[int]string)
	for i, c := range chunks {
		chunkMetas[i] = crypto.ChunkMeta{Index: c.ChunkIndex, IV: c.IV, AuthTag: c.AuthTag}
		chunkMap[c.ChunkIndex] = c.TelegramFileID
	}

	w.Header().Set("Content-Type", dbFile.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(dbFile.Size, 10))
	w.Header().Set("Accept-Ranges", "bytes")

	_ = crypto.DecryptAndStream(w, masterKey, salt, chunkMetas, func(index int) ([]byte, error) {
		return s.tg.DownloadChunk(ctx, chunkMap[index])
	})
}

// cleanupFailedUpload removes DB records and Telegram messages for a partially uploaded file.
func (s *Server) cleanupFailedUpload(fileID string) {
	ctx := context.Background()
	msgIDs, err := s.db.GetAllFileChunkTelegramMsgIDs(ctx, fileID)
	if err == nil {
		for _, id := range msgIDs {
			_ = s.tg.DeleteMessage(ctx, id)
		}
	}
	_ = s.db.DeleteFile(ctx, fileID)
}

