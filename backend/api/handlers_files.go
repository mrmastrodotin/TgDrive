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
	"sync"
	"time"

	"tgdrive/cache"
	"tgdrive/crypto"
	"tgdrive/storage"
)

// detectMimeType returns a precise MIME type for web browsers based on file extension and headers.
func detectMimeType(filename, headerMime string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	custom := map[string]string{
		".md":    "text/markdown; charset=utf-8",
		".py":    "text/x-python; charset=utf-8",
		".js":    "application/javascript; charset=utf-8",
		".ts":    "application/typescript; charset=utf-8",
		".tsx":   "application/typescript; charset=utf-8",
		".jsx":   "application/javascript; charset=utf-8",
		".go":    "text/x-go; charset=utf-8",
		".rs":    "text/rust; charset=utf-8",
		".c":     "text/x-c; charset=utf-8",
		".cpp":   "text/x-c++; charset=utf-8",
		".h":     "text/x-c; charset=utf-8",
		".java":  "text/x-java-source; charset=utf-8",
		".json":  "application/json",
		".yaml":  "text/yaml; charset=utf-8",
		".yml":   "text/yaml; charset=utf-8",
		".toml":  "text/toml; charset=utf-8",
		".xml":   "application/xml",
		".html":  "text/html; charset=utf-8",
		".css":   "text/css; charset=utf-8",
		".scss":  "text/x-scss; charset=utf-8",
		".sql":   "text/x-sql; charset=utf-8",
		".sh":    "application/x-sh; charset=utf-8",
		".bat":   "application/x-bat; charset=utf-8",
		".ps1":   "text/plain; charset=utf-8",
		".txt":   "text/plain; charset=utf-8",
		".csv":   "text/csv; charset=utf-8",
		".tsv":   "text/tab-separated-values; charset=utf-8",
		".log":   "text/plain; charset=utf-8",
		".env":   "text/plain; charset=utf-8",
		".pdf":   "application/pdf",
		".png":   "image/png",
		".jpg":   "image/jpeg",
		".jpeg":  "image/jpeg",
		".gif":   "image/gif",
		".webp":  "image/webp",
		".svg":   "image/svg+xml",
		".ico":   "image/x-icon",
		".mp4":   "video/mp4",
		".webm":  "video/webm",
		".mkv":   "video/x-matroska",
		".mov":   "video/quicktime",
		".mp3":   "audio/mpeg",
		".wav":   "audio/wav",
		".ogg":   "audio/ogg",
		".flac":  "audio/flac",
		".m4a":   "audio/mp4",
		".ppt":   "application/vnd.ms-powerpoint",
		".pptx":  "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		".doc":   "application/msword",
		".docx":  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xls":   "application/vnd.ms-excel",
		".xlsx":  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".zip":   "application/zip",
		".tar":   "application/x-tar",
		".gz":    "application/gzip",
		".7z":    "application/x-7z-compressed",
		".rar":   "application/x-rar-compressed",
	}
	if ct, ok := custom[ext]; ok {
		return ct
	}
	if headerMime != "" && headerMime != "application/octet-stream" {
		return headerMime
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// telegramFetchContext creates a background context with a generous timeout for Telegram API calls.
// This is deliberately detached from the HTTP request context so that browser/proxy timeouts
// don't cascade into Telegram download cancellations.
func telegramFetchContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 8*time.Minute)
}

// flushWriter flushes the http.ResponseWriter if it supports http.Flusher.
func flushWriter(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// isClientGone checks if the HTTP client has disconnected.
func isClientGone(r *http.Request) bool {
	select {
	case <-r.Context().Done():
		return true
	default:
		return false
	}
}

// fetchChunkDecrypted gets a decrypted chunk, using the LRU cache first.
// Uses singleflight to deduplicate concurrent downloads of the same chunk,
// and retries with exponential backoff on transient Telegram failures.
func (s *Server) fetchChunkDecrypted(fileID string, chunk storage.Chunk, derivedKey []byte) ([]byte, error) {
	key := cache.Key(fileID, chunk.ChunkIndex)

	// Fast path: cache hit
	if data := s.chunkCache.Get(key); data != nil {
		return data, nil
	}

	// Singleflight: if another goroutine is already downloading this exact chunk,
	// wait for that result instead of hammering Telegram with duplicate requests.
	result, err, _ := s.chunkFlight.Do(key, func() (interface{}, error) {
		// Double-check cache inside singleflight (another caller might have just cached it)
		if data := s.chunkCache.Get(key); data != nil {
			return data, nil
		}

		iv, err := crypto.HexDecode(chunk.IV)
		if err != nil {
			return nil, fmt.Errorf("decode IV chunk %d: %w", chunk.ChunkIndex, err)
		}
		authTag, err := crypto.HexDecode(chunk.AuthTag)
		if err != nil {
			return nil, fmt.Errorf("decode auth tag chunk %d: %w", chunk.ChunkIndex, err)
		}

		// Retry loop: 3 attempts with exponential backoff (2s, 4s, 8s)
		var encData []byte
		const maxRetries = 3
		for attempt := 0; attempt < maxRetries; attempt++ {
			tgCtx, tgCancel := telegramFetchContext()
			encData, err = s.tg.DownloadChunk(tgCtx, chunk.TelegramFileID)
			tgCancel()

			if err == nil {
				break
			}

			log.Printf("WARN: chunk %d download attempt %d/%d failed: %v", chunk.ChunkIndex, attempt+1, maxRetries, err)
			if attempt < maxRetries-1 {
				backoff := time.Duration(2<<uint(attempt)) * time.Second
				time.Sleep(backoff)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("fetch chunk %d after %d retries: %w", chunk.ChunkIndex, maxRetries, err)
		}

		plaintext, err := crypto.DecryptChunk(derivedKey, iv, encData, authTag)
		if err != nil {
			return nil, fmt.Errorf("decrypt chunk %d: %w", chunk.ChunkIndex, err)
		}

		s.chunkCache.Put(key, plaintext)
		return plaintext, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}

// prefetchAhead starts background goroutines to download and cache upcoming chunks.
// Buffers ahead (up to 3 chunks ~60MB) so playback and seeking stay smooth.
func (s *Server) prefetchAhead(fileID string, chunks []storage.Chunk, currentIdx int, derivedKey []byte) {
	for offset := 1; offset <= 3; offset++ {
		nextIdx := currentIdx + offset
		if nextIdx >= len(chunks) {
			break
		}
		next := chunks[nextIdx]
		key := cache.Key(fileID, next.ChunkIndex)

		if s.chunkCache.Get(key) != nil {
			continue
		}

		go func(c storage.Chunk) {
			_, err := s.fetchChunkDecrypted(fileID, c, derivedKey)
			if err != nil {
				log.Printf("INFO: prefetch chunk %d for %s failed (non-fatal): %v", c.ChunkIndex, fileID, err)
			}
		}(next)
	}
}

// prefetchAllChunks downloads and caches ALL chunks for a file in background.
// This is called by the warmup endpoint so the browser's video element
// never has to wait for cold Telegram downloads.
func (s *Server) prefetchAllChunks(fileID string, chunks []storage.Chunk, derivedKey []byte) {
	// Use a semaphore to limit concurrent Telegram downloads to 2
	sem := make(chan struct{}, 2)
	var wg sync.WaitGroup

	for _, chunk := range chunks {
		key := cache.Key(fileID, chunk.ChunkIndex)
		if s.chunkCache.Get(key) != nil {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(c storage.Chunk) {
			defer wg.Done()
			defer func() { <-sem }()
			_, err := s.fetchChunkDecrypted(fileID, c, derivedKey)
			if err != nil {
				log.Printf("WARN: warmup chunk %d for %s failed: %v", c.ChunkIndex, fileID, err)
			} else {
				log.Printf("INFO: warmup chunk %d for %s cached", c.ChunkIndex, fileID)
			}
		}(chunk)
	}

	// Don't block — let it finish in background
	go func() {
		wg.Wait()
		log.Printf("INFO: warmup complete for file %s (%d chunks)", fileID, len(chunks))
	}()
}

// HandleWarmupFile handles POST /api/files/:id/warmup
// Triggers background download of all chunks for a file and returns
// the current cache status immediately. The frontend polls this to
// know when chunk 0 is ready before setting the video src.
func (s *Server) HandleWarmupFile(w http.ResponseWriter, r *http.Request) {
	fileID := extractPathParam(r.URL.Path, "files", "warmup")

	dbFile, err := s.db.GetFile(r.Context(), fileID)
	if err != nil || dbFile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	chunks, err := s.db.GetChunks(r.Context(), fileID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load chunks"})
		return
	}

	salt, err := crypto.HexDecode(dbFile.EncryptionSalt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid salt"})
		return
	}

	masterKey := []byte(s.cfg.MasterKey)
	derivedKey := crypto.DeriveKey(masterKey, salt)

	// Check which chunks are already cached
	cachedMap := make(map[int]bool)
	cachedChunks := make([]int, 0)
	for _, c := range chunks {
		key := cache.Key(fileID, c.ChunkIndex)
		if s.chunkCache.Get(key) != nil {
			cachedChunks = append(cachedChunks, c.ChunkIndex)
			cachedMap[c.ChunkIndex] = true
		}
	}

	// Ready when at least 2 chunks (or all chunks if total < 2) are cached,
	// ensuring chunks 0 and 1 are ready for instant playback and initial seeking (~40MB)
	minRequired := 2
	if len(chunks) < 2 {
		minRequired = len(chunks)
	}
	ready := true
	for i := 0; i < minRequired; i++ {
		if !cachedMap[chunks[i].ChunkIndex] {
			ready = false
			break
		}
	}

	// Kick off background prefetch for all uncached chunks
	s.prefetchAllChunks(fileID, chunks, derivedKey)

	// Return status immediately
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"file_id":       fileID,
		"total_chunks":  len(chunks),
		"cached_chunks": cachedChunks,
		"cached_count":  len(cachedChunks),
		"ready":         ready,
	})
}


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

	mimeType := detectMimeType(cleanName, header.Header.Get("Content-Type"))

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
// Uses a detached context for Telegram fetches so browser timeouts don't kill the download pipeline.
func (s *Server) HandleDownloadFile(w http.ResponseWriter, r *http.Request) {
	fileID := extractPathParam(r.URL.Path, "files", "download")

	dbFile, err := s.db.GetFile(r.Context(), fileID)
	if err != nil || dbFile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	chunks, err := s.db.GetChunks(r.Context(), fileID)
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
	derivedKey := crypto.DeriveKey(masterKey, salt)

	chunkMap := make(map[int]string)
	for _, c := range chunks {
		chunkMap[c.ChunkIndex] = c.TelegramFileID
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, dbFile.Name))
	w.Header().Set("Content-Type", dbFile.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(dbFile.Size, 10))
	w.WriteHeader(http.StatusOK)
	flushWriter(w)

	for i, chunk := range chunks {
		if isClientGone(r) {
			log.Printf("INFO: client disconnected during download of %s at chunk %d", fileID, chunk.ChunkIndex)
			return
		}

		// Prefetch upcoming chunks while we write the current one
		s.prefetchAhead(fileID, chunks, i, derivedKey)

		plaintext, err := s.fetchChunkDecrypted(fileID, chunk, derivedKey)
		if err != nil {
			log.Printf("ERROR: download %s: %v", fileID, err)
			return
		}

		if _, err := w.Write(plaintext); err != nil {
			return
		}
		flushWriter(w)
	}
}

// HandleStreamFile handles GET /api/files/:id/stream with HTTP Range support.
// This is the critical path for audio/video playback and image/PDF loading.
func (s *Server) HandleStreamFile(w http.ResponseWriter, r *http.Request) {
	fileID := extractPathParam(r.URL.Path, "files", "stream")

	dbFile, err := s.db.GetFile(r.Context(), fileID)
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
	derivedKey := crypto.DeriveKey(masterKey, salt)

	streamMime := detectMimeType(dbFile.Name, dbFile.MimeType)
	w.Header().Set("Content-Type", streamMime)
	w.Header().Set("Accept-Ranges", "bytes")
	
	// Instruct the browser to aggressively cache the video/audio chunks
	// This gives us the "+/- 5min" buffer effect for free.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, dbFile.ID))

	rangeHeader := r.Header.Get("Range")
	if rangeHeader == "" {
		// Full file stream (no range)
		s.streamFullFile(w, r, dbFile, fileID, derivedKey)
		return
	}

	// Range request
	s.streamRangeFile(w, r, dbFile, fileID, derivedKey, rangeHeader)
}

// streamFullFile streams the entire decrypted file to the client.
func (s *Server) streamFullFile(w http.ResponseWriter, r *http.Request, dbFile *storage.File, fileID string, derivedKey []byte) {
	chunks, err := s.db.GetChunks(r.Context(), fileID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load chunks"})
		return
	}

	w.Header().Set("Content-Length", strconv.FormatInt(dbFile.Size, 10))
	w.WriteHeader(http.StatusOK)
	flushWriter(w)

	for i, chunk := range chunks {
		if isClientGone(r) {
			return
		}

		s.prefetchAhead(fileID, chunks, i, derivedKey)

		plaintext, err := s.fetchChunkDecrypted(fileID, chunk, derivedKey)
		if err != nil {
			log.Printf("ERROR: stream full %s: %v", fileID, err)
			return
		}

		if _, err := w.Write(plaintext); err != nil {
			return
		}
		flushWriter(w)
	}
}

// streamRangeFile handles HTTP Range requests for partial content delivery.
func (s *Server) streamRangeFile(w http.ResponseWriter, r *http.Request, dbFile *storage.File, fileID string, derivedKey []byte, rangeHeader string) {
	startByte, endByte, err := parseRangeHeader(rangeHeader, dbFile.Size)
	if err != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", dbFile.Size))
		writeJSON(w, http.StatusRequestedRangeNotSatisfiable, map[string]string{"error": "invalid range"})
		return
	}

	contentLength := endByte - startByte + 1

	chunks, offsetInFirst, err := s.db.GetChunksByRange(r.Context(), fileID, startByte, endByte)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load chunks"})
		return
	}

	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", startByte, endByte, dbFile.Size))
	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	w.WriteHeader(http.StatusPartialContent)
	flushWriter(w)

	var bytesWritten int64
	for i, chunk := range chunks {
		if isClientGone(r) {
			return
		}

		// Prefetch upcoming chunks while we write the current one
		s.prefetchAhead(fileID, chunks, i, derivedKey)

		plaintext, err := s.fetchChunkDecrypted(fileID, chunk, derivedKey)
		if err != nil {
			log.Printf("ERROR: stream range %s: %v", fileID, err)
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
		flushWriter(w)

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
// Uses the stream handler internally since preview and stream have identical data flow.
func (s *Server) HandlePreviewFile(w http.ResponseWriter, r *http.Request) {
	fileID := extractPathParam(r.URL.Path, "files", "preview")

	dbFile, err := s.db.GetFile(r.Context(), fileID)
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
	derivedKey := crypto.DeriveKey(masterKey, salt)

	previewMime := detectMimeType(dbFile.Name, dbFile.MimeType)
	w.Header().Set("Content-Type", previewMime)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", dbFile.Name))
	w.Header().Set("Accept-Ranges", "bytes")

	// Check for range request (PDF viewers and browsers use these)
	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" {
		s.streamRangeFile(w, r, dbFile, fileID, derivedKey, rangeHeader)
		return
	}

	// Full file delivery
	s.streamFullFile(w, r, dbFile, fileID, derivedKey)
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

