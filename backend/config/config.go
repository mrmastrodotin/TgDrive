package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"tgdrive/crypto"
)

type Config struct {
	// Server
	Port     int
	EnableTLS bool
	TLSCert  string
	TLSKey   string

	// Telegram
	TGBotTokens   []string
	TGChannelID   int64
	TGServerURL   string

	// Encryption
	MasterKey                      string
	BotCredentialSecret            string
	BotCredentialSecretIsFallback  bool

	// Database (SQLite local or Turso remote)
	TursoDatabaseURL string
	TursoAuthToken   string

	// Auth
	AppPassword string

	// JWT
	JWTSecret             string
	JWTSecretIsFallback   bool

	// Security
	AllowedOrigin string

	// Chunking & Telegram limits
	DefaultChunkSize int

	// Memory Budget & Concurrency (conservative defaults for 512 MiB deployments)
	CacheMaxBytes          int64
	MaxConcurrentUploads   int
	MaxConcurrentDownloads int
	MaxUserConcurrency     int
	MaxMemoryBudgetBytes   int64
}

// loadDotEnv tries to load key=value from .env in current or parent directory if os env is not set.
func loadDotEnv() {
	paths := []string{".env", "../.env"}
	for _, p := range paths {
		f, err := os.Open(p)
		if err == nil {
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					val := strings.TrimSpace(parts[1])
					val = strings.Trim(val, `"'`)
					if os.Getenv(key) == "" {
						os.Setenv(key, val)
					}
				}
			}
			f.Close()
			break
		}
	}
}

func Load() (*Config, error) {
	loadDotEnv()

	port := 8080
	if p := os.Getenv("PORT"); p != "" {
		parsed, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid PORT: %w", err)
		}
		port = parsed
	}

	botTokensStr := os.Getenv("TG_BOT_TOKENS")
	if botTokensStr == "" {
		// Fallback for single bot configuration
		botTokensStr = os.Getenv("TG_BOT_TOKEN")
	}

	var botTokens []string
	for _, token := range strings.Split(botTokensStr, ",") {
		token = strings.TrimSpace(token)
		if token != "" {
			botTokens = append(botTokens, token)
		}
	}

	if len(botTokens) == 0 {
		return nil, fmt.Errorf("TG_BOT_TOKENS or TG_BOT_TOKEN is required")
	}

	channelIDStr := os.Getenv("TG_CHANNEL_ID")
	if channelIDStr == "" {
		return nil, fmt.Errorf("TG_CHANNEL_ID is required")
	}
	channelID, err := strconv.ParseInt(channelIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid TG_CHANNEL_ID: %w", err)
	}
	// Telegram supergroup/channel IDs require -100 prefix
	if !strings.HasPrefix(channelIDStr, "-100") {
		if channelID < 0 {
			channelID, _ = strconv.ParseInt(fmt.Sprintf("-100%d", -channelID), 10, 64)
		} else {
			channelID, _ = strconv.ParseInt(fmt.Sprintf("-100%d", channelID), 10, 64)
		}
	}

	masterKey := os.Getenv("ENCRYPTION_SECRET_KEY")
	if masterKey == "" {
		masterKey = os.Getenv("ENCRYPTION_SECRET")
	}
	if masterKey == "" {
		return nil, fmt.Errorf("ENCRYPTION_SECRET is required")
	}

	// Pad or derive key if less than 32 chars for user convenience
	for len(masterKey) < 32 {
		masterKey += "!tgdrive_salt_padding_key_32bytes"
	}

	tursoURL := os.Getenv("TURSO_DATABASE_URL")
	tursoAuth := os.Getenv("TURSO_AUTH_TOKEN")
	isProduction := os.Getenv("ENV") == "production" || os.Getenv("REQUIRE_PERSISTENT_DB") == "true"

	if tursoURL != "" {
		if (strings.HasPrefix(tursoURL, "libsql://") || strings.HasPrefix(tursoURL, "https://")) && tursoAuth == "" {
			return nil, fmt.Errorf("TURSO_AUTH_TOKEN is required when TURSO_DATABASE_URL is configured for Turso Cloud (%s)", tursoURL)
		}
	} else if isProduction && os.Getenv("ALLOW_LOCAL_SQLITE") != "true" {
		return nil, fmt.Errorf("production startup requires a persistent Turso database (TURSO_DATABASE_URL and TURSO_AUTH_TOKEN) to prevent authoritative metadata loss on ephemeral container filesystems; to explicitly permit local SQLite with a mounted persistent volume, set ALLOW_LOCAL_SQLITE=true")
	}

	dbURL := tursoURL
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_PATH")
	}
	if dbURL == "" {
		dbURL = "tgdrive.db"
	}

	appPassword := os.Getenv("APP_PASSWORD")
	if appPassword == "" {
		appPassword = os.Getenv("ADMIN_PASSWORD")
	}
	if appPassword == "" {
		return nil, fmt.Errorf("ADMIN_PASSWORD is required")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	jwtIsFallback := false
	if jwtSecret == "" {
		jwtSecret = masterKey[:32]
		jwtIsFallback = true
	}

	enableTLS := false
	if tlsStr := os.Getenv("ENABLE_TLS"); tlsStr == "true" || tlsStr == "1" {
		enableTLS = true
	}
	tlsCert := os.Getenv("TLS_CERT_FILE")
	tlsKey := os.Getenv("TLS_KEY_FILE")

	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "*"
	}

	defaultChunkSize := 16 * 1024 * 1024 // 16 MiB: comfortably below Telegram Bot API 20MB getFile limit
	chunkSizeEnv := os.Getenv("CHUNK_SIZE")
	if chunkSizeEnv == "" {
		chunkSizeEnv = os.Getenv("DEFAULT_CHUNK_SIZE") // backward compatibility fallback
	}
	if chunkSizeEnv != "" {
		if parsed, err := strconv.Atoi(chunkSizeEnv); err == nil && parsed > 0 {
			defaultChunkSize = parsed
		}
	}

	cacheMaxBytes := int64(64 * 1024 * 1024) // 64 MiB default for 512 MiB RAM environments
	if cmbStr := os.Getenv("CACHE_MAX_BYTES"); cmbStr != "" {
		if parsed, err := strconv.ParseInt(cmbStr, 10, 64); err == nil && parsed > 0 {
			cacheMaxBytes = parsed
		}
	}

	maxConcurrentUploads := 4 // 4 in-flight chunks max across server
	if mcuStr := os.Getenv("MAX_CONCURRENT_UPLOADS"); mcuStr != "" {
		if parsed, err := strconv.Atoi(mcuStr); err == nil && parsed > 0 {
			maxConcurrentUploads = parsed
		}
	}

	maxConcurrentDownloads := 4 // 4 in-flight streams max across server
	if mcdStr := os.Getenv("MAX_CONCURRENT_DOWNLOADS"); mcdStr != "" {
		if parsed, err := strconv.Atoi(mcdStr); err == nil && parsed > 0 {
			maxConcurrentDownloads = parsed
		}
	}

	maxUserConcurrency := 6 // 6 in-flight chunks max per user session
	if mucStr := os.Getenv("MAX_USER_CONCURRENCY"); mucStr != "" {
		if parsed, err := strconv.Atoi(mucStr); err == nil && parsed > 0 {
			maxUserConcurrency = parsed
		}
	}

	maxMemoryBudgetBytes := int64(384 * 1024 * 1024) // 384 MiB admission control budget for 512 MiB containers
	if mmbStr := os.Getenv("MAX_MEMORY_BUDGET_BYTES"); mmbStr != "" {
		if parsed, err := strconv.ParseInt(mmbStr, 10, 64); err == nil && parsed > 0 {
			maxMemoryBudgetBytes = parsed
		}
	}

	botCredSecret := os.Getenv("BOT_CREDENTIAL_SECRET")
	isFallback := false
	if botCredSecret == "" {
		if os.Getenv("ENV") == "production" || os.Getenv("REQUIRE_BOT_CREDENTIAL_SECRET") == "true" {
			return nil, fmt.Errorf("BOT_CREDENTIAL_SECRET is required in production: stored bot credentials must use a separately managed secret from ENCRYPTION_SECRET")
		}
		botCredSecret = masterKey
		isFallback = true
	}

	return &Config{
		Port:                          port,
		EnableTLS:                     enableTLS,
		TLSCert:                       tlsCert,
		TLSKey:                        tlsKey,
		TGBotTokens:                   botTokens,
		TGChannelID:                   channelID,
		TGServerURL:                   os.Getenv("TG_LOCAL_SERVER_URL"),
		MasterKey:                     masterKey,
		BotCredentialSecret:           botCredSecret,
		BotCredentialSecretIsFallback: isFallback,
		TursoDatabaseURL:              dbURL,
		TursoAuthToken:                os.Getenv("TURSO_AUTH_TOKEN"),
		AppPassword:                   appPassword,
		JWTSecret:                     jwtSecret,
		JWTSecretIsFallback:           jwtIsFallback,
		AllowedOrigin:                 allowedOrigin,
		DefaultChunkSize:       defaultChunkSize,
		CacheMaxBytes:          cacheMaxBytes,
		MaxConcurrentUploads:   maxConcurrentUploads,
		MaxConcurrentDownloads: maxConcurrentDownloads,
		MaxUserConcurrency:     maxUserConcurrency,
		MaxMemoryBudgetBytes:   maxMemoryBudgetBytes,
	}, nil
}

// GetCredentialKey derives the 256-bit AES key used for encrypting stored bot credentials at rest.
func (c *Config) GetCredentialKey() []byte {
	return crypto.DeriveCredentialKey(c.BotCredentialSecret)
}

