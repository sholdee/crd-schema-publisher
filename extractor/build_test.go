package extractor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

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
	keepPath := filepath.Join(outputDir, "keep.txt")
	if err := os.WriteFile(keepPath, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep file: %v", err)
	}

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
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("expected keep file to remain: %v", err)
	}
}

func TestBuildSite_SuccessReplacesPreviousOutput(t *testing.T) {
	outputDir := t.TempDir()
	stalePath := filepath.Join(outputDir, "stale.txt")
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
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
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("expected stale file to be removed, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "example.io", "test_v1.json")); err != nil {
		t.Fatalf("expected schema output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "index.html")); err != nil {
		t.Fatalf("expected index output: %v", err)
	}
}

func TestBuildSite_FailurePreservesPreviousOutput(t *testing.T) {
	outputDir := t.TempDir()
	keepPath := filepath.Join(outputDir, "keep.txt")
	if err := os.WriteFile(keepPath, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep file: %v", err)
	}

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
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("expected keep file to remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "example.io", "test_v1.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no promoted schema output on failure, got err=%v", err)
	}
}

func TestBuildSite_RenderFailurePreservesPreviousOutput(t *testing.T) {
	outputDir := t.TempDir()
	keepPath := filepath.Join(outputDir, "keep.txt")
	if err := os.WriteFile(keepPath, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep file: %v", err)
	}

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
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("expected keep file to remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "example.io", "test_v1.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no promoted schema output on render failure, got err=%v", err)
	}
}

func TestBuildSite_WriteFailurePreservesPreviousOutput(t *testing.T) {
	outputDir := t.TempDir()
	keepPath := filepath.Join(outputDir, "keep.txt")
	if err := os.WriteFile(keepPath, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep file: %v", err)
	}

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
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("expected keep file to remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "example.io", "test_v1.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no promoted schema output on write failure, got err=%v", err)
	}
}
