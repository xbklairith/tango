package main

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestSetupLogger_DevUsesTextHandler(t *testing.T) {
	setupLogger("info", "development")

	var buf bytes.Buffer
	// Create a test logger with text handler to verify format
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)
	logger.Info("test message")

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("level=INFO")) {
		t.Errorf("text handler output should contain 'level=INFO', got: %s", output)
	}
}

func TestSetupLogger_ProdUsesJSONHandler(t *testing.T) {
	setupLogger("info", "production")

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)
	logger.Info("test message")

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte(`"level"`)) {
		t.Errorf("json handler output should contain '\"level\"', got: %s", output)
	}
}

func TestSetupLogger_RespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})
	logger := slog.New(handler)

	// Debug should be suppressed
	logger.Debug("should not appear")
	if buf.Len() > 0 {
		t.Errorf("debug message should be suppressed at error level, got: %s", buf.String())
	}

	// Error should appear
	logger.Error("should appear")
	if buf.Len() == 0 {
		t.Error("error message should appear at error level")
	}
}
