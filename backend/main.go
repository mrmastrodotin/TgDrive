package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"tgdrive/api"
	"tgdrive/config"
	"tgdrive/storage"
	"tgdrive/telegram"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("⚡ TgDrive starting...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("FATAL: config: %v", err)
	}

	// Initialize database
	if strings.HasPrefix(cfg.TursoDatabaseURL, "libsql://") || strings.HasPrefix(cfg.TursoDatabaseURL, "https://") {
		log.Printf("🗄️ Connecting to persistent Turso cloud database (%s)...", cfg.TursoDatabaseURL)
	} else {
		log.Printf("🗄️ Connecting to local SQLite database (%s)...", cfg.TursoDatabaseURL)
	}
	db, err := storage.New(cfg.TursoDatabaseURL, cfg.TursoAuthToken)
	if err != nil {
		log.Fatalf("FATAL: database: %v", err)
	}
	defer db.Close()
	if strings.HasPrefix(cfg.TursoDatabaseURL, "libsql://") || strings.HasPrefix(cfg.TursoDatabaseURL, "https://") {
		log.Println("✅ Persistent Turso cloud database connection verified")
	} else {
		log.Println("✅ Local SQLite database connection verified")
	}

	// Configure authenticated encryption at rest for Telegram bot credentials
	if cfg.BotCredentialSecretIsFallback {
		log.Println("⚠️ COMPATIBILITY WARNING: BOT_CREDENTIAL_SECRET is unset. Using legacy fallback to ENCRYPTION_SECRET in migration/compatibility mode. In production, configure a dedicated BOT_CREDENTIAL_SECRET.")
	}
	if cfg.JWTSecretIsFallback {
		log.Println("⚠️ NOTICE: JWT_SECRET is unset. Deriving authentication token secret from ENCRYPTION_SECRET. For independent key rotation, configure a dedicated JWT_SECRET.")
	}
	db.SetCredentialKey(cfg.GetCredentialKey())

	// Run migrations
	ctx := context.Background()
	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("FATAL: migrate: %v", err)
	}
	log.Println("✅ Database migrated")

	// Migrate legacy unencrypted bot credentials to authenticated ciphertext at rest
	if err := db.MigrateBotCredentials(ctx, cfg.GetCredentialKey()); err != nil {
		log.Printf("⚠️ Bot credential migration: %v", err)
	}

	// Initialize Telegram Bot Pool
	tg, err := telegram.NewBotPool(cfg.TGBotTokens, cfg.TGChannelID, cfg.TGServerURL)
	if err != nil {
		log.Fatalf("FATAL: telegram pool: %v", err)
	}

	// Load dynamically registered bots from DB into pool (failing safely if any cannot be decrypted)
	dbBots, err := db.GetAllBots(ctx)
	if err == nil {
		for _, b := range dbBots {
			if !b.Disabled {
				if b.Token == "" {
					displayHash := b.TokenHash
					if len(displayHash) > 8 {
						displayHash = displayHash[:8]
					}
					log.Printf("⚠️ WARNING: stored bot %s could not be decrypted; skipping pool registration safely", displayHash)
					continue
				}
				_ = tg.AddBot(b.OwnerID, b.Token)
			}
		}
	}


	if err := tg.VerifyAccess(ctx); err != nil {
		log.Fatalf("FATAL: telegram access: %v", err)
	}
	log.Println("✅ Telegram channel access verified")

	// Create API server
	srv := api.NewServer(cfg, db, tg)

	// Modern TLS 1.2 / 1.3 configuration
	tlsConfig := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.X25519, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}

	// Create HTTP/HTTPS server with defense timeouts
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      srv.Handler(),
		TLSConfig:    tlsConfig,
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		if cfg.EnableTLS && cfg.TLSCert != "" && cfg.TLSKey != "" {
			log.Printf("🔒 HTTPS Server listening on https://localhost:%d (TLS 1.2/1.3)", cfg.Port)
			if err := httpServer.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey); err != nil && err != http.ErrServerClosed {
				log.Fatalf("FATAL: https server: %v", err)
			}
		} else {
			log.Printf("🌐 HTTP Server listening on http://localhost:%d", cfg.Port)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("FATAL: http server: %v", err)
			}
		}
	}()

	<-done
	log.Println("🛑 Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("ERROR: shutdown: %v", err)
	}

	log.Println("👋 Server stopped")
}
