package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sholdee/crd-schema-publisher/extractor"
)

func TestRunAll_SkipsUploadOnNoopBuild(t *testing.T) {
	buildOrig := buildSiteFunc
	publishOrig := publishOutputFunc
	defer func() {
		buildSiteFunc = buildOrig
		publishOutputFunc = publishOrig
	}()

	buildSiteFunc = func(extractor.SiteBuildOptions) (extractor.SiteBuildResult, error) {
		return extractor.SiteBuildResult{Status: extractor.BuildResultNoop}, nil
	}

	called := false
	publishOutputFunc = func() error {
		called = true
		return nil
	}

	if err := runAll(); err != nil {
		t.Fatalf("runAll error: %v", err)
	}
	if called {
		t.Fatal("expected publish to be skipped for no-op build")
	}
}

func TestRunPreview_ValidatesExplicitOutputDir(t *testing.T) {
	validateOrig := validateOutputDirFunc
	defer func() {
		validateOutputDirFunc = validateOrig
	}()

	dir := t.TempDir()
	t.Setenv("OUTPUT_DIR", dir)
	t.Setenv("SKIP_RENDER", "true")

	validateOutputDirFunc = func(path string) error {
		if path != dir {
			t.Fatalf("expected validator to receive %q, got %q", dir, path)
		}
		return fmt.Errorf("unsafe output dir")
	}

	err := runPreview()
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
	if _, err := os.Stat(filepath.Join(serveDir, "monitoring.coreos.com", "servicemonitor_v1.kind")); err != nil {
		t.Fatalf("expected sample kind sidecar under current: %v", err)
	}

	cleanup()
	if _, err := os.Stat(rootDir); !os.IsNotExist(err) {
		t.Fatalf("expected cleanup to remove temp preview root, got err=%v", err)
	}
}

func TestPreparePreviewSite_UsesCurrentForExplicitOutputDir(t *testing.T) {
	validateOrig := validateOutputDirFunc
	renderOrig := renderPreviewFunc
	indexOrig := generatePreviewFunc
	defer func() {
		validateOutputDirFunc = validateOrig
		renderPreviewFunc = renderOrig
		generatePreviewFunc = indexOrig
	}()

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

	validateOutputDirFunc = func(path string) error { return nil }

	renderDir := ""
	indexDir := ""
	renderPreviewFunc = func(dir, basePath string) error {
		renderDir = dir
		return nil
	}
	generatePreviewFunc = func(dir, basePath string) error {
		indexDir = dir
		return nil
	}

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
	if _, err := os.Stat(filepath.Join(rootDir, "current", "index.html")); err != nil {
		t.Fatalf("expected source current to remain readable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(serveDir, "example.io", "test_v1.json")); err != nil {
		t.Fatalf("expected copied schema under preview current: %v", err)
	}
	sourceBytes, err := os.ReadFile(filepath.Join(rootDir, "current", "index.html"))
	if err != nil {
		t.Fatalf("read source index: %v", err)
	}
	if string(sourceBytes) != "original" {
		t.Fatalf("expected source current to remain unchanged, got %q", string(sourceBytes))
	}
}

func TestPreparePreviewSite_FailureDoesNotMutateExplicitOutputDir(t *testing.T) {
	validateOrig := validateOutputDirFunc
	renderOrig := renderPreviewFunc
	defer func() {
		validateOutputDirFunc = validateOrig
		renderPreviewFunc = renderOrig
	}()

	rootDir := t.TempDir()
	generationDir := filepath.Join(rootDir, ".generations", "seed")
	if err := os.MkdirAll(generationDir, 0o755); err != nil {
		t.Fatalf("mkdir generation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(generationDir, "index.html"), []byte("original"), 0o644); err != nil {
		t.Fatalf("write source index: %v", err)
	}
	if err := os.Symlink(filepath.Join(".generations", "seed"), filepath.Join(rootDir, "current")); err != nil {
		t.Fatalf("symlink current: %v", err)
	}

	validateOutputDirFunc = func(path string) error { return nil }
	renderPreviewFunc = func(dir, basePath string) error {
		return fmt.Errorf("render boom")
	}

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

	sourceBytes, readErr := os.ReadFile(filepath.Join(rootDir, "current", "index.html"))
	if readErr != nil {
		t.Fatalf("read source index: %v", readErr)
	}
	if string(sourceBytes) != "original" {
		t.Fatalf("expected source current to remain unchanged, got %q", string(sourceBytes))
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
