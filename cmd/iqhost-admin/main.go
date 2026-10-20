package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"temporal-iqhost/internal/admin"
	"temporal-iqhost/internal/logging"
	"temporal-iqhost/internal/postgres"
)

func main() {
	slog.SetDefault(logging.New(os.Stdout, os.Getenv("LOG_LEVEL")))
	if err := run(); err != nil {
		slog.Error("admin failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	listen := flag.String("listen", "127.0.0.1:8082", "HTTP listen address")
	staticDir := flag.String("static-dir", "web/admin/dist", "Frontend build directory")
	devOrigin := flag.String("dev-origin", "", "Allowed Vite origin")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://iqhost:iqhost-local@localhost:5432/iqhost?sslmode=disable"
	}
	store, err := postgres.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer store.Pool.Close()
	handler := &admin.Server{Store: store, StaticDir: *staticDir, DevOrigin: *devOrigin}
	server := &http.Server{
		Addr:              *listen,
		Handler:           handler.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	stopped := make(chan error, 1)
	go func() { stopped <- server.ListenAndServe() }()
	slog.Info("admin listening", "address", *listen)
	select {
	case serveErr := <-stopped:
		return serveErr
	case <-ctx.Done():
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err = server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return err
	}
	err = <-stopped
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
