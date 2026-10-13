package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tgdrive/cache"
	"tgdrive/config"
	"tgdrive/storage"
	"tgdrive/system"
	"tgdrive/telegram"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/sync/singleflight"
)

// Server holds all dependencies for the HTTP API.
type Server struct {
	cfg                     *config.Config
	db                      *storage.DB
	tg                      *telegram.BotPool
	chunkCache              *cache.ChunkCache
	chunkFlight             singleflight.Group // Deduplicates concurrent downloads of the same chunk
	chunkLockMgr            *ChunkLockManager  // Atomically coordinates concurrent uploads for the exact same chunk
	uploadSem               chan struct{}      // Global concurrency limit for uploads across all files
	downloadSem             chan struct{}      // Global concurrency limit for downloads and stream chunk fetches
	inFlightMemoryBytes     atomic.Int64       // Atomic tracking of in-flight reserved buffer memory
	peakInFlightMemoryBytes atomic.Int64       // Peak in-flight memory recorded
	rejectedRequests        atomic.Uint64      // Counter of operations rejected by admission control
	memMu                   sync.Mutex         // Synchronizes admission checks and reservations under contention
	mux                     *http.ServeMux
}


// NewServer creates a new API server with all routes registered.
func NewServer(cfg *config.Config, db *storage.DB, tg *telegram.BotPool) *Server {
	if cfg.CacheMaxBytes <= 0 {
		cfg.CacheMaxBytes = 64 * 1024 * 1024 // 64 MiB default for 512 MiB container target
	}
	cc := cache.New(cfg.CacheMaxBytes)

	if cfg.MaxConcurrentUploads <= 0 {
		cfg.MaxConcurrentUploads = 4
	}

	if cfg.MaxConcurrentDownloads <= 0 {
		cfg.MaxConcurrentDownloads = 4
	}

	if cfg.MaxUserConcurrency > 0 && tg != nil {
		tg.SetMaxUserConcurrency(cfg.MaxUserConcurrency)
	}

	s := &Server{
		cfg:          cfg,
		db:           db,
		tg:           tg,
		chunkCache:   cc,
		chunkLockMgr: NewChunkLockManager(),
		uploadSem:    make(chan struct{}, cfg.MaxConcurrentUploads),
		downloadSem:  make(chan struct{}, cfg.MaxConcurrentDownloads),
		mux:          http.NewServeMux(),
	}

	s.registerRoutes()
	return s
}

// Handler returns the HTTP handler with full HTTPS and security middleware applied.
func (s *Server) Handler() http.Handler {
	return s.ForceHTTPSMiddleware(s.SecurityHeadersMiddleware(s.mux))
}

func (s *Server) registerRoutes() {
	// Public routes
	s.mux.HandleFunc("POST /api/auth/login", s.HandleLogin)

	// Protected routes — wrap with auth middleware
	protected := http.NewServeMux()

	// Folders
	protected.HandleFunc("GET /api/folders", s.HandleListFolders)
	protected.HandleFunc("POST /api/folders", s.HandleCreateFolder)
	protected.HandleFunc("DELETE /api/folders/{id}", s.HandleDeleteFolder)
	protected.HandleFunc("PUT /api/folders/{id}", s.HandleRenameFolder)

	// Files
	protected.HandleFunc("POST /api/files/upload/init", s.HandleUploadInit)
	protected.HandleFunc("PUT /api/files/upload/{uploadId}/chunks/{chunkIndex}", s.HandleUploadChunk)
	protected.HandleFunc("POST /api/files/upload/{uploadId}/chunks/{chunkIndex}", s.HandleUploadChunk)
	protected.HandleFunc("GET /api/files/upload/{uploadId}/status", s.HandleUploadStatus)
	protected.HandleFunc("POST /api/files/upload/{uploadId}/finalize", s.HandleUploadFinalize)
	protected.HandleFunc("POST /api/files/upload/{uploadId}/cancel", s.HandleUploadCancel)
	protected.HandleFunc("POST /api/files/upload", s.HandleUploadFile) // Legacy/direct single-part upload support
	protected.HandleFunc("POST /api/files/{id}/ticket", s.HandleCreateStreamTicket)
	protected.HandleFunc("GET /api/files/{id}/download", s.HandleDownloadFile)
	protected.HandleFunc("GET /api/files/{id}/thumbnail", s.HandleGetThumbnail)
	protected.HandleFunc("GET /api/files/{id}/stream", s.HandleStreamFile)
	protected.HandleFunc("GET /api/files/{id}/preview", s.HandlePreviewFile)
	protected.HandleFunc("GET /api/files/{id}/info", s.HandleGetFileInfo)
	protected.HandleFunc("POST /api/files/{id}/warmup", s.HandleWarmupFile)
	protected.HandleFunc("GET /api/files/{id}/warmup", s.HandleWarmupFile)
	protected.HandleFunc("POST /api/files/{id}/progress", s.HandleSaveProgress)
	protected.HandleFunc("POST /api/files/{id}/cinema", s.HandleSetCinema)
	protected.HandleFunc("DELETE /api/files/{id}/cinema", s.HandleSetCinema)
	protected.HandleFunc("DELETE /api/files/{id}", s.HandleDeleteFile)

	// Stats
	protected.HandleFunc("GET /api/stats", s.HandleGetStorageStats)
	protected.HandleFunc("GET /api/system/memory", s.HandleGetMemoryStats)

	// Bots Pool Status & Dynamic Worker Management (per-user isolated pools)
	protected.HandleFunc("GET /api/bots", s.HandleListBots)
	protected.HandleFunc("POST /api/bots", s.HandleAddBot)
	protected.HandleFunc("DELETE /api/bots/{hash}", s.HandleRemoveBot)
	protected.HandleFunc("POST /api/bots/{hash}/disable", s.HandleDisableBot)
	protected.HandleFunc("POST /api/bots/{hash}/rotate", s.HandleRotateBot)
	protected.HandleFunc("POST /api/bots/{hash}/migrate", s.HandleMigrateBotChunks)
	protected.HandleFunc("GET /api/bots/status", func(w http.ResponseWriter, r *http.Request) {
		callerID := GetUserFromContext(r.Context())
		total, online := s.tg.GetStatsForOwner(callerID)
		safeCap := s.tg.GetSafeCapacityForOwner(callerID)
		writeJSON(w, http.StatusOK, map[string]int{
			"total":    total,
			"online":   online,
			"capacity": safeCap,
		})
	})
	protected.HandleFunc("GET /api/bots/capacity", func(w http.ResponseWriter, r *http.Request) {
		callerID := GetUserFromContext(r.Context())
		total, online := s.tg.GetStatsForOwner(callerID)
		safeCap := s.tg.GetSafeCapacityForOwner(callerID)
		writeJSON(w, http.StatusOK, map[string]int{
			"capacity":    safeCap,
			"total_bots":  total,
			"online_bots": online,
		})
	})

	// Apply auth middleware to all /api/ routes except login
	s.mux.Handle("/api/", s.AuthMiddleware(protected))

	// Health check (public)
	s.mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Serve frontend static files with SPA fallback (Docker / production)
	staticDir := "./static"
	fs := http.FileServer(http.Dir(staticDir))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Skip /api — those are handled by the protected mux
		if strings.HasPrefix(r.URL.Path, "/api") {
			http.NotFound(w, r)
			return
		}

		// Check if the requested file exists on disk
		path := staticDir + r.URL.Path
		if r.URL.Path != "/" {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				// SPA fallback: serve index.html for client-side routes
				http.ServeFile(w, r, staticDir+"/index.html")
				return
			}
		}
		fs.ServeHTTP(w, r)
	})
}

// writeJSON is a helper to write JSON responses.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("ERROR: write json: %v", err)
	}
}

// extractPathParam extracts an ID from URL patterns like /api/{resource}/{id}/{action}
func extractPathParam(path, resource, action string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// Find the resource in the path, return the next segment
	for i, p := range parts {
		if p == resource && i+1 < len(parts) {
			id := parts[i+1]
			// If action is specified, verify the last segment matches
			if action != "" && i+2 < len(parts) && parts[i+2] == action {
				return id
			}
			if action == "" {
				return id
			}
			return id
		}
	}
	return ""
}

type chunkLockEntry struct {
	mu            sync.Mutex
	refCount      int
	markedDeleted bool
}

// ChunkLockManager guarantees that simultaneous operations on the exact same chunk index
// always share the exact same synchronization primitive, preventing distinct mutexes from
// being created during in-flight races.
type ChunkLockManager struct {
	mu    sync.Mutex
	locks map[string]*chunkLockEntry
}

// NewChunkLockManager creates an initialized ChunkLockManager.
func NewChunkLockManager() *ChunkLockManager {
	return &ChunkLockManager{
		locks: make(map[string]*chunkLockEntry),
	}
}

// Acquire gets or creates the lock for (uploadID, chunkIndex), increments refCount,
// locks the entry's mutex, and returns an unlock/release function.
func (m *ChunkLockManager) Acquire(uploadID string, chunkIndex int) func() {
	key := fmt.Sprintf("%s:%d", uploadID, chunkIndex)

	m.mu.Lock()
	entry, exists := m.locks[key]
	if !exists {
		entry = &chunkLockEntry{}
		m.locks[key] = entry
	}
	entry.refCount++
	m.mu.Unlock()

	entry.mu.Lock()

	return func() {
		entry.mu.Unlock()

		m.mu.Lock()
		entry.refCount--
		if entry.markedDeleted && entry.refCount <= 0 {
			delete(m.locks, key)
		}
		m.mu.Unlock()
	}
}

// ClearSession clears lock entries associated with an upload session.
// If any entry is currently in use (refCount > 0), it is marked for deletion upon completion
// so that in-flight operations continue to synchronize on the same mutex until finished.
func (m *ChunkLockManager) ClearSession(uploadID string) {
	prefix := uploadID + ":"
	m.mu.Lock()
	defer m.mu.Unlock()

	for k, entry := range m.locks {
		if strings.HasPrefix(k, prefix) {
			if entry.refCount <= 0 {
				delete(m.locks, k)
			} else {
				entry.markedDeleted = true
			}
		}
	}
}

// HasLock checks if a lock entry exists in the manager.
func (m *ChunkLockManager) HasLock(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.locks[key]
	return exists
}

// ReserveMemory enforces atomic resource-aware admission control and reserves estimated in-flight memory bytes.
// It checks current in-flight reservations + active runtime HeapAlloc against MaxMemoryBudgetBytes.
// If the budget is saturated, it flushes the chunk LRU cache and triggers a garbage collection cycle.
// If memory remains insufficient, it rejects the operation.
// On admission, it atomically increments inFlightMemoryBytes and returns an idempotent release callback.
func (s *Server) ReserveMemory(estimatedBytes int64) (func(), error) {
	if s.cfg.MaxMemoryBudgetBytes <= 0 {
		return func() {}, nil
	}

	s.memMu.Lock()
	defer s.memMu.Unlock()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	currInFlight := s.inFlightMemoryBytes.Load()

	if currInFlight+int64(m.HeapAlloc)+estimatedBytes > s.cfg.MaxMemoryBudgetBytes {
		// Attempt LRU cache flush and GC under pressure
		s.chunkCache.Clear()
		runtime.GC()
		runtime.ReadMemStats(&m)
		currInFlight = s.inFlightMemoryBytes.Load()

		if currInFlight+int64(m.HeapAlloc)+estimatedBytes > s.cfg.MaxMemoryBudgetBytes {
			s.rejectedRequests.Add(1)
			return nil, fmt.Errorf("server memory budget exceeded (in-flight: %d, heap: %d, request: %d, budget: %d)",
				currInFlight, m.HeapAlloc, estimatedBytes, s.cfg.MaxMemoryBudgetBytes)
		}
	}

	newInFlight := s.inFlightMemoryBytes.Add(estimatedBytes)
	for {
		peak := s.peakInFlightMemoryBytes.Load()
		if newInFlight <= peak {
			break
		}
		if s.peakInFlightMemoryBytes.CompareAndSwap(peak, newInFlight) {
			break
		}
	}

	var once sync.Once
	release := func() {
		once.Do(func() {
			s.inFlightMemoryBytes.Add(-estimatedBytes)
		})
	}

	return release, nil
}

// checkMemoryAdmission enforces resource-aware admission control against the configured memory budget.
// Retained for backwards compatibility with non-streaming checks.
func (s *Server) checkMemoryAdmission(requiredBytes int64) error {
	release, err := s.ReserveMemory(requiredBytes)
	if err != nil {
		return err
	}
	release()
	return nil
}

// HandleCreateStreamTicket handles POST /api/files/{id}/ticket
// Issues a short-lived (5 min) JWT ticket scoped strictly to this file ID and action for native media elements.
func (s *Server) HandleCreateStreamTicket(w http.ResponseWriter, r *http.Request) {
	fileID := extractPathParam(r.URL.Path, "files", "ticket")
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

	action := r.URL.Query().Get("action")
	if action == "" {
		action = "stream"
	}
	switch action {
	case "stream", "preview", "download":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ticket action (must be stream, preview, or download)"})
		return
	}

	expiresAt := time.Now().Add(5 * time.Minute)
	claims := AuthClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   callerID,
			Audience:  jwt.ClaimStrings{action + ":" + fileID},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "tgdrive",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate stream ticket"})
		return
	}

	w.Header().Set("Referrer-Policy", "no-referrer")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ticket":     tokenStr,
		"expires_at": expiresAt.Unix(),
		"file_id":    fileID,
		"action":     action,
	})
}

// HandleGetMemoryStats returns current container memory usage, Linux RSS, LRU cache stats, and active admission metrics.
// Protected as an administrative endpoint: non-admin callers receive 403 Forbidden.
func (s *Server) HandleGetMemoryStats(w http.ResponseWriter, r *http.Request) {
	callerID := GetUserFromContext(r.Context())
	if callerID != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: admin privileges required"})
		return
	}

	containerMetrics := system.ReadContainerMemoryMetrics()
	cachedEntries, cachedBytes, maxCacheBytes := s.chunkCache.Stats()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		// Linux Container & Process Resident Memory
		"process_rss_bytes":           containerMetrics.ProcessRSSBytes,
		"peak_process_rss_bytes":      containerMetrics.PeakProcessRSSBytes,
		"container_memory_bytes":      containerMetrics.ContainerMemoryBytes,
		"peak_container_memory_bytes": containerMetrics.PeakContainerMemoryBytes,
		"container_memory_limit":      containerMetrics.ContainerMemoryLimitBytes,
		"cgroup_version":              containerMetrics.CgroupVersion,
		"cgroup_available":            containerMetrics.CgroupAvailable,
		"rss_available":               containerMetrics.RSSAvailable,

		// Go Runtime Memory
		"alloc_bytes":                 containerMetrics.GoAllocBytes,
		"go_alloc_bytes":              containerMetrics.GoAllocBytes,
		"total_alloc_bytes":           containerMetrics.GoAllocBytes,
		"sys_bytes":                   containerMetrics.GoSysBytes,
		"go_sys_bytes":                containerMetrics.GoSysBytes,
		"heap_alloc_bytes":            containerMetrics.GoHeapAllocBytes,
		"go_heap_alloc_bytes":         containerMetrics.GoHeapAllocBytes,
		"heap_inuse_bytes":            containerMetrics.GoHeapInUseBytes,
		"go_heap_inuse_bytes":         containerMetrics.GoHeapInUseBytes,
		"num_gc":                      containerMetrics.NumGC,

		// In-Flight Admission & Budget Tracking
		"in_flight_bytes":             s.inFlightMemoryBytes.Load(),
		"peak_in_flight_bytes":        s.peakInFlightMemoryBytes.Load(),
		"memory_budget_bytes":         s.cfg.MaxMemoryBudgetBytes,
		"rejected_requests":           s.rejectedRequests.Load(),

		// LRU Cache Stats
		"cache_entries":               cachedEntries,
		"cache_used_bytes":            cachedBytes,
		"cache_max_bytes":             maxCacheBytes,

		// Concurrency Semaphores
		"upload_sem_active":           len(s.uploadSem),
		"upload_sem_capacity":         cap(s.uploadSem),
		"download_sem_active":         len(s.downloadSem),
		"download_sem_capacity":       cap(s.downloadSem),
	})
}


// parseRangeHeader parses an HTTP Range header of the form "bytes=START-END".
func parseRangeHeader(rangeHeader string, fileSize int64) (int64, int64, error) {
	if fileSize <= 0 {
		return 0, 0, fmt.Errorf("range not satisfiable for empty file")
	}

	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return 0, 0, fmt.Errorf("invalid range format")
	}

	rangeParts := strings.TrimPrefix(rangeHeader, "bytes=")
	// If comma-separated multiple ranges requested, take the first range
	if idx := strings.Index(rangeParts, ","); idx != -1 {
		rangeParts = strings.TrimSpace(rangeParts[:idx])
	}

	parts := strings.SplitN(rangeParts, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range format")
	}

	var start, end int64
	var err error

	if parts[0] == "" {
		// Suffix range: -500 means last 500 bytes
		suffix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, fmt.Errorf("invalid suffix range")
		}
		start = fileSize - suffix
		if start < 0 {
			start = 0
		}
		end = fileSize - 1
	} else {
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil || start < 0 {
			return 0, 0, fmt.Errorf("invalid start range")
		}

		if parts[1] == "" {
			end = fileSize - 1
		} else {
			end, err = strconv.ParseInt(parts[1], 10, 64)
			if err != nil || end < 0 {
				return 0, 0, fmt.Errorf("invalid end range")
			}
		}
	}

	if start > end || start >= fileSize {
		return 0, 0, fmt.Errorf("range out of bounds")
	}

	if end >= fileSize {
		end = fileSize - 1
	}

	return start, end, nil
}
