package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// Server
	Port     int
	EnableTLS bool
	TLSCert  string
	TLSKey   string

	// Telegram
	TGBotToken  string
	TGChannelID int64

	// Encryption
	MasterKey string

	// Database (SQLite local or Turso remote)
	TursoDatabaseURL string
	TursoAuthToken   string

	// Auth
	AppPassword string

	// JWT
	JWTSecret string

	// Security
	AllowedOrigin string
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

	botToken := os.Getenv("TG_BOT_TOKEN")
	if botToken == "" {
		return nil, fmt.Errorf("TG_BOT_TOKEN is required")
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

	dbURL := os.Getenv("TURSO_DATABASE_URL")
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
	if jwtSecret == "" {
		jwtSecret = masterKey[:32]
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

	return &Config{
		Port:             port,
		EnableTLS:        enableTLS,
		TLSCert:          tlsCert,
		TLSKey:           tlsKey,
		TGBotToken:       botToken,
		TGChannelID:      channelID,
		MasterKey:        masterKey,
		TursoDatabaseURL: dbURL,
		TursoAuthToken:   os.Getenv("TURSO_AUTH_TOKEN"),
		AppPassword:      appPassword,
		JWTSecret:        jwtSecret,
		AllowedOrigin:    allowedOrigin,
	}, nil
}
