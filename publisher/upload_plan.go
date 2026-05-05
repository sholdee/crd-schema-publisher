package publisher

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

type uploadBucket struct {
	files []*fileEntry
	size  int64
}

type fileEntry struct {
	relPath     string
	absPath     string
	hash        string
	size        int64
	contentType string
}

func (p *Publisher) collectActiveFiles(dir string) ([]*fileEntry, error) {
	activeDir, err := filepath.EvalSymlinks(filepath.Join(dir, "current"))
	if err != nil {
		return nil, fmt.Errorf("resolving active output: %w", err)
	}
	files, err := p.collectFiles(activeDir)
	if err != nil {
		return nil, fmt.Errorf("collecting files: %w", err)
	}
	return files, nil
}

func buildUploadPlan(files []*fileEntry) (map[string]*fileEntry, []string) {
	hashToFile := map[string]*fileEntry{}
	for _, f := range files {
		hashToFile[f.hash] = f
	}
	uniqueHashes := make([]string, 0, len(hashToFile))
	for h := range hashToFile {
		uniqueHashes = append(uniqueHashes, h)
	}
	return hashToFile, uniqueHashes
}

func selectUploadFiles(hashToFile map[string]*fileEntry, missing []string) []*fileEntry {
	toUpload := make([]*fileEntry, 0, len(missing))
	for _, h := range missing {
		if f, ok := hashToFile[h]; ok {
			toUpload = append(toUpload, f)
		}
	}
	return toUpload
}

func buildManifest(files []*fileEntry) map[string]string {
	manifest := make(map[string]string, len(files))
	for _, f := range files {
		manifest["/"+f.relPath] = f.hash
	}
	return manifest
}

func (p *Publisher) collectFiles(dir string) ([]*fileEntry, error) {
	var files []*fileEntry
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if shouldSkipPublishedFile(rel) {
			return nil
		}
		hash, err := HashFile(path)
		if err != nil {
			return err
		}
		ct := mime.TypeByExtension(filepath.Ext(path))
		if ct == "" {
			ct = "application/octet-stream"
		}
		files = append(files, &fileEntry{relPath: rel, absPath: path, hash: hash, size: info.Size(), contentType: ct})
		return nil
	})
	return files, err
}

func shouldSkipPublishedFile(rel string) bool {
	rel = filepath.ToSlash(rel)
	return filepath.Ext(rel) == ".kind" || strings.HasPrefix(rel, "_meta/")
}

func (p *Publisher) planUploadBuckets(files []*fileEntry) []uploadBucket {
	maxSize := p.uploadConfig().BucketSizeBytes
	var buckets []uploadBucket
	current := uploadBucket{}
	for _, f := range files {
		if len(current.files) >= maxBucketFiles || current.size+f.size > maxSize {
			if len(current.files) > 0 {
				buckets = append(buckets, current)
			}
			current = uploadBucket{}
		}
		current.files = append(current.files, f)
		current.size += f.size
	}
	if len(current.files) > 0 {
		buckets = append(buckets, current)
	}
	return buckets
}

func buildUploadBucketBody(files []*fileEntry) ([]byte, error) {
	var body bytes.Buffer
	if size := estimateUploadBucketBodySize(files); size > 0 {
		body.Grow(size)
	}

	body.WriteByte('[')
	for i, f := range files {
		if i > 0 {
			body.WriteByte(',')
		}
		if err := writeUploadItem(&body, f); err != nil {
			return nil, err
		}
	}
	body.WriteByte(']')

	return body.Bytes(), nil
}

func writeUploadItem(body *bytes.Buffer, f *fileEntry) error {
	body.WriteString(`{"key":`)
	if err := writeJSONString(body, f.hash); err != nil {
		return err
	}
	body.WriteString(`,"value":"`)
	if err := writeBase64File(body, f.absPath); err != nil {
		return err
	}
	body.WriteString(`","metadata":{"contentType":`)
	if err := writeJSONString(body, f.contentType); err != nil {
		return err
	}
	body.WriteString(`},"base64":true}`)
	return nil
}

func writeJSONString(body *bytes.Buffer, value string) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, _ = body.Write(encoded)
	return nil
}

func writeBase64File(body *bytes.Buffer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	encoder := base64.NewEncoder(base64.StdEncoding, body)
	if _, err := io.Copy(encoder, file); err != nil {
		_ = encoder.Close()
		return err
	}
	return encoder.Close()
}

func estimateUploadBucketBodySize(files []*fileEntry) int {
	total := 2
	if len(files) > 1 {
		total += len(files) - 1
	}
	const uploadItemStaticSize = len(`{"key":"","value":"","metadata":{"contentType":""},"base64":true}`)
	for _, f := range files {
		if f.size > int64(maxInt) {
			return 0
		}
		encodedLen := base64.StdEncoding.EncodedLen(int(f.size))
		next := total + uploadItemStaticSize + len(f.hash) + encodedLen + len(f.contentType)
		if next < total {
			return 0
		}
		total = next
	}
	return total
}

const maxInt = int(^uint(0) >> 1)
