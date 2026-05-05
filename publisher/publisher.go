package publisher

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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

type cfResponse struct {
	Success bool              `json:"success"`
	Result  json.RawMessage   `json:"result"`
	Errors  []json.RawMessage `json:"errors"`
}

func (p *Publisher) sleepFunc() func(time.Duration) {
	if p.SleepFunc != nil {
		return p.SleepFunc
	}
	return time.Sleep
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
