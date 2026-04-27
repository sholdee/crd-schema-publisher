package publisher

import (
	"fmt"
	"os"
	"strconv"
)

const (
	uploadBucketSizeBytesEnv = "UPLOAD_BUCKET_SIZE_BYTES"
	uploadConcurrencyEnv     = "UPLOAD_CONCURRENCY"
)

type UploadConfig struct {
	BucketSizeBytes int64
	Concurrency     int
}

func DefaultUploadConfig() UploadConfig {
	return UploadConfig{
		BucketSizeBytes: maxBucketSize,
		Concurrency:     uploadConcurrency,
	}
}

func UploadConfigFromEnv() (UploadConfig, error) {
	cfg := DefaultUploadConfig()

	if raw := os.Getenv(uploadBucketSizeBytesEnv); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			return UploadConfig{}, fmt.Errorf("invalid %s %q: must be a positive integer byte count", uploadBucketSizeBytesEnv, raw)
		}
		cfg.BucketSizeBytes = value
	}

	if raw := os.Getenv(uploadConcurrencyEnv); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return UploadConfig{}, fmt.Errorf("invalid %s %q: must be a positive integer", uploadConcurrencyEnv, raw)
		}
		cfg.Concurrency = value
	}

	return cfg, nil
}
