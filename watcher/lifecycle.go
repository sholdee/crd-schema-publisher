package watcher

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/sholdee/crd-schema-publisher/extractor"
	"github.com/sholdee/crd-schema-publisher/metrics"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

func activeSiteReady(outputDir string) bool {
	_, err := os.Stat(filepath.Join(extractor.ActiveOutputDir(outputDir), "index.html"))
	return err == nil
}

func newSiteReadyChecker(outputDir string) func() bool {
	var logged atomic.Bool
	return func() bool {
		ready := activeSiteReady(outputDir)
		if ready && logged.CompareAndSwap(false, true) {
			slog.Info("site ready", "dir", extractor.ActiveOutputDir(outputDir))
		}
		return ready
	}
}

func runLeader(ctx context.Context, cfg Config) {
	trigger := make(chan struct{}, 1)
	controller := newCRDController(ctx, cfg, trigger)
	runLeaderWithController(ctx, cfg, trigger, controller)
}

func newCRDController(ctx context.Context, cfg Config, trigger chan struct{}) cache.Controller {
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
		UpdateFunc: func(_, _ interface{}) { signalTrigger(trigger) },
		DeleteFunc: func(obj interface{}) { signalTrigger(trigger) },
	}

	_, controller := cache.NewInformerWithOptions(cache.InformerOptions{
		ListerWatcher: lw,
		ObjectType:    &apiextensionsv1.CustomResourceDefinition{},
		Handler:       notify,
	})
	return controller
}

func runLeaderWithController(ctx context.Context, cfg Config, trigger chan struct{}, controller cache.Controller) {
	go controller.Run(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), controller.HasSynced) {
		slog.Error("failed to sync informer cache")
		return
	}
	slog.Info("CRD informer synced, watching for changes")
	// Publish once after sync so zero-CRD clusters can still emit optional schemas.
	signalTrigger(trigger)

	debounceLoop(trigger, cfg.Debounce, func() error {
		cycleCfg := cfg
		cycleCfg.Context = ctx
		return publishCycle(cycleCfg)
	}, cfg.Metrics, ctx.Done())
}

func startHealthServer(port string, ready *atomic.Bool, m *metrics.Metrics, extraReady func() bool) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if ready.Load() && extraReady() {
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
