package logging

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

func New(w io.Writer, level string) *slog.Logger {
	l := slog.LevelInfo
	switch strings.ToUpper(level) {
	case "DEBUG":
		l = slog.LevelDebug
	case "WARN":
		l = slog.LevelWarn
	case "ERROR":
		l = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: l}))
}

type Temporal struct{ Log *slog.Logger }

func (l Temporal) Debug(msg string, args ...interface{}) { l.Log.Debug(msg, args...) }
func (l Temporal) Info(msg string, args ...interface{})  { l.Log.Info(msg, args...) }
func (l Temporal) Warn(msg string, args ...interface{})  { l.Log.Warn(msg, args...) }
func (l Temporal) Error(msg string, args ...interface{}) { l.Log.Error(msg, args...) }

type loggerKey struct{}

// WithLogger передаёт диагностические поля через границы прикладных пакетов.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
