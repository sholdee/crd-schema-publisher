package publisher

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sholdee/crd-schema-publisher/diagnostics"
)

const (
	maxBucketSize     = 40 * 1024 * 1024
	maxBucketFiles    = 2000
	uploadConcurrency = 3
	maxUploadRetries  = 5
	maxDeployRetries  = 3
	httpTimeout       = 120 * time.Second
	cfBaseURL         = "https://api.cloudflare.com/client/v4"
)

type Publisher struct {
	APIToken              string
	AccountID             string
	ProjectName           string
	BaseURL               string
	AssetsURL             string
	HTTPClient            *http.Client
	SleepFunc             func(time.Duration)
	Profiler              diagnostics.Snapshotter
	UploadBucketSizeBytes int64
	UploadConcurrency     int
}

type uploadBucket struct {
	files []*fileEntry
	size  int64
}

func (p *Publisher) sleepFunc() func(time.Duration) {
	if p.SleepFunc != nil {
		return p.SleepFunc
	}
	return time.Sleep
}

type fileEntry struct {
	relPath     string
	absPath     string
	hash        string
	size        int64
	contentType string
}

type cfResponse struct {
	Success bool              `json:"success"`
	Result  json.RawMessage   `json:"result"`
	Errors  []json.RawMessage `json:"errors"`
}

func (p *Publisher) baseURL() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}
	return cfBaseURL
}

func (p *Publisher) assetsURL() string {
	if p.AssetsURL != "" {
		return p.AssetsURL
	}
	return cfBaseURL
}

func (p *Publisher) httpClient() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: httpTimeout}
}

func (p *Publisher) Publish(dir string) error {
	p.snapshot("upload.start", "dir", dir)
	if err := p.ensureProject(); err != nil {
		return fmt.Errorf("ensuring project: %w", err)
	}
	p.snapshot("upload.after-ensure-project", "dir", dir)
	files, err := p.collectActiveFiles(dir)
	if err != nil {
		return err
	}
	p.snapshot("upload.after-collect-files", "file_count", len(files))
	if len(files) == 0 {
		return fmt.Errorf("no files found in %s", dir)
	}
	slog.Info("collected files", "count", len(files))

	jwt, err := p.getUploadToken()
	if err != nil {
		return fmt.Errorf("getting upload token: %w", err)
	}

	hashToFile, uniqueHashes := buildUploadPlan(files)
	p.snapshot("upload.after-upload-plan", "file_count", len(files), "unique_hashes", len(uniqueHashes))

	missing, err := p.checkMissing(jwt, uniqueHashes)
	if err != nil {
		return fmt.Errorf("checking missing: %w", err)
	}
	p.snapshot("upload.after-check-missing", "missing", len(missing), "cached", len(uniqueHashes)-len(missing))
	slog.Info("uploading files", "new", len(missing), "cached", len(uniqueHashes)-len(missing))

	uploaded := 0
	if len(missing) > 0 {
		toUpload := selectUploadFiles(hashToFile, missing)
		if err := p.uploadFiles(jwt, toUpload); err != nil {
			return fmt.Errorf("uploading files: %w", err)
		}
		uploaded = len(toUpload)
	}
	p.snapshot("upload.after-upload-files", "uploaded", uploaded)

	if err := p.upsertHashes(jwt, uniqueHashes); err != nil {
		return fmt.Errorf("upserting hashes: %w", err)
	}
	p.snapshot("upload.after-upsert-hashes", "unique_hashes", len(uniqueHashes))

	url, err := p.createDeployment(buildManifest(files))
	if err != nil {
		return fmt.Errorf("creating deployment: %w", err)
	}
	p.snapshot("upload.after-create-deployment", "file_count", len(files))
	slog.Info("deployment successful", "url", url)
	return nil
}

func (p *Publisher) snapshot(phase string, attrs ...any) {
	if p.Profiler != nil {
		p.Profiler.Snapshot(phase, attrs...)
	}
}

func (p *Publisher) uploadConfig() UploadConfig {
	cfg := DefaultUploadConfig()
	if p.UploadBucketSizeBytes > 0 {
		cfg.BucketSizeBytes = p.UploadBucketSizeBytes
	}
	if p.UploadConcurrency > 0 {
		cfg.Concurrency = p.UploadConcurrency
	}
	return cfg
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

func (p *Publisher) ensureProject() error {
	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s", p.baseURL(), p.AccountID, p.ProjectName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.APIToken)
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	var cr cfResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	if cr.Success {
		return nil
	}

	slog.Info("creating pages project", "project", p.ProjectName)
	body, err := json.Marshal(map[string]string{"name": p.ProjectName, "production_branch": "production"})
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}
	createURL := fmt.Sprintf("%s/accounts/%s/pages/projects", p.baseURL(), p.AccountID)
	req, err = http.NewRequest("POST", createURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.APIToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = p.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	if !cr.Success {
		return fmt.Errorf("failed to create project: %s", cr.Errors)
	}
	return nil
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

func (p *Publisher) getUploadToken() (string, error) {
	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/upload-token", p.baseURL(), p.AccountID, p.ProjectName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.APIToken)
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var cr cfResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}
	if !cr.Success {
		return "", fmt.Errorf("failed to get upload token: %s", cr.Errors)
	}
	var result struct {
		JWT string `json:"jwt"`
	}
	if err := json.Unmarshal(cr.Result, &result); err != nil {
		return "", fmt.Errorf("parsing upload token: %w", err)
	}
	if result.JWT == "" {
		return "", fmt.Errorf("upload token response contained empty JWT")
	}
	return result.JWT, nil
}

func (p *Publisher) checkMissing(jwt string, hashes []string) ([]string, error) {
	body, err := json.Marshal(map[string][]string{"hashes": hashes})
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}
	url := fmt.Sprintf("%s/pages/assets/check-missing", p.assetsURL())
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var cr cfResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if !cr.Success {
		return nil, fmt.Errorf("check-missing failed: %s", cr.Errors)
	}
	var missing []string
	if err := json.Unmarshal(cr.Result, &missing); err != nil {
		return nil, fmt.Errorf("parsing missing hashes: %w", err)
	}
	return missing, nil
}

func (p *Publisher) uploadFiles(jwt string, files []*fileEntry) error {
	buckets := p.planUploadBuckets(files)
	concurrency := p.uploadConfig().Concurrency

	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	for i, b := range buckets {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, b uploadBucket) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := p.uploadBucketWithIndex(jwt, b.files, idx); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("bucket %d: %w", idx, err)
				}
				mu.Unlock()
			}
		}(i, b)
	}
	wg.Wait()
	return firstErr
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

func (p *Publisher) uploadBucket(jwt string, files []*fileEntry) error {
	return p.uploadBucketWithIndex(jwt, files, -1)
}

func (p *Publisher) uploadBucketWithIndex(jwt string, files []*fileEntry, bucketIndex int) error {
	body, err := buildUploadBucketBody(files)
	if err != nil {
		return fmt.Errorf("marshaling upload payload: %w", err)
	}
	if bucketIndex >= 0 {
		p.snapshot(fmt.Sprintf("upload.bucket.%d.after-marshal", bucketIndex), "files", len(files), "body_bytes", len(body))
	} else {
		p.snapshot("upload.bucket.after-marshal", "files", len(files), "body_bytes", len(body))
	}
	url := fmt.Sprintf("%s/pages/assets/upload", p.assetsURL())
	var lastErr error
	for attempt := range maxUploadRetries {
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("building request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+jwt)
		req.Header.Set("Content-Type", "application/json")
		resp, err := p.httpClient().Do(req)
		if err != nil {
			lastErr = err
			slog.Warn("upload attempt failed, retrying", "attempt", attempt+1, "max", maxUploadRetries, "error", lastErr)
			p.sleepFunc()(time.Duration(math.Pow(2, float64(attempt))) * time.Second)
			continue
		}
		var cr cfResponse
		if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("decoding response: %w", err)
			slog.Warn("upload attempt failed, retrying", "attempt", attempt+1, "max", maxUploadRetries, "error", lastErr)
			p.sleepFunc()(time.Duration(math.Pow(2, float64(attempt))) * time.Second)
			continue
		}
		_ = resp.Body.Close()
		if cr.Success {
			return nil
		}
		lastErr = fmt.Errorf("upload failed: %s", cr.Errors)
		if resp.StatusCode >= 500 {
			slog.Warn("upload attempt failed, retrying", "attempt", attempt+1, "max", maxUploadRetries, "error", lastErr)
			p.sleepFunc()(time.Duration(math.Pow(2, float64(attempt))) * time.Second)
			continue
		}
		return lastErr
	}
	return fmt.Errorf("upload failed after %d retries: %w", maxUploadRetries, lastErr)
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

func (p *Publisher) upsertHashes(jwt string, hashes []string) error {
	body, err := json.Marshal(map[string][]string{"hashes": hashes})
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}
	url := fmt.Sprintf("%s/pages/assets/upsert-hashes", p.assetsURL())
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	var cr cfResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	if !cr.Success {
		return fmt.Errorf("upsert-hashes failed: %s", cr.Errors)
	}
	return nil
}

func (p *Publisher) createDeployment(manifest map[string]string) (string, error) {
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("marshaling manifest: %w", err)
	}
	var lastErr error
	for attempt := range maxDeployRetries {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormField("manifest")
		if err != nil {
			return "", err
		}
		_, _ = part.Write(manifestJSON)
		_ = writer.Close()
		url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/deployments", p.baseURL(), p.AccountID, p.ProjectName)
		req, err := http.NewRequest("POST", url, &body)
		if err != nil {
			return "", fmt.Errorf("building request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+p.APIToken)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		resp, err := p.httpClient().Do(req)
		if err != nil {
			lastErr = err
			slog.Warn("deployment attempt failed, retrying", "attempt", attempt+1, "max", maxDeployRetries, "error", lastErr)
			p.sleepFunc()(time.Duration(math.Pow(2, float64(attempt))) * time.Second)
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var cr cfResponse
		if err := json.Unmarshal(respBody, &cr); err != nil {
			lastErr = fmt.Errorf("decoding response: %w", err)
			slog.Warn("deployment attempt failed, retrying", "attempt", attempt+1, "max", maxDeployRetries, "error", lastErr)
			p.sleepFunc()(time.Duration(math.Pow(2, float64(attempt))) * time.Second)
			continue
		}
		if cr.Success {
			var result struct {
				URL string `json:"url"`
			}
			if err := json.Unmarshal(cr.Result, &result); err != nil {
				return "", fmt.Errorf("parsing deployment URL: %w", err)
			}
			return result.URL, nil
		}
		lastErr = fmt.Errorf("deployment failed: %s", string(respBody))
		if resp.StatusCode >= 500 {
			slog.Warn("deployment attempt failed, retrying", "attempt", attempt+1, "max", maxDeployRetries, "error", lastErr)
			p.sleepFunc()(time.Duration(math.Pow(2, float64(attempt))) * time.Second)
			continue
		}
		return "", lastErr
	}
	return "", fmt.Errorf("deployment failed after %d retries: %w", maxDeployRetries, lastErr)
}
