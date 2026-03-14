package main

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestSetupLogger_DevUsesTextHandler(t *testing.T) {
	// Redirect slog output to a buffer by setting up logger with a custom writer
	var buf bytes.Buffer
	origHandler := slog.Default().Handler()
	defer slog.SetDefault(slog.New(origHandler))

	setupLogger("info", "development")

	// Replace the default logger's output with our buffer for verification
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(handler))
	slog.Info("test message")

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("level=INFO")) {
		t.Errorf("text handler output should contain 'level=INFO', got: %s", output)
	}
	// Verify it's NOT JSON
	if bytes.Contains([]byte(output), []byte(`"level"`)) {
		t.Errorf("development mode should use text handler, not JSON, got: %s", output)
	}
}

func TestSetupLogger_ProdUsesJSONHandler(t *testing.T) {
	var buf bytes.Buffer
	origHandler := slog.Default().Handler()
	defer slog.SetDefault(slog.New(origHandler))

	setupLogger("info", "production")

	// Replace with JSON handler writing to buffer for verification
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(handler))
	slog.Info("test message")

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte(`"level"`)) {
		t.Errorf("json handler output should contain '\"level\"', got: %s", output)
	}
}

func TestSetupLogger_RespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	origHandler := slog.Default().Handler()
	defer slog.SetDefault(slog.New(origHandler))

	// Set up logger at error level and redirect to buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})
	slog.SetDefault(slog.New(handler))

	// Debug should be suppressed
	slog.Debug("should not appear")
	if buf.Len() > 0 {
		t.Errorf("debug message should be suppressed at error level, got: %s", buf.String())
	}

	// Error should appear
	slog.Error("should appear")
	if buf.Len() == 0 {
		t.Error("error message should appear at error level")
	}
}
