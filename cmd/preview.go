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
	"syscall"
	"time"

	"github.com/sholdee/crd-schema-publisher/extractor"
	"github.com/sholdee/crd-schema-publisher/schemametadata"
	"github.com/sholdee/crd-schema-publisher/site"
)

func init() {
	registerCommand("preview", runPreview)
	if scaffoldPreviewFunc == nil {
		scaffoldPreviewFunc = scaffoldSampleData
	}
	if preparePreviewSiteFunc == nil {
		preparePreviewSiteFunc = preparePreviewSite
	}
}

func runPreview(args []string) error {
	cfg, err := parsePreviewConfig(args)
	if err != nil {
		return err
	}
	if cfg.ExplicitOutputDir {
		if err := requireExistingOutputDir(cfg.OutputDir, "Pass --output-dir to a pre-created directory"); err != nil {
			return err
		}
	}
	basePath := normalizeBasePath(os.Getenv("BASE_PATH"))
	serveDir, cleanup, err := preparePreviewSiteFunc(cfg.OutputDir, basePath, os.Getenv("SKIP_RENDER") != "true")
	if err != nil {
		return err
	}
	defer cleanup()

	addr := getEnv("PREVIEW_ADDR", "127.0.0.1:8989")
	handler := site.NewStaticHandler(serveDir, basePath)
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
		file   string
		kind   string
		source schemametadata.SchemaSource
	}

	sampleGroups := map[string][]sampleSchema{
		"cert-manager.io": {
			{file: "certificate_v1.json", kind: "Certificate", source: schemametadata.SchemaSourceCRD},
			{file: "clusterissuer_v1.json", kind: "ClusterIssuer", source: schemametadata.SchemaSourceCRD},
			{file: "issuer_v1.json", kind: "Issuer", source: schemametadata.SchemaSourceCRD},
		},
		"monitoring.coreos.com": {
			{file: "alertmanager_v1.json", kind: "Alertmanager", source: schemametadata.SchemaSourceCRD},
			{file: "podmonitor_v1.json", kind: "PodMonitor", source: schemametadata.SchemaSourceCRD},
			{file: "prometheus_v1.json", kind: "Prometheus", source: schemametadata.SchemaSourceCRD},
			{file: "servicemonitor_v1.json", kind: "ServiceMonitor", source: schemametadata.SchemaSourceCRD},
		},
		"helm.toolkit.fluxcd.io": {
			{file: "helmrelease_v2.json", kind: "HelmRelease", source: schemametadata.SchemaSourceCRD},
			{file: "helmrelease_v2beta1.json", kind: "HelmRelease", source: schemametadata.SchemaSourceCRD},
		},
		"source.toolkit.fluxcd.io": {
			{file: "gitrepository_v1.json", kind: "GitRepository", source: schemametadata.SchemaSourceCRD},
			{file: "helmchart_v1.json", kind: "HelmChart", source: schemametadata.SchemaSourceCRD},
			{file: "helmrepository_v1.json", kind: "HelmRepository", source: schemametadata.SchemaSourceCRD},
			{file: "ocirepository_v1beta2.json", kind: "OCIRepository", source: schemametadata.SchemaSourceCRD},
		},
		"kustomize.toolkit.fluxcd.io": {
			{file: "kustomization_v1.json", kind: "Kustomization", source: schemametadata.SchemaSourceCRD},
		},
		"cilium.io": {
			{file: "ciliumnetworkpolicy_v2.json", kind: "CiliumNetworkPolicy", source: schemametadata.SchemaSourceCRD},
			{file: "ciliumclusterwidenetworkpolicy_v2.json", kind: "CiliumClusterwideNetworkPolicy", source: schemametadata.SchemaSourceCRD},
			{file: "ciliumendpoint_v2.json", kind: "CiliumEndpoint", source: schemametadata.SchemaSourceCRD},
		},
		"traefik.io": {
			{file: "ingressroute_v1alpha1.json", kind: "IngressRoute", source: schemametadata.SchemaSourceCRD},
			{file: "middleware_v1alpha1.json", kind: "Middleware", source: schemametadata.SchemaSourceCRD},
			{file: "tlsoption_v1alpha1.json", kind: "TLSOption", source: schemametadata.SchemaSourceCRD},
		},
		"external-secrets.io": {
			{file: "externalsecret_v1beta1.json", kind: "ExternalSecret", source: schemametadata.SchemaSourceCRD},
			{file: "clustersecretstore_v1beta1.json", kind: "ClusterSecretStore", source: schemametadata.SchemaSourceCRD},
			{file: "secretstore_v1beta1.json", kind: "SecretStore", source: schemametadata.SchemaSourceCRD},
		},
		"metallb.io": {
			{file: "ipaddresspool_v1beta1.json", kind: "IPAddressPool", source: schemametadata.SchemaSourceCRD},
			{file: "l2advertisement_v1beta1.json", kind: "L2Advertisement", source: schemametadata.SchemaSourceCRD},
		},
		"volsync.backube": {
			{file: "replicationsource_v1alpha1.json", kind: "ReplicationSource", source: schemametadata.SchemaSourceCRD},
			{file: "replicationdestination_v1alpha1.json", kind: "ReplicationDestination", source: schemametadata.SchemaSourceCRD},
		},
		"core": {
			{file: "pod_v1.json", kind: "Pod", source: schemametadata.SchemaSourceBuiltin},
			{file: "service_v1.json", kind: "Service", source: schemametadata.SchemaSourceBuiltin},
		},
		"apps": {
			{file: "deployment_v1.json", kind: "Deployment", source: schemametadata.SchemaSourceBuiltin},
			{file: "statefulset_v1.json", kind: "StatefulSet", source: schemametadata.SchemaSourceBuiltin},
		},
		"kustomize.config.k8s.io": {
			{file: "kustomization_v1beta1.json", kind: "Kustomization", source: schemametadata.SchemaSourceKustomize},
		},
	}
	kinds := make(map[string]string)
	metadata := make(map[string]schemametadata.SchemaMetadataEntry)
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
			relPath := filepath.ToSlash(filepath.Join(group, schema.file))
			kinds[relPath] = schema.kind
			metadata[relPath] = schemametadata.SchemaMetadataEntry{
				Kind:   schema.kind,
				Source: schema.source,
			}
		}
	}
	manifestDir := filepath.Join(dir, schemametadata.MetadataDirName)
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		return fmt.Errorf("creating metadata dir: %w", err)
	}
	manifestBytes, err := json.MarshalIndent(kinds, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding kind manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, schemametadata.KindsManifestName), append(manifestBytes, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing kind manifest: %w", err)
	}
	metadataBytes, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding schema metadata manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, schemametadata.SchemaMetadataManifestName), append(metadataBytes, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing schema metadata manifest: %w", err)
	}
	return nil
}
