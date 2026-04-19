package main

import (
	"log/slog"
	"testing"
)

func TestNormalizeBasePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"normal", "/iac", "/iac"},
		{"trailing slash", "/iac/", "/iac"},
		{"no leading slash", "iac", "/iac"},
		{"both issues", "iac/", "/iac"},
		{"just slash", "/", ""},
		{"multi-segment", "/docs/schemas", "/docs/schemas"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeBasePath(tt.input); got != tt.expected {
				t.Errorf("normalizeBasePath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestInitLogger_JSONForRun(t *testing.T) {
	orig := slog.Default()
	defer slog.SetDefault(orig)
	initLogger("run")
	handler := slog.Default().Handler()
	if _, ok := handler.(*slog.JSONHandler); !ok {
		t.Errorf("expected JSONHandler for 'run', got %T", handler)
	}
}

func TestInitLogger_JSONForWatch(t *testing.T) {
	orig := slog.Default()
	defer slog.SetDefault(orig)
	initLogger("watch")
	handler := slog.Default().Handler()
	if _, ok := handler.(*slog.JSONHandler); !ok {
		t.Errorf("expected JSONHandler for 'watch', got %T", handler)
	}
}

func TestInitLogger_TextForPreview(t *testing.T) {
	orig := slog.Default()
	defer slog.SetDefault(orig)
	initLogger("preview")
	handler := slog.Default().Handler()
	if _, ok := handler.(*slog.TextHandler); !ok {
		t.Errorf("expected TextHandler for 'preview', got %T", handler)
	}
}
