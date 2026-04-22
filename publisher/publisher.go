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
	APIToken    string
	AccountID   string
	ProjectName string
	BaseURL     string
	AssetsURL   string
	HTTPClient  *http.Client
	SleepFunc   func(time.Duration)
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
	if err := p.ensureProject(); err != nil {
		return fmt.Errorf("ensuring project: %w", err)
	}
	files, err := p.collectActiveFiles(dir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no files found in %s", dir)
	}
	slog.Info("collected files", "count", len(files))

	jwt, err := p.getUploadToken()
	if err != nil {
		return fmt.Errorf("getting upload token: %w", err)
	}

	hashToFile, uniqueHashes := buildUploadPlan(files)

	missing, err := p.checkMissing(jwt, uniqueHashes)
	if err != nil {
		return fmt.Errorf("checking missing: %w", err)
	}
	slog.Info("uploading files", "new", len(missing), "cached", len(uniqueHashes)-len(missing))

	if len(missing) > 0 {
		toUpload := selectUploadFiles(hashToFile, missing)
		if err := p.uploadFiles(jwt, toUpload); err != nil {
			return fmt.Errorf("uploading files: %w", err)
		}
	}

	if err := p.upsertHashes(jwt, uniqueHashes); err != nil {
		return fmt.Errorf("upserting hashes: %w", err)
	}

	url, err := p.createDeployment(buildManifest(files))
	if err != nil {
		return fmt.Errorf("creating deployment: %w", err)
	}
	slog.Info("deployment successful", "url", url)
	return nil
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
	type bucket struct {
		files []*fileEntry
		size  int64
	}
	var buckets []bucket
	current := bucket{}
	for _, f := range files {
		if len(current.files) >= maxBucketFiles || current.size+f.size > maxBucketSize {
			if len(current.files) > 0 {
				buckets = append(buckets, current)
			}
			current = bucket{}
		}
		current.files = append(current.files, f)
		current.size += f.size
	}
	if len(current.files) > 0 {
		buckets = append(buckets, current)
	}

	sem := make(chan struct{}, uploadConcurrency)
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	for i, b := range buckets {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, b bucket) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := p.uploadBucket(jwt, b.files); err != nil {
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

func (p *Publisher) uploadBucket(jwt string, files []*fileEntry) error {
	type uploadItem struct {
		Key      string            `json:"key"`
		Value    string            `json:"value"`
		Metadata map[string]string `json:"metadata"`
		Base64   bool              `json:"base64"`
	}
	var payload []uploadItem
	for _, f := range files {
		content, err := os.ReadFile(f.absPath)
		if err != nil {
			return err
		}
		payload = append(payload, uploadItem{
			Key:      f.hash,
			Value:    base64.StdEncoding.EncodeToString(content),
			Metadata: map[string]string{"contentType": f.contentType},
			Base64:   true,
		})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling upload payload: %w", err)
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
