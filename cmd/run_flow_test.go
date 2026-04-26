package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sholdee/crd-schema-publisher/extractor"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/rest"
)

func TestRunAll_SkipsUploadOnNoopBuild(t *testing.T) {
	clientOrig := buildClientFunc
	buildOrig := buildSiteFunc
	publishOrig := publishOutputFunc
	defer func() {
		buildClientFunc = clientOrig
		buildSiteFunc = buildOrig
		publishOutputFunc = publishOrig
	}()

	buildClientFunc = func(string) (*apiextensionsclient.Clientset, error) {
		return apiextensionsclient.NewForConfig(&rest.Config{Host: "https://example.invalid"})
	}
	buildSiteFunc = func(extractor.SiteBuildOptions) (extractor.SiteBuildResult, error) {
		return extractor.SiteBuildResult{Status: extractor.BuildResultNoop}, nil
	}

	called := false
	publishOutputFunc = func(string) error {
		called = true
		return nil
	}

	dir := t.TempDir()
	t.Setenv("OUTPUT_DIR", dir)

	if err := runAll(nil); err != nil {
		t.Fatalf("runAll error: %v", err)
	}
	if called {
		t.Fatal("expected publish to be skipped for no-op build")
	}
}

func TestRunAll_RequiresPreexistingOutputDir(t *testing.T) {
	clientOrig := buildClientFunc
	defer func() {
		buildClientFunc = clientOrig
	}()

	called := false
	buildClientFunc = func(string) (*apiextensionsclient.Clientset, error) {
		called = true
		return apiextensionsclient.NewForConfig(&rest.Config{Host: "https://example.invalid"})
	}

	missing := filepath.Join(t.TempDir(), "missing")
	t.Setenv("OUTPUT_DIR", missing)

	err := runAll(nil)
	if err == nil {
		t.Fatal("expected runAll error")
	}
	if !strings.Contains(err.Error(), "must already exist") {
		t.Fatalf("expected explicit missing output dir error, got %v", err)
	}
	if !strings.Contains(err.Error(), "Set OUTPUT_DIR or pass --output-dir") {
		t.Fatalf("expected actionable error, got %v", err)
	}
	if called {
		t.Fatal("expected runAll to fail before building kubernetes client")
	}
}

func TestRunAll_OutputDirFlagOverridesEnvVar(t *testing.T) {
	clientOrig := buildClientFunc
	buildOrig := buildSiteFunc
	publishOrig := publishOutputFunc
	defer func() {
		buildClientFunc = clientOrig
		buildSiteFunc = buildOrig
		publishOutputFunc = publishOrig
	}()

	var captured extractor.SiteBuildOptions
	buildClientFunc = func(string) (*apiextensionsclient.Clientset, error) {
		return apiextensionsclient.NewForConfig(&rest.Config{Host: "https://example.invalid"})
	}
	buildSiteFunc = func(opts extractor.SiteBuildOptions) (extractor.SiteBuildResult, error) {
		captured = opts
		return extractor.SiteBuildResult{Status: extractor.BuildResultNoop}, nil
	}
	publishOutputFunc = func(string) error {
		t.Fatal("publish should not be called for noop build")
		return nil
	}

	envDir := t.TempDir()
	flagDir := t.TempDir()
	t.Setenv("OUTPUT_DIR", envDir)

	if err := runAll([]string{"--output-dir", flagDir}); err != nil {
		t.Fatalf("runAll error: %v", err)
	}
	if captured.OutputDir != flagDir {
		t.Fatalf("expected flag output dir %q, got %q", flagDir, captured.OutputDir)
	}
}

func TestRunAll_OutputDirShortFlagOverridesEnvVar(t *testing.T) {
	clientOrig := buildClientFunc
	buildOrig := buildSiteFunc
	publishOrig := publishOutputFunc
	defer func() {
		buildClientFunc = clientOrig
		buildSiteFunc = buildOrig
		publishOutputFunc = publishOrig
	}()

	var captured extractor.SiteBuildOptions
	buildClientFunc = func(string) (*apiextensionsclient.Clientset, error) {
		return apiextensionsclient.NewForConfig(&rest.Config{Host: "https://example.invalid"})
	}
	buildSiteFunc = func(opts extractor.SiteBuildOptions) (extractor.SiteBuildResult, error) {
		captured = opts
		return extractor.SiteBuildResult{Status: extractor.BuildResultNoop}, nil
	}
	publishOutputFunc = func(string) error {
		t.Fatal("publish should not be called for noop build")
		return nil
	}

	envDir := t.TempDir()
	flagDir := t.TempDir()
	t.Setenv("OUTPUT_DIR", envDir)

	if err := runAll([]string{"-o", flagDir}); err != nil {
		t.Fatalf("runAll error: %v", err)
	}
	if captured.OutputDir != flagDir {
		t.Fatalf("expected short output dir %q, got %q", flagDir, captured.OutputDir)
	}
}

func TestRunAll_UsesSchemaFilterEnvVars(t *testing.T) {
	clientOrig := buildClientFunc
	buildOrig := buildSiteFunc
	publishOrig := publishOutputFunc
	defer func() {
		buildClientFunc = clientOrig
		buildSiteFunc = buildOrig
		publishOutputFunc = publishOrig
	}()

	var captured extractor.SiteBuildOptions
	buildClientFunc = func(string) (*apiextensionsclient.Clientset, error) {
		return apiextensionsclient.NewForConfig(&rest.Config{Host: "https://example.invalid"})
	}
	buildSiteFunc = func(opts extractor.SiteBuildOptions) (extractor.SiteBuildResult, error) {
		captured = opts
		return extractor.SiteBuildResult{Status: extractor.BuildResultNoop}, nil
	}
	publishOutputFunc = func(string) error {
		t.Fatal("publish should not be called for noop build")
		return nil
	}

	t.Setenv("OUTPUT_DIR", t.TempDir())
	t.Setenv("SCHEMA_FILTER_KIND", "Certificate,Issuer")
	t.Setenv("SCHEMA_FILTER_GROUP", "Cert-Manager.IO")
	t.Setenv("SCHEMA_FILTER_VERSION", "V1")

	if err := runAll(nil); err != nil {
		t.Fatalf("runAll error: %v", err)
	}
	if got := strings.Join(captured.Filter.Kinds, ","); got != "certificate,issuer" {
		t.Fatalf("expected env kind filter, got %q", got)
	}
	if got := strings.Join(captured.Filter.Groups, ","); got != "cert-manager.io" {
		t.Fatalf("expected env group filter, got %q", got)
	}
	if got := strings.Join(captured.Filter.Versions, ","); got != "v1" {
		t.Fatalf("expected env version filter, got %q", got)
	}
}

func TestRunAll_FilterFlagsOverrideEnvVars(t *testing.T) {
	clientOrig := buildClientFunc
	buildOrig := buildSiteFunc
	publishOrig := publishOutputFunc
	defer func() {
		buildClientFunc = clientOrig
		buildSiteFunc = buildOrig
		publishOutputFunc = publishOrig
	}()

	var captured extractor.SiteBuildOptions
	buildClientFunc = func(string) (*apiextensionsclient.Clientset, error) {
		return apiextensionsclient.NewForConfig(&rest.Config{Host: "https://example.invalid"})
	}
	buildSiteFunc = func(opts extractor.SiteBuildOptions) (extractor.SiteBuildResult, error) {
		captured = opts
		return extractor.SiteBuildResult{Status: extractor.BuildResultNoop}, nil
	}
	publishOutputFunc = func(string) error {
		t.Fatal("publish should not be called for noop build")
		return nil
	}

	t.Setenv("OUTPUT_DIR", t.TempDir())
	t.Setenv("SCHEMA_FILTER_KIND", "Certificate")
	t.Setenv("SCHEMA_FILTER_GROUP", "cert-manager.io")
	t.Setenv("SCHEMA_FILTER_VERSION", "v1")

	err := runAll([]string{
		"--kind", "Issuer",
		"--group", "issuers.example.io",
		"--version", "v1alpha1",
	})
	if err != nil {
		t.Fatalf("runAll error: %v", err)
	}
	if got := strings.Join(captured.Filter.Kinds, ","); got != "issuer" {
		t.Fatalf("expected flag kind filter, got %q", got)
	}
	if got := strings.Join(captured.Filter.Groups, ","); got != "issuers.example.io" {
		t.Fatalf("expected flag group filter, got %q", got)
	}
	if got := strings.Join(captured.Filter.Versions, ","); got != "v1alpha1" {
		t.Fatalf("expected flag version filter, got %q", got)
	}
}

func TestRunAll_RejectsUnexpectedPositionalArgs(t *testing.T) {
	clientOrig := buildClientFunc
	defer func() {
		buildClientFunc = clientOrig
	}()

	called := false
	buildClientFunc = func(string) (*apiextensionsclient.Clientset, error) {
		called = true
		return apiextensionsclient.NewForConfig(&rest.Config{Host: "https://example.invalid"})
	}

	dir := t.TempDir()
	err := runAll([]string{"--output-dir", dir, "upload"})
	if err == nil {
		t.Fatal("expected runAll error")
	}
	if !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("expected unexpected arguments error, got %v", err)
	}
	if called {
		t.Fatal("expected runAll to fail before building kubernetes client")
	}
}

func TestRunPreview_ValidatesExplicitOutputDir(t *testing.T) {
	validateOrig := validateOutputDirFunc
	defer func() {
		validateOutputDirFunc = validateOrig
	}()

	dir := t.TempDir()
	t.Setenv("SKIP_RENDER", "true")

	validateOutputDirFunc = func(path string) error {
		if path != dir {
			t.Fatalf("expected validator to receive %q, got %q", dir, path)
		}
		return fmt.Errorf("unsafe output dir")
	}

	err := runPreview([]string{"--output-dir", dir})
	if err == nil {
		t.Fatal("expected runPreview error")
	}
	if err.Error() != "unsafe output dir" {
		t.Fatalf("expected validator error, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "index.html")); !os.IsNotExist(err) {
		t.Fatalf("expected preview to stop before mutating output dir, got err=%v", err)
	}
}

func TestRunPreview_IgnoresOutputDirEnvWithoutFlag(t *testing.T) {
	prepareOrig := preparePreviewSiteFunc
	validateOrig := validateOutputDirFunc
	defer func() {
		preparePreviewSiteFunc = prepareOrig
		validateOutputDirFunc = validateOrig
	}()

	envDir := t.TempDir()
	t.Setenv("OUTPUT_DIR", envDir)
	t.Setenv("PREVIEW_ADDR", "127.0.0.1:0")
	t.Setenv("SKIP_RENDER", "true")

	validateOutputDirFunc = func(path string) error {
		t.Fatalf("did not expect validator to run for ambient OUTPUT_DIR %q", path)
		return nil
	}

	called := false
	preparePreviewSiteFunc = func(outputDir, basePath string, render bool) (string, func(), error) {
		called = true
		if outputDir != "" {
			t.Fatalf("expected ambient OUTPUT_DIR to be ignored, got %q", outputDir)
		}
		return "", func() {}, fmt.Errorf("preview stub stop")
	}

	err := runPreview(nil)
	if err == nil {
		t.Fatal("expected runPreview error")
	}
	if err.Error() != "preview stub stop" {
		t.Fatalf("expected stub error, got %v", err)
	}
	if !called {
		t.Fatal("expected preparePreviewSite to be called")
	}
}

func TestRunPreview_OutputDirFlagOverridesEnvVar(t *testing.T) {
	validateOrig := validateOutputDirFunc
	defer func() {
		validateOutputDirFunc = validateOrig
	}()

	envDir := t.TempDir()
	flagDir := t.TempDir()
	t.Setenv("OUTPUT_DIR", envDir)
	t.Setenv("SKIP_RENDER", "true")

	validateOutputDirFunc = func(path string) error {
		if path != flagDir {
			t.Fatalf("expected validator to receive %q, got %q", flagDir, path)
		}
		return fmt.Errorf("preview stop")
	}

	err := runPreview([]string{"--output-dir", flagDir})
	if err == nil {
		t.Fatal("expected runPreview error")
	}
	if err.Error() != "preview stop" {
		t.Fatalf("expected validator error, got %v", err)
	}
}

func TestRunPreview_RejectsExplicitEmptyOutputDir(t *testing.T) {
	err := runPreview([]string{"--output-dir", ""})
	if err == nil {
		t.Fatal("expected runPreview error")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("expected empty output dir error, got %v", err)
	}
	if !strings.Contains(err.Error(), "Pass --output-dir") {
		t.Fatalf("expected actionable guidance, got %v", err)
	}
	if strings.Contains(err.Error(), "Set OUTPUT_DIR") {
		t.Fatalf("did not expect preview error to mention ambient OUTPUT_DIR, got %v", err)
	}
}

func TestRunUpload_OutputDirFlagOverridesEnvVar(t *testing.T) {
	envDir := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing")
	t.Setenv("OUTPUT_DIR", envDir)
	t.Setenv("CLOUDFLARE_API_TOKEN", "")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "")

	err := runUpload([]string{"--output-dir", missing})
	if err == nil {
		t.Fatal("expected runUpload error")
	}
	if !strings.Contains(err.Error(), "must already exist") {
		t.Fatalf("expected missing output dir error, got %v", err)
	}
	if strings.Contains(err.Error(), "CLOUDFLARE_API_TOKEN") {
		t.Fatalf("expected output-dir validation before credential checks, got %v", err)
	}
}

func TestRunUpload_RejectsNonDirectoryOutputDirWithGuidance(t *testing.T) {
	file := filepath.Join(t.TempDir(), "output-file")
	if err := os.WriteFile(file, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runUpload([]string{"--output-dir", file})
	if err == nil {
		t.Fatal("expected runUpload error")
	}
	if !strings.Contains(err.Error(), "is not a directory") {
		t.Fatalf("expected non-directory error, got %v", err)
	}
	if !strings.Contains(err.Error(), "Set OUTPUT_DIR or pass --output-dir") {
		t.Fatalf("expected actionable guidance, got %v", err)
	}
}

func TestRunWatch_OutputDirFlagOverridesEnvVar(t *testing.T) {
	envDir := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing")
	t.Setenv("OUTPUT_DIR", envDir)

	err := runWatch([]string{"--output-dir", missing})
	if err == nil {
		t.Fatal("expected runWatch error")
	}
	if !strings.Contains(err.Error(), "must already exist") {
		t.Fatalf("expected missing output dir error, got %v", err)
	}
	if strings.Contains(err.Error(), "POD_NAME") || strings.Contains(err.Error(), "building kubeconfig") {
		t.Fatalf("expected output-dir validation before runtime setup, got %v", err)
	}
}

func TestPreparePreviewSite_CreatesSampleGenerationUnderCurrent(t *testing.T) {
	renderOrig := renderPreviewFunc
	indexOrig := generatePreviewFunc
	scaffoldOrig := scaffoldPreviewFunc
	defer func() {
		renderPreviewFunc = renderOrig
		generatePreviewFunc = indexOrig
		scaffoldPreviewFunc = scaffoldOrig
	}()

	renderDir := ""
	renderPreviewFunc = func(dir, basePath string) error {
		renderDir = dir
		return nil
	}
	generatePreviewFunc = func(dir, basePath string) error {
		return os.WriteFile(filepath.Join(dir, "index.html"), []byte("preview"), 0o644)
	}

	serveDir, cleanup, err := preparePreviewSite("", "", true)
	if err != nil {
		t.Fatalf("preparePreviewSite error: %v", err)
	}
	rootDir := filepath.Dir(serveDir)
	defer cleanup()

	if !strings.HasSuffix(serveDir, filepath.Join("", "current")) {
		t.Fatalf("expected active preview dir to end with current, got %q", serveDir)
	}
	if renderDir == "" {
		t.Fatal("expected render to run for sample preview")
	}
	if _, err := os.Lstat(filepath.Join(rootDir, "current")); err != nil {
		t.Fatalf("expected current symlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "index.html")); !os.IsNotExist(err) {
		t.Fatalf("expected preview root to stay empty, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(serveDir, "index.html")); err != nil {
		t.Fatalf("expected generated index under current: %v", err)
	}
	manifestPath := filepath.Join(serveDir, "_meta", "kinds.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("expected sample kind manifest under current: %v", err)
	}
	var manifest map[string]string
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("parse sample kind manifest: %v", err)
	}
	if got := manifest["monitoring.coreos.com/servicemonitor_v1.json"]; got != "ServiceMonitor" {
		t.Fatalf("expected ServiceMonitor in sample kind manifest, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(serveDir, "monitoring.coreos.com", "servicemonitor_v1.kind")); !os.IsNotExist(err) {
		t.Fatalf("expected sample preview to stop writing .kind sidecars, got err=%v", err)
	}

	cleanup()
	if _, err := os.Stat(rootDir); !os.IsNotExist(err) {
		t.Fatalf("expected cleanup to remove temp preview root, got err=%v", err)
	}
}

func seedPreviewSource(t *testing.T) string {
	t.Helper()
	rootDir := t.TempDir()
	generationDir := filepath.Join(rootDir, ".generations", "seed")
	if err := os.MkdirAll(filepath.Join(generationDir, "example.io"), 0o755); err != nil {
		t.Fatalf("mkdir generation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(generationDir, "index.html"), []byte("original"), 0o644); err != nil {
		t.Fatalf("write source index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(generationDir, "example.io", "test_v1.json"), []byte(`{"type":"object"}`), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	if err := os.Symlink(filepath.Join(".generations", "seed"), filepath.Join(rootDir, "current")); err != nil {
		t.Fatalf("symlink current: %v", err)
	}
	return rootDir
}

func withPreviewHooks(t *testing.T, renderFn func(string, string) error, indexFn func(string, string) error) {
	t.Helper()
	validateOrig := validateOutputDirFunc
	renderOrig := renderPreviewFunc
	indexOrig := generatePreviewFunc
	t.Cleanup(func() {
		validateOutputDirFunc = validateOrig
		renderPreviewFunc = renderOrig
		generatePreviewFunc = indexOrig
	})
	validateOutputDirFunc = func(path string) error { return nil }
	if renderFn != nil {
		renderPreviewFunc = renderFn
	}
	if indexFn != nil {
		generatePreviewFunc = indexFn
	}
}

func assertPreviewSourceUnchanged(t *testing.T, rootDir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(rootDir, "current", "index.html")); err != nil {
		t.Fatalf("expected source current to remain readable: %v", err)
	}
	sourceBytes, err := os.ReadFile(filepath.Join(rootDir, "current", "index.html"))
	if err != nil {
		t.Fatalf("read source index: %v", err)
	}
	if string(sourceBytes) != "original" {
		t.Fatalf("expected source current to remain unchanged, got %q", string(sourceBytes))
	}
}

func TestPreparePreviewSite_UsesCurrentForExplicitOutputDir(t *testing.T) {
	rootDir := seedPreviewSource(t)

	renderDir := ""
	indexDir := ""
	withPreviewHooks(t, func(dir, basePath string) error {
		renderDir = dir
		return nil
	}, func(dir, basePath string) error {
		indexDir = dir
		return nil
	})

	serveDir, cleanup, err := preparePreviewSite(rootDir, "/docs", true)
	if err != nil {
		t.Fatalf("preparePreviewSite error: %v", err)
	}
	defer cleanup()

	if serveDir == filepath.Join(rootDir, "current") {
		t.Fatalf("expected preview to use an isolated copy, got source current dir %q", serveDir)
	}
	if filepath.Base(serveDir) != "current" {
		t.Fatalf("expected preview serve dir to end with current, got %q", serveDir)
	}
	if filepath.Dir(renderDir) == filepath.Dir(filepath.Join(rootDir, "current")) {
		t.Fatalf("expected render to avoid source output root, got %q", renderDir)
	}
	if filepath.Dir(indexDir) == filepath.Dir(filepath.Join(rootDir, "current")) {
		t.Fatalf("expected index generation to avoid source output root, got %q", indexDir)
	}
	if _, err := os.Stat(filepath.Join(renderDir, "example.io", "test_v1.json")); err != nil {
		t.Fatalf("expected staged preview generation to contain copied schema: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "index.html")); !os.IsNotExist(err) {
		t.Fatalf("expected preview to avoid mutating flat root, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(serveDir, "example.io", "test_v1.json")); err != nil {
		t.Fatalf("expected copied schema under preview current: %v", err)
	}
	assertPreviewSourceUnchanged(t, rootDir)
}

func TestPreparePreviewSite_FailureDoesNotMutateExplicitOutputDir(t *testing.T) {
	rootDir := seedPreviewSource(t)
	withPreviewHooks(t, func(dir, basePath string) error {
		return fmt.Errorf("render boom")
	}, nil)

	_, cleanup, err := preparePreviewSite(rootDir, "/docs", true)
	if cleanup != nil {
		defer cleanup()
	}
	if err == nil {
		t.Fatal("expected preparePreviewSite error")
	}
	if !strings.Contains(err.Error(), "rendering schemas") {
		t.Fatalf("expected render error, got %v", err)
	}
	assertPreviewSourceUnchanged(t, rootDir)
}

func TestRunAll_SkipsUploadWhenNoCredentials(t *testing.T) {
	clientOrig := buildClientFunc
	buildOrig := buildSiteFunc
	publishOrig := publishOutputFunc
	defer func() {
		buildClientFunc = clientOrig
		buildSiteFunc = buildOrig
		publishOutputFunc = publishOrig
	}()

	buildClientFunc = func(string) (*apiextensionsclient.Clientset, error) {
		return apiextensionsclient.NewForConfig(&rest.Config{Host: "https://example.invalid"})
	}
	buildSiteFunc = func(extractor.SiteBuildOptions) (extractor.SiteBuildResult, error) {
		return extractor.SiteBuildResult{Status: extractor.BuildResultBuilt, SchemaCount: 5}, nil
	}

	called := false
	publishOutputFunc = func(string) error {
		called = true
		return nil
	}

	t.Setenv("CLOUDFLARE_API_TOKEN", "")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "")
	t.Setenv("OUTPUT_DIR", t.TempDir())

	if err := runAll(nil); err != nil {
		t.Fatalf("runAll error: %v", err)
	}
	if called {
		t.Fatal("expected publish to be skipped when credentials are missing")
	}
}

func TestRunAll_UploadsWhenCredentialsPresent(t *testing.T) {
	clientOrig := buildClientFunc
	buildOrig := buildSiteFunc
	publishOrig := publishOutputFunc
	defer func() {
		buildClientFunc = clientOrig
		buildSiteFunc = buildOrig
		publishOutputFunc = publishOrig
	}()

	buildClientFunc = func(string) (*apiextensionsclient.Clientset, error) {
		return apiextensionsclient.NewForConfig(&rest.Config{Host: "https://example.invalid"})
	}
	buildSiteFunc = func(extractor.SiteBuildOptions) (extractor.SiteBuildResult, error) {
		return extractor.SiteBuildResult{Status: extractor.BuildResultBuilt, SchemaCount: 5}, nil
	}

	called := false
	publishOutputFunc = func(string) error {
		called = true
		return nil
	}

	t.Setenv("CLOUDFLARE_API_TOKEN", "test-token")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "test-account")
	t.Setenv("OUTPUT_DIR", t.TempDir())

	if err := runAll(nil); err != nil {
		t.Fatalf("runAll error: %v", err)
	}
	if !called {
		t.Fatal("expected publish to be called when credentials are present")
	}
}

func TestRunAll_BuildError_PrintsGuidance(t *testing.T) {
	clientOrig := buildClientFunc
	defer func() {
		buildClientFunc = clientOrig
	}()

	buildClientFunc = func(string) (*apiextensionsclient.Clientset, error) {
		return nil, fmt.Errorf("unable to load kubeconfig")
	}

	t.Setenv("OUTPUT_DIR", t.TempDir())

	err := runAll(nil)
	if err == nil {
		t.Fatal("expected error")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "extract") || !strings.Contains(errMsg, "convert") {
		t.Errorf("expected guidance mentioning extract and convert commands, got: %s", errMsg)
	}
}

func TestPreparePreviewSite_RequiresCurrentForExplicitOutputDir(t *testing.T) {
	validateOrig := validateOutputDirFunc
	defer func() {
		validateOutputDirFunc = validateOrig
	}()

	rootDir := t.TempDir()
	validateOutputDirFunc = func(path string) error { return nil }

	_, cleanup, err := preparePreviewSite(rootDir, "", false)
	if cleanup != nil {
		defer cleanup()
	}
	if err == nil {
		t.Fatal("expected preparePreviewSite error")
	}
	if !strings.Contains(err.Error(), `active output "`) {
		t.Fatalf("expected missing current error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(rootDir, "index.html")); !os.IsNotExist(statErr) {
		t.Fatalf("expected explicit preview without current to leave root untouched, got err=%v", statErr)
	}
}
