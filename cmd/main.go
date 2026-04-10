package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/sholdee/crd-schema-publisher/extractor"
	"github.com/sholdee/crd-schema-publisher/index"
	"github.com/sholdee/crd-schema-publisher/publisher"
	"github.com/sholdee/crd-schema-publisher/watcher"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
)

func main() {
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "run":
		if err := runAll(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "extract":
		if err := runExtract(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "upload":
		if err := runUpload(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "watch":
		if err := runWatch(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "preview":
		if err := runPreview(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\nusage: crd-schema-publisher [run|extract|upload|watch|preview]\n", cmd)
		os.Exit(1)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func requireEnv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("required environment variable %s is not set", key)
	}
	return v, nil
}

func runExtract() error {
	outputDir := getEnv("OUTPUT_DIR", "/output")
	kubeContext := os.Getenv("KUBECTL_CONTEXT")

	fmt.Println("Building Kubernetes client...")
	client, err := extractor.BuildClient(kubeContext)
	if err != nil {
		return fmt.Errorf("building client: %w", err)
	}

	fmt.Println("Listing CRDs...")
	crds, err := extractor.ListCRDs(client)
	if err != nil {
		return err
	}
	fmt.Printf("Found %d CRDs\n", len(crds))

	if len(crds) == 0 {
		fmt.Println("No CRDs found, nothing to extract")
		return nil
	}

	count, err := extractor.WriteSchemas(crds, outputDir)
	if err != nil {
		return err
	}
	fmt.Printf("Wrote %d JSON schemas to %s\n", count, outputDir)

	fmt.Println("Generating index.html...")
	if err := index.Generate(outputDir); err != nil {
		return fmt.Errorf("generating index: %w", err)
	}

	fmt.Println("Extract complete")
	return nil
}

func runUpload() error {
	outputDir := getEnv("OUTPUT_DIR", "/output")

	apiToken, err := requireEnv("CLOUDFLARE_API_TOKEN")
	if err != nil {
		return err
	}
	accountID, err := requireEnv("CLOUDFLARE_ACCOUNT_ID")
	if err != nil {
		return err
	}
	projectName := getEnv("CF_PAGES_PROJECT", "kubernetes-schemas")

	p := &publisher.Publisher{
		APIToken:    apiToken,
		AccountID:   accountID,
		ProjectName: projectName,
	}

	return p.Publish(outputDir)
}

func runWatch() error {
	outputDir := getEnv("OUTPUT_DIR", "/output")
	kubeContext := os.Getenv("KUBECTL_CONTEXT")

	podName, err := requireEnv("POD_NAME")
	if err != nil {
		return err
	}
	podNamespace, err := requireEnv("POD_NAMESPACE")
	if err != nil {
		return err
	}

	debounceSeconds := 15
	if v := os.Getenv("DEBOUNCE_SECONDS"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid DEBOUNCE_SECONDS: %w", err)
		}
		debounceSeconds = parsed
	}

	leaseName := getEnv("LEASE_NAME", "crd-schema-publisher")
	healthPort := getEnv("HEALTH_PORT", "8080")

	cfg, err := extractor.BuildConfig(kubeContext)
	if err != nil {
		return fmt.Errorf("building kubeconfig: %w", err)
	}

	client, err := apiextensionsclient.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("building apiextensions client: %w", err)
	}

	var pub *publisher.Publisher
	apiToken := os.Getenv("CLOUDFLARE_API_TOKEN")
	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	if apiToken != "" && accountID != "" {
		pub = &publisher.Publisher{
			APIToken:    apiToken,
			AccountID:   accountID,
			ProjectName: getEnv("CF_PAGES_PROJECT", "kubernetes-schemas"),
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	return watcher.Run(ctx, watcher.Config{
		Client:     client,
		KubeConfig: cfg,
		OutputDir:  outputDir,
		Publisher:  pub,
		Debounce:   time.Duration(debounceSeconds) * time.Second,
		Namespace:  podNamespace,
		LeaseName:  leaseName,
		PodName:    podName,
		HealthPort: healthPort,
	})
}

func runPreview() error {
	dir := getEnv("OUTPUT_DIR", "")
	isTempDir := false
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "crd-preview-*")
		if err != nil {
			return fmt.Errorf("creating temp dir: %w", err)
		}
		isTempDir = true
		if err := scaffoldSampleData(dir); err != nil {
			os.RemoveAll(dir)
			return fmt.Errorf("scaffolding sample data: %w", err)
		}
		fmt.Printf("Using sample data in %s\n", dir)
	} else {
		fmt.Printf("Using existing output at %s\n", dir)
	}

	fmt.Println("Generating index.html...")
	if err := index.Generate(dir); err != nil {
		if isTempDir {
			os.RemoveAll(dir)
		}
		return fmt.Errorf("generating index: %w", err)
	}

	addr := getEnv("PREVIEW_ADDR", "127.0.0.1:8989")
	srv := &http.Server{Addr: addr, Handler: http.FileServer(http.Dir(dir))}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("Serving at http://%s (Ctrl+C to stop)\n", addr)
	err := srv.ListenAndServe()
	if isTempDir {
		os.RemoveAll(dir)
	}
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func scaffoldSampleData(dir string) error {
	sampleGroups := map[string][]string{
		"cert-manager.io":             {"certificate_v1.json", "clusterissuer_v1.json", "issuer_v1.json"},
		"monitoring.coreos.com":       {"alertmanager_v1.json", "podmonitor_v1.json", "prometheus_v1.json", "servicemonitor_v1.json"},
		"helm.toolkit.fluxcd.io":      {"helmrelease_v2.json", "helmrelease_v2beta1.json"},
		"source.toolkit.fluxcd.io":    {"gitrepository_v1.json", "helmchart_v1.json", "helmrepository_v1.json", "ocirepository_v1beta2.json"},
		"kustomize.toolkit.fluxcd.io": {"kustomization_v1.json"},
		"cilium.io":                   {"ciliumnetworkpolicy_v2.json", "ciliumclusterwidenetworkpolicy_v2.json", "ciliumendpoint_v2.json"},
		"traefik.io":                  {"ingressroute_v1alpha1.json", "middleware_v1alpha1.json", "tlsoption_v1alpha1.json"},
		"external-secrets.io":         {"externalsecret_v1beta1.json", "clustersecretstore_v1beta1.json", "secretstore_v1beta1.json"},
		"metallb.io":                  {"ipaddresspool_v1beta1.json", "l2advertisement_v1beta1.json"},
		"volsync.backube":             {"replicationsource_v1alpha1.json", "replicationdestination_v1alpha1.json"},
	}
	for group, files := range sampleGroups {
		groupDir := filepath.Join(dir, group)
		if err := os.MkdirAll(groupDir, 0o755); err != nil {
			return fmt.Errorf("creating group dir %s: %w", group, err)
		}
		for _, f := range files {
			if err := os.WriteFile(filepath.Join(groupDir, f), []byte(`{"type":"object"}`), 0o644); err != nil {
				return fmt.Errorf("writing %s/%s: %w", group, f, err)
			}
		}
	}
	return nil
}

func runAll() error {
	if err := runExtract(); err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	if err := runUpload(); err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	return nil
}
