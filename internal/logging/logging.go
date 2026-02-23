package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if err := h.Handle(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	hs := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		hs[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: hs}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	hs := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		hs[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: hs}
}

func NewLogger(filename string) (*slog.Logger, *os.File, error) {
	file, err := os.OpenFile(
		filename,
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0o644, // TODO: consider 0o640 to restrict log file access

	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open log file: %w", err)
	}

	jsonHandler := slog.NewJSONHandler(file, &slog.HandlerOptions{
		Level: slog.LevelDebug, // TODO: make log level configurable
	})

	consoleHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo, // TODO: make log level configurable
	})

	handler := &multiHandler{
		handlers: []slog.Handler{
			jsonHandler,
			consoleHandler,
		},
	}

	return slog.New(handler), file, nil
}
