package watcher

import (
	"context"
	"log/slog"
	"time"

	"github.com/sholdee/crd-schema-publisher/extractor"
	"github.com/sholdee/crd-schema-publisher/metrics"
)

// startBuiltinsRefresh polls the API server's OpenAPI v2 document on a timer
// and signals the publish trigger when it changes. CRD informer events are the
// only other trigger source, so without this a control-plane upgrade that
// changes built-in schemas would never republish on a quiet cluster. It is a
// no-op unless built-ins are enabled, an interval is set, and the OpenAPI
// source can detect changes.
func startBuiltinsRefresh(ctx context.Context, cfg Config, trigger chan struct{}) bool {
	if !cfg.IncludeBuiltins || cfg.BuiltinsRefreshInterval <= 0 {
		return false
	}
	detector, ok := cfg.OpenAPISource.(extractor.OpenAPIChangeDetector)
	if !ok {
		slog.Warn("built-in schema refresh disabled: OpenAPI source cannot detect changes")
		return false
	}
	slog.Info("built-in schema refresh enabled", "interval", cfg.BuiltinsRefreshInterval)
	go runBuiltinsRefresh(ctx, detector, cfg.BuiltinsRefreshInterval, trigger, cfg.Metrics)
	return true
}

func runBuiltinsRefresh(ctx context.Context, detector extractor.OpenAPIChangeDetector, interval time.Duration, trigger chan struct{}, m *metrics.Metrics) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkBuiltinsRefresh(ctx, detector, trigger, m)
		}
	}
}

// checkBuiltinsRefresh runs one change check. A detected change enqueues a
// publish through the same one-slot trigger CRD events use, so it coalesces
// with them and goes through the normal debounce. Check failures are logged
// and counted but never stop the loop: the next tick retries.
func checkBuiltinsRefresh(ctx context.Context, detector extractor.OpenAPIChangeDetector, trigger chan struct{}, m *metrics.Metrics) {
	changed, err := detector.OpenAPIV2Changed(ctx)
	m.RecordBuiltinsRefresh(changed, err)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("built-in schema refresh check failed", "error", err)
		}
		return
	}
	if changed {
		slog.Info("API server OpenAPI document changed, scheduling publish")
		signalTrigger(trigger)
	}
}
