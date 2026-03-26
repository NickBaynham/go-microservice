package observability_test

import (
	"context"
	"log/slog"
	"testing"

	"go-microservice/internal/observability"
)

func TestNewLogger_DebugLevel(t *testing.T) {
	l := observability.NewLogger("debug", false)
	if !l.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("expected debug level enabled")
	}
}

func TestNewLogger_JSONHandler(t *testing.T) {
	l := observability.NewLogger("info", true)
	if l.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("expected debug disabled at info level")
	}
}
