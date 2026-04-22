package main

import (
	"fmt"
	"os"
	"path/filepath"
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
