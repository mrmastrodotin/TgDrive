package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"tgdrive/cache"
	"tgdrive/config"
	"tgdrive/storage"
	"tgdrive/telegram"

	"golang.org/x/sync/singleflight"
)

// Server holds all dependencies for the HTTP API.
type Server struct {
	cfg         *config.Config
	db          *storage.DB
	tg          *telegram.Client
	chunkCache  *cache.ChunkCache
	chunkFlight singleflight.Group // Deduplicates concurrent downloads of the same chunk
	mux         *http.ServeMux
}

// NewServer creates a new API server with all routes registered.
func NewServer(cfg *config.Config, db *storage.DB, tg *telegram.Client) *Server {
	// 256MB LRU cache — holds ~12 decrypted chunks in RAM
	// Eliminates re-downloads from Telegram on audio seek / video scrub / PDF page turns
	cc := cache.New(256 * 1024 * 1024)

	s := &Server{
		cfg:        cfg,
		db:         db,
		tg:         tg,
		chunkCache: cc,
		mux:        http.NewServeMux(),
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
	protected.HandleFunc("POST /api/files/upload", s.HandleUploadFile)
	protected.HandleFunc("GET /api/files/{id}/download", s.HandleDownloadFile)
	protected.HandleFunc("GET /api/files/{id}/stream", s.HandleStreamFile)
	protected.HandleFunc("GET /api/files/{id}/preview", s.HandlePreviewFile)
	protected.HandleFunc("GET /api/files/{id}/info", s.HandleGetFileInfo)
	protected.HandleFunc("POST /api/files/{id}/warmup", s.HandleWarmupFile)
	protected.HandleFunc("GET /api/files/{id}/warmup", s.HandleWarmupFile)
	protected.HandleFunc("DELETE /api/files/{id}", s.HandleDeleteFile)

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

// parseRangeHeader parses an HTTP Range header of the form "bytes=START-END".
func parseRangeHeader(rangeHeader string, fileSize int64) (int64, int64, error) {
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return 0, 0, fmt.Errorf("invalid range format")
	}

	rangeParts := strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.SplitN(rangeParts, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range format")
	}

	var start, end int64
	var err error

	if parts[0] == "" {
		// Suffix range: -500 means last 500 bytes
		end = fileSize - 1
		suffix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid suffix range")
		}
		start = fileSize - suffix
		if start < 0 {
			start = 0
		}
	} else {
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid start range")
		}

		if parts[1] == "" {
			end = fileSize - 1
		} else {
			end, err = strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
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
