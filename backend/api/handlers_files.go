package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"tgdrive/cache"
	"tgdrive/crypto"
	"tgdrive/storage"
	"tgdrive/telegram"
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
		".html":  "text/plain; charset=utf-8",
		".htm":   "text/plain; charset=utf-8",
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

		// Check memory admission control with atomic in-flight reservation
		releaseMem, err := s.ReserveMemory(int64(chunk.Size) * 2)
		if err != nil {
			return nil, fmt.Errorf("server memory budget exceeded: %w", err)
		}
		defer releaseMem()

		// Enforce global download concurrency semaphore across all streams/downloads
		s.downloadSem <- struct{}{}
		defer func() { <-s.downloadSem }()

		// Retry loop: 3 attempts with exponential backoff (2s, 4s, 8s)
		var encData []byte
		const maxRetries = 3
		for attempt := 0; attempt < maxRetries; attempt++ {
			tgCtx, tgCancel := telegramFetchContext()
			encData, err = s.tg.DownloadChunk(tgCtx, chunk.TelegramFileID, chunk.BotTokenHash)
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
		encData = nil // Free ciphertext buffer immediately
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
	// Entire prefetch runs asynchronously so the warmup handler returns immediately
	go func() {
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
	if fileID == "" {
		fileID = r.PathValue("id")
	}

	dbFile, err := s.db.GetFile(r.Context(), fileID)
	if err != nil || dbFile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	callerID := GetUserFromContext(r.Context())
	if dbFile.OwnerID != "" && dbFile.OwnerID != callerID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized file access"})
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

	r.Body = http.MaxBytesReader(w, r.Body, 512<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse multipart form or payload exceeds 512MB limit"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file field required"})
		return
	}
	defer file.Close()

	var duration *float64
	if d, err := strconv.ParseFloat(r.FormValue("duration"), 64); err == nil {
		duration = &d
	}
	var width *int
	if wVal, err := strconv.Atoi(r.FormValue("width")); err == nil {
		width = &wVal
	}
	var height *int
	if hVal, err := strconv.Atoi(r.FormValue("height")); err == nil {
		height = &hVal
	}

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

	callerID := GetUserFromContext(ctx)
	saltHex := crypto.HexEncode(salt)
	dbFile, err := s.db.CreateFile(ctx, cleanName, header.Size, mimeType, folderIDPtr, saltHex, len(encChunks), s.cfg.DefaultChunkSize, callerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("database error: %v", err)})
		return
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(encChunks))
	uploadResults := make([]*telegram.UploadResult, len(encChunks))

	for i, chunk := range encChunks {
		wg.Add(1)
		go func(i int, chunk crypto.EncryptedChunk) {
			defer wg.Done()
			
			// Use the global server semaphore to prevent "clumsy" overload when uploading multiple files at once
			s.uploadSem <- struct{}{}
			defer func() { <-s.uploadSem }()

			chunkFilename := fmt.Sprintf("%s.chunk.%d", dbFile.ID, chunk.Index)
			result, err := s.tg.UploadChunk(ctx, callerID, chunk.Data, chunkFilename)
			if err != nil {
				log.Printf("ERROR: upload chunk %d for file %s: %v", chunk.Index, dbFile.ID, err)
				errCh <- fmt.Errorf("telegram upload failed at chunk %d", chunk.Index)
				return
			}
			uploadResults[i] = result
		}(i, chunk)
	}

	wg.Wait()
	close(errCh)

	if len(errCh) > 0 {
		s.cleanupFailedUpload(dbFile.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to upload one or more chunks to Telegram"})
		return
	}

	// Store metadata for all successfully uploaded chunks
	for i, chunk := range encChunks {
		res := uploadResults[i]
		_, err = s.db.CreateChunk(ctx,
			dbFile.ID,
			chunk.Index,
			res.MessageID,
			res.FileID,
			res.BotTokenHash,
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

	// 5. Upload Thumbnail if exists
	var tFileID, tIV, tAuthTag *string
	thumbFile, _, errThumb := r.FormFile("thumbnail")
	if errThumb == nil {
		defer thumbFile.Close()
		thumbChunks, err := crypto.ChunkAndEncrypt(thumbFile, masterKey, salt)
		if err == nil && len(thumbChunks) == 1 {
			thumbChunk := thumbChunks[0]
			res, err := s.tg.UploadChunk(ctx, callerID, thumbChunk.Data, dbFile.ID+".thumb")
			if err == nil {
				tf := res.FileID
				iv := crypto.HexEncode(thumbChunk.IV)
				auth := crypto.HexEncode(thumbChunk.AuthTag)
				tFileID = &tf
				tIV = &iv
				tAuthTag = &auth
			} else {
				log.Printf("WARN: failed to upload thumbnail to telegram: %v", err)
			}
		}
	}

	// 6. Save Metadata to Turso
	if duration != nil || width != nil || height != nil || tFileID != nil {
		meta := &storage.FileMetadata{
			Duration:                duration,
			Width:                   width,
			Height:                  height,
			ThumbnailTelegramFileID: tFileID,
			ThumbnailIV:             tIV,
			ThumbnailAuthTag:        tAuthTag,
		}
		if err := s.db.SaveFileMetadata(ctx, dbFile.ID, meta); err != nil {
			log.Printf("WARN: failed to save file metadata: %v", err)
		} else {
			dbFile.Metadata = meta
		}
	}

	writeJSON(w, http.StatusOK, dbFile)
}

// HandleDownloadFile handles GET /api/files/:id/download
// Uses a detached context for Telegram fetches so browser timeouts don't kill the download pipeline.
func (s *Server) HandleDownloadFile(w http.ResponseWriter, r *http.Request) {
	fileID := extractPathParam(r.URL.Path, "files", "download")
	if fileID == "" {
		fileID = r.PathValue("id")
	}

	dbFile, err := s.db.GetFile(r.Context(), fileID)
	if err != nil || dbFile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	callerID := GetUserFromContext(r.Context())
	if dbFile.OwnerID != "" && dbFile.OwnerID != callerID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized file access"})
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

	cleanFilename := filepath.Base(dbFile.Name)
	cleanFilename = strings.ReplaceAll(cleanFilename, `"`, "")
	cleanFilename = strings.ReplaceAll(cleanFilename, "\r", "")
	cleanFilename = strings.ReplaceAll(cleanFilename, "\n", "")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q; filename*=UTF-8''%s`, cleanFilename, url.PathEscape(cleanFilename)))
	w.Header().Set("Content-Type", dbFile.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(dbFile.Size, 10))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Referrer-Policy", "no-referrer")

	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

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
	if fileID == "" {
		fileID = r.PathValue("id")
	}

	dbFile, err := s.db.GetFile(r.Context(), fileID)
	if err != nil || dbFile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	callerID := GetUserFromContext(r.Context())
	if dbFile.OwnerID != "" && dbFile.OwnerID != callerID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized file access"})
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
	if strings.Contains(streamMime, "html") || strings.Contains(streamMime, "svg") || strings.Contains(streamMime, "xml") {
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	}
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Referrer-Policy", "no-referrer")
	
	// Private cache prevents public proxy caching of user files while allowing browser seeking cache
	w.Header().Set("Cache-Control", "private, max-age=86400, no-transform")
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
	w.Header().Set("Content-Length", strconv.FormatInt(dbFile.Size, 10))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

	chunks, err := s.db.GetChunks(r.Context(), fileID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load chunks"})
		return
	}

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

	// Historical legacy files always used fixed 20 MiB chunk boundaries (never variable ceil(size/count))
	const LegacyChunkSize = 20 * 1024 * 1024
	chunkSize := int64(dbFile.ChunkSize)
	if chunkSize <= 0 {
		chunkSize = LegacyChunkSize
	}

	chunks, offsetInFirst, err := s.db.GetChunksByRange(r.Context(), fileID, startByte, endByte, chunkSize)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load chunks"})
		return
	}

	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", startByte, endByte, dbFile.Size))
	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))

	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusPartialContent)
		return
	}

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

		if chunkStart < chunkEnd {
			n, err := w.Write(plaintext[chunkStart:chunkEnd])
			if err != nil {
				return
			}
			bytesWritten += int64(n)
			flushWriter(w)
		}

		if bytesWritten >= contentLength {
			break
		}
	}
}

// HandleDeleteFile handles DELETE /api/files/:id
func (s *Server) HandleDeleteFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fileID := extractPathParam(r.URL.Path, "files", "")
	if fileID == "" {
		fileID = r.PathValue("id")
	}

	dbFile, err := s.db.GetFile(ctx, fileID)
	if err != nil || dbFile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	callerID := GetUserFromContext(ctx)
	if dbFile.OwnerID != "" && dbFile.OwnerID != callerID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized file access"})
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
	if fileID == "" {
		fileID = r.PathValue("id")
	}

	dbFile, err := s.db.GetFile(r.Context(), fileID)
	if err != nil || dbFile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	callerID := GetUserFromContext(r.Context())
	if dbFile.OwnerID != "" && dbFile.OwnerID != callerID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized file access"})
		return
	}

	writeJSON(w, http.StatusOK, dbFile)
}

// HandleGetThumbnail handles GET /api/files/:id/thumbnail
func (s *Server) HandleGetThumbnail(w http.ResponseWriter, r *http.Request) {
	fileID := extractPathParam(r.URL.Path, "files", "thumbnail")
	if fileID == "" {
		fileID = r.PathValue("id")
	}

	dbFile, err := s.db.GetFile(r.Context(), fileID)
	if err != nil || dbFile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	callerID := GetUserFromContext(r.Context())
	if dbFile.OwnerID != "" && dbFile.OwnerID != callerID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized file access"})
		return
	}

	meta := dbFile.Metadata
	if meta == nil || meta.ThumbnailTelegramFileID == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thumbnail not found"})
		return
	}

	// Download the thumbnail chunk
	chunkData, err := s.tg.DownloadFileAnyBot(r.Context(), *meta.ThumbnailTelegramFileID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to download thumbnail from telegram"})
		return
	}

	// Decrypt
	iv, _ := crypto.HexDecode(*meta.ThumbnailIV)
	auth, _ := crypto.HexDecode(*meta.ThumbnailAuthTag)
	salt, _ := crypto.HexDecode(dbFile.EncryptionSalt)
	derivedKey := crypto.DeriveKey([]byte(s.cfg.MasterKey), salt)

	plaintext, err := crypto.DecryptChunk(derivedKey, iv, chunkData, auth)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to decrypt thumbnail"})
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400, no-transform")
	w.WriteHeader(http.StatusOK)
	w.Write(plaintext)
}

// HandlePreviewFile handles GET /api/files/:id/preview — inline content for viewing.
// Uses the stream handler internally since preview and stream have identical data flow.
func (s *Server) HandlePreviewFile(w http.ResponseWriter, r *http.Request) {
	fileID := extractPathParam(r.URL.Path, "files", "preview")
	if fileID == "" {
		fileID = r.PathValue("id")
	}

	dbFile, err := s.db.GetFile(r.Context(), fileID)
	if err != nil || dbFile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	callerID := GetUserFromContext(r.Context())
	if dbFile.OwnerID != "" && dbFile.OwnerID != callerID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized file access"})
		return
	}

	salt, err := crypto.HexDecode(dbFile.EncryptionSalt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid salt"})
		return
	}

	masterKey := []byte(s.cfg.MasterKey)
	derivedKey := crypto.DeriveKey(masterKey, salt)

	cleanFilename := filepath.Base(dbFile.Name)
	cleanFilename = strings.ReplaceAll(cleanFilename, `"`, "")
	cleanFilename = strings.ReplaceAll(cleanFilename, "\r", "")
	cleanFilename = strings.ReplaceAll(cleanFilename, "\n", "")
	previewMime := detectMimeType(dbFile.Name, dbFile.MimeType)
	w.Header().Set("Content-Type", previewMime)
	if strings.Contains(previewMime, "html") || strings.Contains(previewMime, "svg") || strings.Contains(previewMime, "xml") {
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename=%q; filename*=UTF-8''%s`, cleanFilename, url.PathEscape(cleanFilename)))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Referrer-Policy", "no-referrer")

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

// HandleGetStorageStats handles GET /api/stats
func (s *Server) HandleGetStorageStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.db.GetStorageStats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get storage stats"})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// HandleSaveProgress handles POST /api/files/:id/progress
func (s *Server) HandleSaveProgress(w http.ResponseWriter, r *http.Request) {
	fileID := extractPathParam(r.URL.Path, "files", "progress")
	if fileID == "" {
		fileID = r.PathValue("id")
	}

	dbFile, err := s.db.GetFile(r.Context(), fileID)
	if err != nil || dbFile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	callerID := GetUserFromContext(r.Context())
	if dbFile.OwnerID != "" && dbFile.OwnerID != callerID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized file access"})
		return
	}

	var req struct {
		Progress float64 `json:"progress"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := s.db.SaveFileProgress(r.Context(), fileID, req.Progress); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save progress"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleSetCinema handles POST/PUT/DELETE /api/files/:id/cinema
func (s *Server) HandleSetCinema(w http.ResponseWriter, r *http.Request) {
	fileID := extractPathParam(r.URL.Path, "files", "cinema")
	if fileID == "" {
		fileID = r.PathValue("id")
	}
	if fileID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing file id"})
		return
	}

	dbFile, err := s.db.GetFile(r.Context(), fileID)
	if err != nil || dbFile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	callerID := GetUserFromContext(r.Context())
	if dbFile.OwnerID != "" && dbFile.OwnerID != callerID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized file access"})
		return
	}

	isCinema := true
	if r.Method == http.MethodDelete {
		isCinema = false
	} else if r.Body != nil {
		var req struct {
			IsCinema *bool `json:"is_cinema"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err == nil && req.IsCinema != nil {
			isCinema = *req.IsCinema
		}
	}

	if err := s.db.SetFileCinema(r.Context(), fileID, isCinema); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update cinema status"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"file_id":   fileID,
		"is_cinema": isCinema,
	})
}

