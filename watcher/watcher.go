package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sholdee/crd-schema-publisher/extractor"
	"github.com/sholdee/crd-schema-publisher/metrics"
	"github.com/sholdee/crd-schema-publisher/publisher"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

// Config holds the configuration for the CRD watcher.
type Config struct {
	Client     *apiextensionsclient.Clientset
	KubeConfig *rest.Config
	OutputDir  string
	BasePath   string
	Publisher  *publisher.Publisher // nil = extract-only
	Debounce   time.Duration
	Namespace  string
	LeaseName  string
	PodName    string
	HealthPort string
	Metrics    *metrics.Metrics    // nil = no metrics recording
	CRDLister  extractor.CRDLister // nil = derive from Client
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
	healthServer := startHealthServer(cfg.HealthPort, healthReady, cfg.Metrics)

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

	slog.Info("shutdown complete")
	return nil
}

func runLeader(ctx context.Context, cfg Config) {
	// Set up CRD informer
	trigger := make(chan struct{}, 1)
	lw := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (k8sruntime.Object, error) {
			return cfg.Client.ApiextensionsV1().CustomResourceDefinitions().List(ctx, opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			return cfg.Client.ApiextensionsV1().CustomResourceDefinitions().Watch(ctx, opts)
		},
	}

	notify := cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { signalTrigger(trigger) },
		UpdateFunc: func(old, new interface{}) { signalTrigger(trigger) },
		DeleteFunc: func(obj interface{}) { signalTrigger(trigger) },
	}

	_, controller := cache.NewInformerWithOptions(cache.InformerOptions{
		ListerWatcher: lw,
		ObjectType:    &apiextensionsv1.CustomResourceDefinition{},
		Handler:       notify,
	})

	go controller.Run(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), controller.HasSynced) {
		slog.Error("failed to sync informer cache")
		return
	}
	slog.Info("CRD informer synced, watching for changes")

	debounceLoop(trigger, cfg.Debounce, func() error {
		return publishCycle(cfg)
	}, cfg.Metrics, ctx.Done())
}

func signalTrigger(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
		// Channel already has a pending signal
	}
}

// drainTimeout bounds how long we wait for an in-flight publish to finish
// during shutdown. Must be less than terminationGracePeriodSeconds (default 30s)
// to leave time for health server shutdown and process cleanup.
const drainTimeout = 25 * time.Second

func debounceLoop(trigger <-chan struct{}, duration time.Duration, publish func() error, m *metrics.Metrics, done <-chan struct{}) {
	var timer *time.Timer
	var timerC <-chan time.Time
	var publishing atomic.Bool
	var wg sync.WaitGroup
	first := true
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	m.Heartbeat() // initial heartbeat on loop entry

	for {
		select {
		case <-done:
			if timer != nil {
				timer.Stop()
			}
			// Wait for in-flight publish to complete, bounded by drainTimeout
			drained := make(chan struct{})
			go func() {
				wg.Wait()
				close(drained)
			}()
			select {
			case <-drained:
			case <-time.After(drainTimeout):
				slog.Warn("drain timeout exceeded, abandoning in-flight publish")
			}
			return
		case <-trigger:
			m.Heartbeat()
			d := duration
			if first {
				d = 0
				first = false
			}
			if timer == nil {
				timer = time.NewTimer(d)
				timerC = timer.C
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(d)
			}
		case <-heartbeat.C:
			m.Heartbeat()
		case <-timerC:
			m.Heartbeat()
			timer = nil
			timerC = nil
			if !publishing.CompareAndSwap(false, true) {
				slog.Warn("publish already in progress, skipping")
				m.RecordSkip()
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer publishing.Store(false)
				slog.Info("running publish cycle")
				if err := publish(); err != nil {
					slog.Error("publish cycle failed", "error", err)
				}
			}()
		}
	}
}

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
		if err := cfg.Publisher.Publish(cfg.OutputDir); err != nil {
			return fmt.Errorf("publishing: %w", err)
		}
	}

	slog.Info("publish cycle complete", "duration", time.Since(start).Round(time.Millisecond))
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

func startHealthServer(port string, ready *atomic.Bool, m *metrics.Metrics) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if ready.Load() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
		}
	})
	mux.Handle("/metrics", m.Handler())
	server := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("health server error", "error", err)
		}
	}()
	return server
}
