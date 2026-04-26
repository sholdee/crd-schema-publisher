package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sholdee/crd-schema-publisher/extractor"
	"github.com/sholdee/crd-schema-publisher/publisher"
	"github.com/sholdee/crd-schema-publisher/watcher"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
)

const (
	schemaFilterKindEnv    = "SCHEMA_FILTER_KIND"
	schemaFilterGroupEnv   = "SCHEMA_FILTER_GROUP"
	schemaFilterVersionEnv = "SCHEMA_FILTER_VERSION"
)

type runtimeCommandOptions struct {
	OutputDir string
	Filter    extractor.SchemaFilter
}

func init() {
	registerCommand("run", runAll)
	registerCommand("extract", runExtract)
	registerCommand("upload", runUpload)
	registerCommand("watch", runWatch)
	if publishOutputFunc == nil {
		publishOutputFunc = func(outputDir string) error {
			return runUpload([]string{"--output-dir", outputDir})
		}
	}
}

func runExtract(args []string) error {
	fs := newCommandFlagSet("extract")
	var outputDir string
	stringFlagWithAlias(fs, &outputDir, "output-dir", "o", os.Getenv("OUTPUT_DIR"), "output directory")
	basePath := fs.String("base-path", os.Getenv("BASE_PATH"), "URL path prefix for subpath deployments")
	kubeContext := fs.String("context", os.Getenv("KUBECTL_CONTEXT"), "Kubernetes context")
	skipRender := fs.Bool("skip-render", os.Getenv("SKIP_RENDER") == "true", "skip HTML rendering")
	kind := fs.String("kind", os.Getenv(schemaFilterKindEnv), "filter by kind (comma-separated, case-insensitive)")
	group := fs.String("group", os.Getenv(schemaFilterGroupEnv), "filter by group (comma-separated, case-insensitive)")
	version := fs.String("version", os.Getenv(schemaFilterVersionEnv), "filter by version (comma-separated, case-insensitive)")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if extras := fs.Args(); len(extras) > 0 {
		return fmt.Errorf("unexpected arguments for extract: %s", strings.Join(extras, " "))
	}
	if err := requireConfiguredOutputDir(outputDir, "Provide a writable directory for extracted schemas"); err != nil {
		return err
	}

	slog.Info("building kubernetes client")
	client, err := buildClientFunc(*kubeContext)
	if err != nil {
		return fmt.Errorf("building client: %w", err)
	}

	filter := extractor.ParseFilter(*kind, *group, *version)

	result, err := buildSiteFunc(extractor.SiteBuildOptions{
		Lister:    client.ApiextensionsV1().CustomResourceDefinitions(),
		OutputDir: outputDir,
		BasePath:  normalizeBasePath(*basePath),
		Render:    !*skipRender,
		Filter:    filter,
	})
	if err != nil {
		return err
	}
	if result.Status == extractor.BuildResultNoop {
		slog.Info("no CRDs found, leaving existing output untouched")
		return nil
	}

	slog.Info("extract complete", "count", result.SchemaCount, "dir", outputDir)
	return nil
}

func parseRuntimeCommandArgs(cmd string, args []string, fallbackOutputDir string) (runtimeCommandOptions, error) {
	fs := newCommandFlagSet(cmd)
	var outputDir string
	stringFlagWithAlias(fs, &outputDir, "output-dir", "o", fallbackOutputDir, "output directory")
	kind := fs.String("kind", os.Getenv(schemaFilterKindEnv), "filter by kind (comma-separated, case-insensitive)")
	group := fs.String("group", os.Getenv(schemaFilterGroupEnv), "filter by group (comma-separated, case-insensitive)")
	version := fs.String("version", os.Getenv(schemaFilterVersionEnv), "filter by version (comma-separated, case-insensitive)")
	if err := fs.Parse(args); err != nil {
		return runtimeCommandOptions{}, err
	}
	if extras := fs.Args(); len(extras) > 0 {
		return runtimeCommandOptions{}, fmt.Errorf("unexpected arguments for %s: %s", cmd, strings.Join(extras, " "))
	}

	return runtimeCommandOptions{
		OutputDir: outputDir,
		Filter:    extractor.ParseFilter(*kind, *group, *version),
	}, nil
}

func runBuild(outputDir string, filter extractor.SchemaFilter) (extractor.SiteBuildResult, error) {
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
		Filter:    filter,
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

func runUpload(args []string) error {
	outputDir, _, err := parseOutputDirArg("upload", args, getEnv("OUTPUT_DIR", "/output"))
	if err != nil {
		return err
	}
	if err := requireExistingOutputDir(outputDir, "Set OUTPUT_DIR or pass --output-dir to a pre-created directory"); err != nil {
		return err
	}

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

func runWatch(args []string) error {
	opts, err := parseRuntimeCommandArgs("watch", args, getEnv("OUTPUT_DIR", "/output"))
	if err != nil {
		return err
	}
	if err := requireExistingOutputDir(opts.OutputDir, "Set OUTPUT_DIR or pass --output-dir to a pre-created directory"); err != nil {
		return err
	}
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
		OutputDir:  opts.OutputDir,
		BasePath:   basePath,
		Publisher:  pub,
		Debounce:   time.Duration(debounceSeconds) * time.Second,
		Namespace:  podNamespace,
		LeaseName:  leaseName,
		PodName:    podName,
		HealthPort: healthPort,
		Filter:     opts.Filter,
	})
}

func runAll(args []string) error {
	opts, err := parseRuntimeCommandArgs("run", args, getEnv("OUTPUT_DIR", "/output"))
	if err != nil {
		return err
	}
	if err := requireExistingOutputDir(opts.OutputDir, "Set OUTPUT_DIR or pass --output-dir to a pre-created directory"); err != nil {
		return err
	}

	result, err := runBuild(opts.OutputDir, opts.Filter)
	if err != nil {
		return fmt.Errorf("%w\n\n"+
			"The \"run\" command extracts schemas from a Kubernetes cluster and uploads when Cloudflare credentials are configured.\n\n"+
			"To extract schemas from a cluster:\n"+
			"  crd-schema-publisher extract --output-dir ./schemas\n\n"+
			"To convert CRD YAML files directly:\n"+
			"  crd-schema-publisher convert --file crd.yaml --output-dir ./schemas\n\n"+
			"Use \"crd-schema-publisher --help\" for all commands", err)
	}
	if result.Status == extractor.BuildResultNoop {
		return nil
	}

	apiToken := os.Getenv("CLOUDFLARE_API_TOKEN")
	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	if apiToken == "" || accountID == "" {
		slog.Info("skipping upload: CLOUDFLARE_API_TOKEN or CLOUDFLARE_ACCOUNT_ID not set")
		return nil
	}

	if err := publishOutputFunc(opts.OutputDir); err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	return nil
}
