package publisher

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeebo/blake3"
)

// HashFile computes a file hash matching wrangler's hashFile function:
// hex(blake3(base64(content) + extension))[0:32]
func HashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening file %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	hash, err := HashReaderWithExtension(file, ext)
	if err != nil {
		return "", fmt.Errorf("hashing file %s: %w", path, err)
	}
	return hash, nil
}

func HashReaderWithExtension(r io.Reader, ext string) (string, error) {
	hasher := blake3.New()
	encoder := base64.NewEncoder(base64.StdEncoding, hasher)
	if _, err := io.Copy(encoder, r); err != nil {
		_ = encoder.Close()
		return "", err
	}
	if err := encoder.Close(); err != nil {
		return "", err
	}
	_, _ = hasher.WriteString(ext)
	sum := hasher.Sum(nil)
	return hex.EncodeToString(sum)[:32], nil
}
