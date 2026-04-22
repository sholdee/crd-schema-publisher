package extractor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func seedActiveGeneration(t *testing.T, outputDir string, files map[string]string) string {
	t.Helper()

	generationsDir := filepath.Join(outputDir, ".generations")
	if err := os.MkdirAll(generationsDir, 0o755); err != nil {
		t.Fatalf("creating generations dir: %v", err)
	}

	generationDir := filepath.Join(generationsDir, "seed")
	if err := os.MkdirAll(generationDir, 0o755); err != nil {
		t.Fatalf("creating generation dir: %v", err)
	}
	for rel, content := range files {
		path := filepath.Join(generationDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("creating parent dir for %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("writing %s: %v", rel, err)
		}
	}

	currentPath := filepath.Join(outputDir, "current")
	if err := os.Symlink(filepath.Join(".generations", "seed"), currentPath); err != nil {
		t.Fatalf("creating current symlink: %v", err)
	}

	return generationDir
}

func currentTarget(t *testing.T, outputDir string) string {
	t.Helper()

	target, err := os.Readlink(filepath.Join(outputDir, "current"))
	if err != nil {
		t.Fatalf("reading current symlink: %v", err)
	}
	return target
}

func TestValidateOutputDir(t *testing.T) {
	t.Run("rejects catastrophic targets", func(t *testing.T) {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}

		rootLink := filepath.Join(t.TempDir(), "root-link")
		if err := os.Symlink(string(filepath.Separator), rootLink); err != nil {
			t.Fatalf("symlink root: %v", err)
		}

		tests := []struct {
			name  string
			input string
		}{
			{name: "empty", input: ""},
			{name: "dot", input: "."},
			{name: "dotdot", input: ".."},
			{name: "root", input: string(filepath.Separator)},
			{name: "cwd", input: cwd},
			{name: "symlinked root", input: rootLink},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if err := ValidateOutputDir(tt.input); err == nil {
					t.Fatalf("ValidateOutputDir(%q) expected error", tt.input)
				}
			})
		}
	})

	t.Run("allows dedicated directories", func(t *testing.T) {
		for _, dir := range []string{
			filepath.Join(t.TempDir(), "output"),
			filepath.Join("testdata", "output"),
		} {
			if err := ValidateOutputDir(dir); err != nil {
				t.Fatalf("ValidateOutputDir(%q) unexpected error: %v", dir, err)
			}
		}
	})
}

func TestBuildSite_ZeroCRDsIsNoopAndPreservesOutput(t *testing.T) {
	outputDir := t.TempDir()
	seedActiveGeneration(t, outputDir, map[string]string{
		"index.html": "keep",
	})
	before := currentTarget(t, outputDir)

	result, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: nil},
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatalf("BuildSite error: %v", err)
	}
	if result.Status != BuildResultNoop {
		t.Fatalf("expected BuildResultNoop, got %q", result.Status)
	}
	if got := currentTarget(t, outputDir); got != before {
		t.Fatalf("expected current to remain %q, got %q", before, got)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "index.html")); err != nil {
		t.Fatalf("expected active generation to remain readable: %v", err)
	}
}

func TestBuildSite_SuccessCreatesGenerationAndSwitchesCurrent(t *testing.T) {
	outputDir := t.TempDir()
	seedActiveGeneration(t, outputDir, map[string]string{
		"index.html": "old",
	})
	before := currentTarget(t, outputDir)

	result, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatalf("BuildSite error: %v", err)
	}
	if result.Status != BuildResultBuilt {
		t.Fatalf("expected BuildResultBuilt, got %q", result.Status)
	}
	after := currentTarget(t, outputDir)
	if after == before {
		t.Fatalf("expected current to switch generations, stayed at %q", before)
	}
	if !strings.HasPrefix(after, ".generations") {
		t.Fatalf("expected current target under .generations, got %q", after)
	}
	targetInfo, err := os.Stat(filepath.Join(outputDir, after))
	if err != nil {
		t.Fatalf("stat current target: %v", err)
	}
	if got := targetInfo.Mode().Perm(); got != 0o755 {
		t.Fatalf("expected active generation dir perms 0755, got %#o", got)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "example.io", "test_v1.json")); err != nil {
		t.Fatalf("expected schema output under current: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "index.html")); err != nil {
		t.Fatalf("expected index output under current: %v", err)
	}
}

func TestBuildSite_SuccessPrunesSupersededGenerations(t *testing.T) {
	outputDir := t.TempDir()
	seedActiveGeneration(t, outputDir, map[string]string{
		"index.html": "previous",
	})

	staleDir := filepath.Join(outputDir, ".generations", "stale")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatalf("mkdir stale generation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, "index.html"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale index: %v", err)
	}

	result, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatalf("BuildSite error: %v", err)
	}
	if result.Status != BuildResultBuilt {
		t.Fatalf("expected BuildResultBuilt, got %q", result.Status)
	}

	generations, err := os.ReadDir(filepath.Join(outputDir, ".generations"))
	if err != nil {
		t.Fatalf("read generations dir: %v", err)
	}
	if len(generations) != 2 {
		names := make([]string, 0, len(generations))
		for _, generation := range generations {
			names = append(names, generation.Name())
		}
		t.Fatalf("expected current and previous generations only, got %v", names)
	}
	for _, generation := range generations {
		if generation.Name() == "stale" {
			t.Fatal("expected stale superseded generation to be pruned")
		}
	}
}

func TestBuildSite_FailurePreservesPreviousOutput(t *testing.T) {
	outputDir := t.TempDir()
	seedActiveGeneration(t, outputDir, map[string]string{
		"index.html": "keep",
	})
	before := currentTarget(t, outputDir)

	orig := generateIndexFunc
	generateIndexFunc = func(string, string) error {
		return fmt.Errorf("boom")
	}
	defer func() {
		generateIndexFunc = orig
	}()

	_, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir: outputDir,
	})
	if err == nil {
		t.Fatal("expected BuildSite error")
	}
	if !strings.Contains(err.Error(), "generating index") {
		t.Fatalf("expected index error, got %v", err)
	}
	if got := currentTarget(t, outputDir); got != before {
		t.Fatalf("expected current to remain %q, got %q", before, got)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "index.html")); err != nil {
		t.Fatalf("expected prior active generation to remain readable: %v", err)
	}
}

func TestBuildSite_PostActivationFailureKeepsActiveGenerationReadable(t *testing.T) {
	outputDir := t.TempDir()
	seedActiveGeneration(t, outputDir, map[string]string{
		"index.html": "previous",
	})
	before := currentTarget(t, outputDir)

	orig := pruneGenerationsFunc
	pruneGenerationsFunc = func(string, ...string) error {
		return fmt.Errorf("prune boom")
	}
	defer func() {
		pruneGenerationsFunc = orig
	}()

	_, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir: outputDir,
	})
	if err == nil {
		t.Fatal("expected BuildSite error")
	}
	if !strings.Contains(err.Error(), "pruning generations") {
		t.Fatalf("expected pruning error, got %v", err)
	}

	after := currentTarget(t, outputDir)
	if after == before {
		t.Fatalf("expected current to switch generations, stayed at %q", before)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "index.html")); err != nil {
		t.Fatalf("expected activated generation to remain readable: %v", err)
	}
}

func TestBuildSite_RenderFailurePreservesPreviousOutput(t *testing.T) {
	outputDir := t.TempDir()
	seedActiveGeneration(t, outputDir, map[string]string{
		"index.html": "keep",
	})
	before := currentTarget(t, outputDir)

	orig := renderAllFunc
	renderAllFunc = func(string, string) error {
		return fmt.Errorf("render boom")
	}
	defer func() {
		renderAllFunc = orig
	}()

	_, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir: outputDir,
		Render:    true,
	})
	if err == nil {
		t.Fatal("expected BuildSite error")
	}
	if !strings.Contains(err.Error(), "rendering schemas") {
		t.Fatalf("expected render error, got %v", err)
	}
	if got := currentTarget(t, outputDir); got != before {
		t.Fatalf("expected current to remain %q, got %q", before, got)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "index.html")); err != nil {
		t.Fatalf("expected prior active generation to remain readable: %v", err)
	}
}

func TestBuildSite_WriteFailurePreservesPreviousOutput(t *testing.T) {
	outputDir := t.TempDir()
	seedActiveGeneration(t, outputDir, map[string]string{
		"index.html": "keep",
	})
	before := currentTarget(t, outputDir)

	orig := writeSchemasFunc
	writeSchemasFunc = func([]apiextensionsv1.CustomResourceDefinition, string) (int, error) {
		return 0, fmt.Errorf("write boom")
	}
	defer func() {
		writeSchemasFunc = orig
	}()

	_, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir: outputDir,
	})
	if err == nil {
		t.Fatal("expected BuildSite error")
	}
	if !strings.Contains(err.Error(), "writing schemas") {
		t.Fatalf("expected write error, got %v", err)
	}
	if got := currentTarget(t, outputDir); got != before {
		t.Fatalf("expected current to remain %q, got %q", before, got)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "index.html")); err != nil {
		t.Fatalf("expected prior active generation to remain readable: %v", err)
	}
}
