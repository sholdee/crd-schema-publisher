package watcher

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/sholdee/crd-schema-publisher/extractor"
)

func publishCycle(cfg Config) (retErr error) {
	start := time.Now()
	defer func() {
		cfg.Metrics.RecordPublishCycle(time.Since(start), retErr)
	}()
	// Reset discovery gauges so they reflect each cycle, not stale previous values
	cfg.Metrics.RecordDiscovery(0, 0)

	var lister extractor.CRDLister
	if cfg.CRDLister != nil {
		lister = cfg.CRDLister
	} else {
		lister = cfg.Client.ApiextensionsV1().CustomResourceDefinitions()
	}

	result, err := extractor.BuildSite(extractor.SiteBuildOptions{
		Lister:    lister,
		OutputDir: cfg.OutputDir,
		BasePath:  cfg.BasePath,
		Render:    os.Getenv("SKIP_RENDER") != "true",
		Filter:    cfg.Filter,
		Profiler:  cfg.Profiler,
	})
	if err != nil {
		return err
	}
	if result.Status == extractor.BuildResultNoop {
		slog.Info("no CRDs found, leaving existing output untouched")
		return nil
	}

	cfg.Metrics.RecordDiscovery(result.CRDCount, result.SchemaCount)
	slog.Info("wrote schemas", "count", result.SchemaCount)
	slog.Info("generated index")

	// Upload (if publisher configured)
	if cfg.Publisher != nil {
		if cfg.Publisher.Profiler == nil {
			cfg.Publisher.Profiler = cfg.Profiler
		}
		if err := cfg.Publisher.Publish(cfg.OutputDir); err != nil {
			return fmt.Errorf("publishing: %w", err)
		}
	}

	slog.Info("publish cycle complete", "duration", time.Since(start).Round(time.Millisecond))
	return nil
}
