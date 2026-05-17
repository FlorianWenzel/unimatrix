// Command unimatrix is the HTTP server for the Borg-themed social
// network of the same name.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/FlorianWenzel/unimatrix/internal/store"
	"github.com/FlorianWenzel/unimatrix/internal/web"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	dbPath := envOr("UNIMATRIX_DB", "./unimatrix.db")
	addr := envOr("UNIMATRIX_ADDR", ":8080")

	s, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	logger.Info("store opened", "path", dbPath)

	key, err := loadOrGenerateSessionKey()
	if err != nil {
		return err
	}

	srv, err := web.NewServer(s, web.Config{
		SessionKey:   key,
		CookieSecure: os.Getenv("UNIMATRIX_COOKIE_SECURE") == "1",
		Logger:       logger,
	})
	if err != nil {
		return err
	}

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdownCtx)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// loadOrGenerateSessionKey reads UNIMATRIX_SESSION_KEY (hex-encoded
// optional) or generates a random 32-byte key. In production the key
// MUST be provided so sessions survive restarts.
func loadOrGenerateSessionKey() ([]byte, error) {
	if v := os.Getenv("UNIMATRIX_SESSION_KEY"); v != "" {
		if len(v) < 32 {
			return nil, errors.New("UNIMATRIX_SESSION_KEY must be at least 32 bytes")
		}
		return []byte(v), nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	slog.Warn("UNIMATRIX_SESSION_KEY not set; generated ephemeral key (sessions will not survive restart)")
	return buf, nil
}
