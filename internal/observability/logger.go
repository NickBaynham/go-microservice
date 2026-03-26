package observability

import (
	"log/slog"
	"os"
	"strings"
)

// NewLogger returns the application slog.Logger. Use JSON output for production log pipelines;
// text is easier in local development.
func NewLogger(levelStr string, jsonLogs bool) *slog.Logger {
	lvl := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler
	if jsonLogs {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(h)
}
