package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sholdee/crd-schema-publisher/extractor"
	"github.com/sholdee/crd-schema-publisher/index"
	"github.com/sholdee/crd-schema-publisher/publisher"
	"github.com/sholdee/crd-schema-publisher/renderer"
	"github.com/sholdee/crd-schema-publisher/watcher"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
)

var (
	buildClientFunc       = extractor.BuildClient
	buildSiteFunc         = extractor.BuildSite
	validateOutputDirFunc = extractor.ValidateOutputDir
	renderPreviewFunc     = renderer.RenderAll
	generatePreviewFunc   = index.Generate
	scaffoldPreviewFunc   = scaffoldSampleData
	publishOutputFunc     = runUpload
)

func initLogger(cmd string) {
	var handler slog.Handler
	switch cmd {
	case "preview":
		handler = slog.NewTextHandler(os.Stderr, nil)
	default:
		handler = slog.NewJSONHandler(os.Stderr, nil)
	}
	slog.SetDefault(slog.New(handler))
}

func main() {
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	initLogger(cmd)

	switch cmd {
	case "run":
		if err := runAll(); err != nil {
			slog.Error("command failed", "command", cmd, "error", err)
			os.Exit(1)
		}
	case "extract":
		if err := runExtract(); err != nil {
			slog.Error("command failed", "command", cmd, "error", err)
			os.Exit(1)
		}
	case "upload":
		if err := runUpload(); err != nil {
			slog.Error("command failed", "command", cmd, "error", err)
			os.Exit(1)
		}
	case "watch":
		if err := runWatch(); err != nil {
			slog.Error("command failed", "command", cmd, "error", err)
			os.Exit(1)
		}
	case "preview":
		if err := runPreview(); err != nil {
			slog.Error("command failed", "command", cmd, "error", err)
			os.Exit(1)
		}
	default:
		slog.Error("unknown command", "command", cmd)
		fmt.Fprintf(os.Stderr, "usage: crd-schema-publisher [run|extract|upload|watch|preview]\n")
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

func normalizeBasePath(s string) string {
	s = strings.TrimRight(s, "/")
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	return s
}

func runExtract() error {
	_, err := runBuild()
	return err
}

func runBuild() (extractor.SiteBuildResult, error) {
	outputDir := getEnv("OUTPUT_DIR", "/output")
	basePath := normalizeBasePath(os.Getenv("BASE_PATH"))
	kubeContext := os.Getenv("KUBECTL_CONTEXT")

	slog.Info("building kubernetes client")
	client, err := buildClientFunc(kubeContext)
	if err != nil {
		return extractor.SiteBuildResult{}, fmt.Errorf("building client: %w", err)
	}

	result, err := buildSiteFunc(extractor.SiteBuildOptions{
		Lister:    client.ApiextensionsV1().CustomResourceDefinitions(),
		OutputDir: outputDir,
		BasePath:  basePath,
		Render:    os.Getenv("SKIP_RENDER") != "true",
	})
	if err != nil {
		return extractor.SiteBuildResult{}, err
	}
	if result.Status == extractor.BuildResultNoop {
		slog.Info("no CRDs found, leaving existing output untouched")
		return result, nil
	}

	slog.Info("extract complete", "count", result.SchemaCount, "dir", outputDir)
	return result, nil
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
	basePath := normalizeBasePath(os.Getenv("BASE_PATH"))
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
		BasePath:   basePath,
		Publisher:  pub,
		Debounce:   time.Duration(debounceSeconds) * time.Second,
		Namespace:  podNamespace,
		LeaseName:  leaseName,
		PodName:    podName,
		HealthPort: healthPort,
	})
}

func runPreview() error {
	basePath := normalizeBasePath(os.Getenv("BASE_PATH"))
	serveDir, cleanup, err := preparePreviewSite(getEnv("OUTPUT_DIR", ""), basePath, os.Getenv("SKIP_RENDER") != "true")
	if err != nil {
		return err
	}
	defer cleanup()

	addr := getEnv("PREVIEW_ADDR", "127.0.0.1:8989")
	var handler http.Handler
	if basePath != "" {
		mux := http.NewServeMux()
		mux.Handle(basePath+"/", http.StripPrefix(basePath, http.FileServer(http.Dir(serveDir))))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, basePath+"/", http.StatusFound)
		})
		handler = mux
	} else {
		handler = http.FileServer(http.Dir(serveDir))
	}
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	if basePath != "" {
		slog.Info("serving preview", "addr", addr, "url", "http://"+addr+basePath+"/")
	} else {
		slog.Info("serving preview", "addr", addr)
	}
	err = srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func preparePreviewSite(outputDir, basePath string, render bool) (string, func(), error) {
	if outputDir == "" {
		rootDir, cleanup, err := newPreviewRoot()
		if err != nil {
			return "", nil, err
		}
		if err := preparePreviewGeneration(rootDir, basePath, render, scaffoldPreviewFunc, "scaffolding sample data"); err != nil {
			cleanup()
			return "", nil, err
		}
		slog.Info("using sample data", "dir", rootDir, "active_dir", extractor.ActiveOutputDir(rootDir))
		return extractor.ActiveOutputDir(rootDir), cleanup, nil
	}

	if err := validateOutputDirFunc(outputDir); err != nil {
		return "", nil, err
	}

	activeDir, resolvedActiveDir, err := resolvePreviewActiveDir(outputDir)
	if err != nil {
		return "", nil, err
	}

	rootDir, cleanup, err := newPreviewRoot()
	if err != nil {
		return "", nil, err
	}
	if err := preparePreviewGeneration(rootDir, basePath, render, func(generationDir string) error {
		return copyPreviewFiles(resolvedActiveDir, generationDir)
	}, "copying active output"); err != nil {
		cleanup()
		return "", nil, err
	}
	slog.Info("using existing output", "dir", outputDir, "active_dir", activeDir, "preview_dir", extractor.ActiveOutputDir(rootDir))
	return extractor.ActiveOutputDir(rootDir), cleanup, nil
}

func newPreviewRoot() (string, func(), error) {
	rootDir, err := os.MkdirTemp("", "crd-preview-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp dir: %w", err)
	}
	return rootDir, func() {
		_ = os.RemoveAll(rootDir)
	}, nil
}

func resolvePreviewActiveDir(outputDir string) (string, string, error) {
	activeDir := extractor.ActiveOutputDir(outputDir)
	resolvedActiveDir, err := filepath.EvalSymlinks(activeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", fmt.Errorf("active output %q does not exist; run extract first", activeDir)
		}
		return "", "", fmt.Errorf("resolving active output: %w", err)
	}
	return activeDir, resolvedActiveDir, nil
}

func preparePreviewGeneration(rootDir, basePath string, render bool, seed func(string) error, seedAction string) error {
	generationDir, err := makePreviewGenerationDir(rootDir)
	if err != nil {
		return fmt.Errorf("creating preview generation: %w", err)
	}
	if err := seed(generationDir); err != nil {
		return fmt.Errorf("%s: %w", seedAction, err)
	}
	if render {
		slog.Info("rendering schema pages")
		if err := renderPreviewFunc(generationDir, basePath); err != nil {
			return fmt.Errorf("rendering schemas: %w", err)
		}
	}
	slog.Info("generating index")
	if err := generatePreviewFunc(generationDir, basePath); err != nil {
		return fmt.Errorf("generating index: %w", err)
	}
	if err := activatePreviewGeneration(rootDir, generationDir); err != nil {
		return fmt.Errorf("activating preview generation: %w", err)
	}
	return nil
}

func makePreviewGenerationDir(outputDir string) (string, error) {
	generationsDir := filepath.Join(outputDir, ".generations")
	if err := os.MkdirAll(generationsDir, 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(generationsDir, "preview-")
}

func activatePreviewGeneration(outputDir, generationDir string) error {
	currentPath := extractor.ActiveOutputDir(outputDir)
	tmpPath := filepath.Join(outputDir, ".current.tmp")
	target := filepath.Join(".generations", filepath.Base(generationDir))

	_ = os.Remove(tmpPath)
	if err := os.Symlink(target, tmpPath); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, currentPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func copyPreviewFiles(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		dstPath := filepath.Join(dstDir, rel)
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		return copyPreviewFile(path, dstPath, info.Mode())
	})
}

func copyPreviewFile(srcPath, dstPath string, mode os.FileMode) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = src.Close()
	}()

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer func() {
		_ = dst.Close()
	}()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return nil
}

func scaffoldSampleData(dir string) error {
	type sampleSchema struct {
		file string
		kind string
	}

	sampleGroups := map[string][]sampleSchema{
		"cert-manager.io": {
			{file: "certificate_v1.json", kind: "Certificate"},
			{file: "clusterissuer_v1.json", kind: "ClusterIssuer"},
			{file: "issuer_v1.json", kind: "Issuer"},
		},
		"monitoring.coreos.com": {
			{file: "alertmanager_v1.json", kind: "Alertmanager"},
			{file: "podmonitor_v1.json", kind: "PodMonitor"},
			{file: "prometheus_v1.json", kind: "Prometheus"},
			{file: "servicemonitor_v1.json", kind: "ServiceMonitor"},
		},
		"helm.toolkit.fluxcd.io": {
			{file: "helmrelease_v2.json", kind: "HelmRelease"},
			{file: "helmrelease_v2beta1.json", kind: "HelmRelease"},
		},
		"source.toolkit.fluxcd.io": {
			{file: "gitrepository_v1.json", kind: "GitRepository"},
			{file: "helmchart_v1.json", kind: "HelmChart"},
			{file: "helmrepository_v1.json", kind: "HelmRepository"},
			{file: "ocirepository_v1beta2.json", kind: "OCIRepository"},
		},
		"kustomize.toolkit.fluxcd.io": {
			{file: "kustomization_v1.json", kind: "Kustomization"},
		},
		"cilium.io": {
			{file: "ciliumnetworkpolicy_v2.json", kind: "CiliumNetworkPolicy"},
			{file: "ciliumclusterwidenetworkpolicy_v2.json", kind: "CiliumClusterwideNetworkPolicy"},
			{file: "ciliumendpoint_v2.json", kind: "CiliumEndpoint"},
		},
		"traefik.io": {
			{file: "ingressroute_v1alpha1.json", kind: "IngressRoute"},
			{file: "middleware_v1alpha1.json", kind: "Middleware"},
			{file: "tlsoption_v1alpha1.json", kind: "TLSOption"},
		},
		"external-secrets.io": {
			{file: "externalsecret_v1beta1.json", kind: "ExternalSecret"},
			{file: "clustersecretstore_v1beta1.json", kind: "ClusterSecretStore"},
			{file: "secretstore_v1beta1.json", kind: "SecretStore"},
		},
		"metallb.io": {
			{file: "ipaddresspool_v1beta1.json", kind: "IPAddressPool"},
			{file: "l2advertisement_v1beta1.json", kind: "L2Advertisement"},
		},
		"volsync.backube": {
			{file: "replicationsource_v1alpha1.json", kind: "ReplicationSource"},
			{file: "replicationdestination_v1alpha1.json", kind: "ReplicationDestination"},
		},
	}
	kinds := make(map[string]string)
	for group, files := range sampleGroups {
		groupDir := filepath.Join(dir, group)
		if err := os.MkdirAll(groupDir, 0o755); err != nil {
			return fmt.Errorf("creating group dir %s: %w", group, err)
		}
		for _, schema := range files {
			path := filepath.Join(groupDir, schema.file)
			if err := os.WriteFile(path, []byte(`{"type":"object"}`), 0o644); err != nil {
				return fmt.Errorf("writing %s/%s: %w", group, schema.file, err)
			}
			kinds[filepath.ToSlash(filepath.Join(group, schema.file))] = schema.kind
		}
	}
	manifestDir := filepath.Join(dir, "_meta")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		return fmt.Errorf("creating metadata dir: %w", err)
	}
	manifestBytes, err := json.MarshalIndent(kinds, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding kind manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "kinds.json"), append(manifestBytes, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing kind manifest: %w", err)
	}
	return nil
}

func runAll() error {
	result, err := runBuild()
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	if result.Status == extractor.BuildResultNoop {
		return nil
	}
	if publishOutputFunc == nil {
		return nil
	}
	if err := publishOutputFunc(); err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	return nil
}
