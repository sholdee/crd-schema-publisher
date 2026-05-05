package publisher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"sync"
)

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
			p.sleepFunc()(retryDelay(attempt))
			continue
		}
		var cr cfResponse
		if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("decoding response: %w", err)
			slog.Warn("upload attempt failed, retrying", "attempt", attempt+1, "max", maxUploadRetries, "error", lastErr)
			p.sleepFunc()(retryDelay(attempt))
			continue
		}
		_ = resp.Body.Close()
		if cr.Success {
			return nil
		}
		lastErr = fmt.Errorf("upload failed: %s", cr.Errors)
		if resp.StatusCode >= 500 {
			slog.Warn("upload attempt failed, retrying", "attempt", attempt+1, "max", maxUploadRetries, "error", lastErr)
			p.sleepFunc()(retryDelay(attempt))
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
			p.sleepFunc()(retryDelay(attempt))
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var cr cfResponse
		if err := json.Unmarshal(respBody, &cr); err != nil {
			lastErr = fmt.Errorf("decoding response: %w", err)
			slog.Warn("deployment attempt failed, retrying", "attempt", attempt+1, "max", maxDeployRetries, "error", lastErr)
			p.sleepFunc()(retryDelay(attempt))
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
			p.sleepFunc()(retryDelay(attempt))
			continue
		}
		return "", lastErr
	}
	return "", fmt.Errorf("deployment failed after %d retries: %w", maxDeployRetries, lastErr)
}
