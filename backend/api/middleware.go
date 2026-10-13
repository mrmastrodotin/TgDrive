package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AuthClaims holds JWT token claims.
type AuthClaims struct {
	jwt.RegisteredClaims
}

// loginRequest is the POST body for /api/auth/login.
type loginRequest struct {
	Password string `json:"password"`
}

// loginResponse is the response for /api/auth/login.
type loginResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

// Rate limiter for brute force protection
type clientRate struct {
	attempts     int
	blockedUntil time.Time
}

const maxLoginLimiterEntries = 5000

var (
	loginLimiter = make(map[string]*clientRate)
	loginMu      sync.Mutex
)

// cleanupLoginLimiterLocked evicts expired rate-limit records to prevent unbounded memory growth.
func cleanupLoginLimiterLocked(now time.Time) {
	if len(loginLimiter) < maxLoginLimiterEntries {
		return
	}
	// Evict entries whose block has expired and attempts reset
	for k, v := range loginLimiter {
		if now.After(v.blockedUntil) && v.attempts == 0 {
			delete(loginLimiter, k)
		}
	}
	// If still above capacity, evict any entry whose block has expired
	if len(loginLimiter) >= maxLoginLimiterEntries {
		for k, v := range loginLimiter {
			if now.After(v.blockedUntil) {
				delete(loginLimiter, k)
			}
		}
	}
}

// HandleLogin validates the password and returns a JWT with brute-force rate limiting.
func (s *Server) HandleLogin(w http.ResponseWriter, r *http.Request) {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		ip = r.RemoteAddr
	}
	if comma := strings.Index(ip, ","); comma != -1 {
		ip = strings.TrimSpace(ip[:comma])
	}
	if colon := strings.LastIndex(ip, ":"); colon != -1 {
		ip = ip[:colon]
	}

	loginMu.Lock()
	now := time.Now()
	cleanupLoginLimiterLocked(now)

	state, exists := loginLimiter[ip]
	if !exists {
		state = &clientRate{}
		loginLimiter[ip] = state
	}
	if state.blockedUntil.After(now) {
		loginMu.Unlock()
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "too many failed attempts, please try again in 5 minutes",
		})
		return
	}
	loginMu.Unlock()

	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.cfg.AppPassword)) != 1 {
		loginMu.Lock()
		state.attempts++
		if state.attempts >= 5 {
			state.blockedUntil = now.Add(5 * time.Minute)
			state.attempts = 0
		}
		loginMu.Unlock()

		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid password"})
		return
	}

	// Reset attempts on successful login
	loginMu.Lock()
	state.attempts = 0
	state.blockedUntil = time.Time{}
	loginMu.Unlock()

	expiresAt := time.Now().Add(24 * time.Hour)
	// Single-User Mode: All direct password-authenticated sessions receive the "admin" identity.
	// Architectural multi-user isolation is enforced whenever distinct Subject identities
	// are supplied via external identity providers or tenant-scoped JWT tokens.
	claims := AuthClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "admin",
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "tgdrive",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}

	writeJSON(w, http.StatusOK, loginResponse{
		Token:     tokenStr,
		ExpiresAt: expiresAt.Unix(),
	})
}

type contextKey string

const userContextKey = contextKey("user_id")

// GetUserFromContext extracts the authenticated user ID from context, defaulting to "admin".
func GetUserFromContext(ctx context.Context) string {
	if val, ok := ctx.Value(userContextKey).(string); ok && val != "" {
		return val
	}
	return "admin"
}

// WithUserContext wraps a context with a specific user ID for tests or internal calls.
func WithUserContext(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userContextKey, userID)
}

// AuthMiddleware validates JWT tokens on protected routes.
func (s *Server) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := ""
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
		} else if r.Method == http.MethodGet || r.Method == http.MethodHead {
			// Query parameter token/ticket is permitted strictly on read-only media requests (e.g. native video/audio/img/a tags)
			if qToken := r.URL.Query().Get("token"); qToken != "" {
				tokenStr = qToken
			} else if ticket := r.URL.Query().Get("ticket"); ticket != "" {
				tokenStr = ticket
			}
		}

		if tokenStr == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
			return
		}

		claims := &AuthClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(s.cfg.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
			return
		}

		// If this is a scoped stream ticket, verify its audience matches the target route exactly
		if len(claims.Audience) > 0 {
			authorized := false
			for _, aud := range claims.Audience {
				parts := strings.SplitN(aud, ":", 2)
				if len(parts) == 2 {
					action := parts[0]
					targetFileID := parts[1]
					expectedPath := fmt.Sprintf("/api/files/%s/%s", targetFileID, action)
					if r.URL.Path == expectedPath {
						authorized = true
						break
					}
				}
			}
			if !authorized {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "ticket audience not authorized for this resource"})
				return
			}
		}

		userID := "admin"
		if claims.Subject != "" {
			userID = claims.Subject
		}

		ctx := context.WithValue(r.Context(), userContextKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ForceHTTPSMiddleware redirects all plain HTTP requests to HTTPS with a 301 Permanent Redirect.
// Health check endpoint is excluded so cloud provider probes work on internal HTTP.
func (s *Server) ForceHTTPSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip redirect for health probes (internal HTTP from Koyeb/Render LB)
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		// Detect plain HTTP behind edge proxies (Koyeb, Render, Cloudflare)
		proto := r.Header.Get("X-Forwarded-Proto")
		if proto == "http" {
			target := "https://" + r.Host + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SecurityHeadersMiddleware adds OWASP defense-in-depth headers.
func (s *Server) SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// HSTS (Strict-Transport-Security)
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		// XSS filter
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		// Referrer policy: strictly no-referrer for sensitive media tickets and streams
		if r.URL.Query().Get("ticket") != "" || r.URL.Query().Get("token") != "" || strings.Contains(r.URL.Path, "/stream") || strings.Contains(r.URL.Path, "/preview") {
			w.Header().Set("Referrer-Policy", "no-referrer")
		} else {
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		}
		// Content-Security-Policy
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' blob: data:; media-src 'self' blob:; frame-src 'self' blob:;")

		// CORS handling
		origin := r.Header.Get("Origin")
		allowedOrigin := s.cfg.AllowedOrigin
		if allowedOrigin == "*" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if origin != "" {
			matched := false
			for _, o := range strings.Split(allowedOrigin, ",") {
				if strings.TrimSpace(o) == origin {
					matched = true
					break
				}
			}
			if matched {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Range")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Range, Content-Length, Accept-Ranges")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RedactSensitiveURL redacts ticket and token parameters from a URL string for safe logging.
func RedactSensitiveURL(rawURL string) string {
	sensitiveParams := []string{"ticket", "token", "password", "key", "secret", "bot_token", "auth"}
	hasSensitive := false
	for _, p := range sensitiveParams {
		if strings.Contains(strings.ToLower(rawURL), p+"=") {
			hasSensitive = true
			break
		}
	}
	if !hasSensitive {
		return rawURL
	}
	parts := strings.SplitN(rawURL, "?", 2)
	if len(parts) != 2 {
		return rawURL
	}
	base := parts[0]
	qParts := strings.Split(parts[1], "&")
	for i, qp := range qParts {
		kv := strings.SplitN(qp, "=", 2)
		if len(kv) == 2 {
			k := strings.ToLower(kv[0])
			for _, sp := range sensitiveParams {
				if k == sp {
					qParts[i] = kv[0] + "=[REDACTED]"
					break
				}
			}
		}
	}
	return base + "?" + strings.Join(qParts, "&")
}

