package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"temporal-iqhost/internal/logging"
	"temporal-iqhost/internal/provider/simulator"
)

func main() {
	slog.SetDefault(logging.New(os.Stdout, os.Getenv("LOG_LEVEL")))
	if err := run(); err != nil {
		slog.Error("simulator failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	listen := flag.String("listen", "127.0.0.1:8081", "HTTP listen address")
	stateFile := flag.String("state-file", "", "файл состояния симулятора (без флага — только память)")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	adapter := simulator.New()
	if *stateFile != "" {
		var err error
		adapter, err = simulator.NewPersistent(*stateFile)
		if err != nil {
			return err
		}
	}
	server := &http.Server{
		Addr:              *listen,
		Handler:           simulator.NewHandler(adapter),
		ReadHeaderTimeout: 5 * time.Second,
	}
	stopped := make(chan error, 1)
	go func() { stopped <- server.ListenAndServe() }()
	select {
	case err := <-stopped:
		return err
	case <-ctx.Done():
	}
	// Даём принятым командам закончиться перед остановкой HTTP-сервера.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return err
	}
	err := <-stopped
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
