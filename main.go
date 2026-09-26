package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/remyhuang03/sja-backend/internal/httpapi"
	"github.com/remyhuang03/sja-backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	dir := os.Getenv("DATA_DIR")
	if dir == "" {
		dir = "./var"
	}
	for _, sub := range []string{"reports", "media"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			return err
		}
	}
	if os.Getenv("DATABASE_URL") == "" {
		return errors.New("DATABASE_URL is required")
	}
	startup, stop := context.WithTimeout(ctx, 30*time.Second)
	defer stop()
	db, err := store.Open(startup, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Pool.Close()
	if err = db.Migrate(startup); err != nil {
		return err
	}
	token := os.Getenv("ADMIN_TOKEN")
	if token != "" && len(token) < 32 {
		return errors.New("ADMIN_TOKEN must contain at least 32 characters")
	}
	api := httpapi.New(db, dir, token)
	go api.RunCleanup(ctx)
	port := os.Getenv("BACKEND_PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{Addr: ":" + port, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 120 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan error, 1)
	go func() { slog.Info("listening", "port", port); done <- server.ListenAndServe() }()
	select {
	case err = <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, c := context.WithTimeout(context.Background(), 20*time.Second)
		defer c()
		return server.Shutdown(shutdown)
	}
}
