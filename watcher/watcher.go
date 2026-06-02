package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/sholdee/crd-schema-publisher/diagnostics"
	"github.com/sholdee/crd-schema-publisher/extractor"
	"github.com/sholdee/crd-schema-publisher/metrics"
	"github.com/sholdee/crd-schema-publisher/publisher"
	"github.com/sholdee/crd-schema-publisher/site"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

// Config holds the configuration for the CRD watcher.
type Config struct {
	Context          context.Context
	Client           *apiextensionsclient.Clientset
	KubeConfig       *rest.Config
	OutputDir        string
	BasePath         string
	Publisher        *publisher.Publisher // nil = extract-only
	Debounce         time.Duration
	Namespace        string
	LeaseName        string
	PodName          string
	HealthPort       string
	SitePort         string // empty = disabled
	SiteAccessLog    bool
	Metrics          *metrics.Metrics    // nil = no metrics recording
	CRDLister        extractor.CRDLister // nil = derive from Client
	Filter           extractor.SchemaFilter
	IncludeBuiltins  bool
	IncludeKustomize bool
	OpenAPISource    extractor.OpenAPISource
	Profiler         diagnostics.Snapshotter
}

// Run starts the watcher with leader election and health server.
func Run(ctx context.Context, cfg Config) error {
	slog.Info("watcher starting",
		"namespace", cfg.Namespace,
		"pod", cfg.PodName,
		"lease", cfg.LeaseName,
		"debounce", cfg.Debounce,
		"health_port", cfg.HealthPort,
		"publisher_configured", cfg.Publisher != nil,
	)
	// Start health server before leader election
	healthReady := &atomic.Bool{}
	cfg.Metrics = metrics.New()
	siteReady := func() bool { return true }
	if cfg.SitePort != "" {
		siteReady = newSiteReadyChecker(cfg.OutputDir)
	}
	healthServer := startHealthServer(cfg.HealthPort, healthReady, cfg.Metrics, siteReady)
	var siteServer *http.Server
	if cfg.SitePort != "" {
		siteDir := extractor.ActiveOutputDir(cfg.OutputDir)
		handler := site.NewStaticHandler(siteDir, cfg.BasePath)
		if cfg.SiteAccessLog {
			handler = site.WithAccessLog(handler)
		}
		var err error
		siteServer, err = site.StartServer(":"+cfg.SitePort, handler)
		if err != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if shutdownErr := healthServer.Shutdown(shutdownCtx); shutdownErr != nil {
				slog.Error("health server shutdown error", "error", shutdownErr)
			}
			return fmt.Errorf("starting site server: %w", err)
		}
		slog.Info("site server started", "port", cfg.SitePort, "dir", siteDir, "base_path", cfg.BasePath, "access_log", cfg.SiteAccessLog)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg.KubeConfig)
	if err != nil {
		return fmt.Errorf("building kubernetes client: %w", err)
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      cfg.LeaseName,
			Namespace: cfg.Namespace,
		},
		Client: kubeClient.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: cfg.PodName,
		},
	}

	// Mark ready once we're participating in leader election
	healthReady.Store(true)

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: 15 * time.Second,
		RenewDeadline: 10 * time.Second,
		RetryPeriod:   2 * time.Second,
		// ReleaseOnCancel releases the lease on context cancellation so another
		// replica can acquire leadership quickly. This means the lease is released
		// while an in-flight publish may still be running. This is safe because
		// publish cycles are idempotent and the new leader does a full re-publish.
		ReleaseOnCancel: true,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				slog.Info("acquired leadership, starting watch loop")
				cfg.Metrics.SetLeader(true)
				runLeader(ctx, cfg)
			},
			OnStoppedLeading: func() {
				cfg.Metrics.SetLeader(false)
				// Distinguish graceful shutdown (context cancelled) from unexpected lease loss.
				// On unexpected loss, exit immediately — standard controller pattern.
				// On graceful shutdown, return normally so Run() can drain.
				if ctx.Err() != nil {
					slog.Info("leadership released during shutdown")
				} else {
					slog.Error("lost leadership unexpectedly, exiting")
					os.Exit(1)
				}
			},
			OnNewLeader: func(identity string) {
				if identity != cfg.PodName {
					slog.Info("new leader elected", "identity", identity)
				}
			},
		},
	})

	// Graceful shutdown: stop health server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("health server shutdown error", "error", err)
	}
	if siteServer != nil {
		siteShutdownCtx, siteShutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer siteShutdownCancel()
		if err := siteServer.Shutdown(siteShutdownCtx); err != nil {
			slog.Error("site server shutdown error", "error", err)
		}
	}

	slog.Info("shutdown complete")
	return nil
}

func cleanDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(dir, 0o755)
		}
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
