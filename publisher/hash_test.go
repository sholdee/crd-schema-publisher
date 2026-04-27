package publisher

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeebo/blake3"
)

func TestHashFile_MatchesWrangler(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.json")
	_ = os.WriteFile(path, []byte(`{"type":"object"}`), 0o644)
	hash, err := HashFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hash) != 32 {
		t.Fatalf("expected 32 hex chars, got %d: %s", len(hash), hash)
	}
	hash2, err := HashFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != hash2 {
		t.Fatal("same file should produce same hash")
	}
}

func TestHashFile_DifferentExtensionDifferentHash(t *testing.T) {
	tmpDir := t.TempDir()
	content := []byte(`{"type":"object"}`)
	jsonPath := filepath.Join(tmpDir, "test.json")
	htmlPath := filepath.Join(tmpDir, "test.html")
	_ = os.WriteFile(jsonPath, content, 0o644)
	_ = os.WriteFile(htmlPath, content, 0o644)
	hashJSON, _ := HashFile(jsonPath)
	hashHTML, _ := HashFile(htmlPath)
	if hashJSON == hashHTML {
		t.Fatal("different extensions should produce different hashes")
	}
}

func TestHashFile_GoldenValues(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content []byte
		want    string
	}{
		{"json_content", "test.json", []byte(`{"type":"object"}`), "5ed65b07f4c35e72bdf5ae858e6c994f"},
		{"empty_file", "empty.json", []byte{}, "a384408d93abe898a69b6ab90c2aef05"},
		{"no_extension", "noext", []byte("hello"), "324ea05bea4d7f75b8d9ed695e65b2ca"},
		{"binary_content", "data.bin", []byte{0x00, 0xFF, 0x80, 0x01}, "e9287a7e1de263d3d6297fcbb974123a"},
		{"html_content", "page.html", []byte("<html></html>"), "4752155c2c0c0320b40bca1d83e8380a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, tt.file)
			if err := os.WriteFile(path, tt.content, 0o644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}
			got, err := HashFile(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("HashFile(%q) = %q, want %q", tt.file, got, tt.want)
			}
		})
	}
}

func TestHashFile_DifferentContentDifferentHash(t *testing.T) {
	tmpDir := t.TempDir()
	path1 := filepath.Join(tmpDir, "a.json")
	path2 := filepath.Join(tmpDir, "b.json")
	_ = os.WriteFile(path1, []byte(`{"a":1}`), 0o644)
	_ = os.WriteFile(path2, []byte(`{"b":2}`), 0o644)
	hash1, _ := HashFile(path1)
	hash2, _ := HashFile(path2)
	if hash1 == hash2 {
		t.Fatal("different content should produce different hashes")
	}
}

func TestHashReaderWithExtension_MatchesWranglerAlgorithm(t *testing.T) {
	content := []byte("streamed content with enough bytes to cross an encoder boundary")
	got, err := HashReaderWithExtension(bytes.NewReader(content), "json")
	if err != nil {
		t.Fatalf("HashReaderWithExtension returned error: %v", err)
	}

	hasher := blake3.New()
	_, _ = hasher.WriteString(base64.StdEncoding.EncodeToString(content) + "json")
	want := hex.EncodeToString(hasher.Sum(nil))[:32]

	if got != want {
		t.Fatalf("HashReaderWithExtension() = %q, want %q", got, want)
	}
}
