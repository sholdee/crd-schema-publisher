package index

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerate_CreatesIndexHTML(t *testing.T) {
	tmpDir := t.TempDir()
	// 2 groups, 3 total schemas — distinct counts so we can verify each stat independently
	os.MkdirAll(filepath.Join(tmpDir, "cert-manager.io"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "cert-manager.io", "certificate_v1.json"), []byte(`{}`), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "cert-manager.io", "issuer_v1.json"), []byte(`{}`), 0o644)
	os.MkdirAll(filepath.Join(tmpDir, "monitoring.coreos.com"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "monitoring.coreos.com", "servicemonitor_v1.json"), []byte(`{}`), 0o644)

	err := Generate(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "index.html"))
	if err != nil {
		t.Fatalf("index.html not created: %v", err)
	}

	html := string(data)
	checks := []struct {
		substr string
		desc   string
	}{
		{"cert-manager.io", "group name cert-manager.io"},
		{"monitoring.coreos.com", "group name monitoring.coreos.com"},
		{"certificate_v1.json", "schema link"},
		{`href="/cert-manager.io/certificate_v1.json"`, "schema link href format"},
		{">2</strong> API groups", "group count stat"},
		{">3</strong> schemas", "total schema count stat"},
		{"id=\"search\"", "search input"},
		{"toggleTheme", "theme toggle JS"},
		{"documentElement.className", "FOUC prevention script in head"},
		{`e.key === '/'`, "slash keyboard shortcut"},
		{`class="flare"`, "flare div"},
		{"body::before", "starfield CSS"},
		{"yaml-language-server", "usage section"},
		{"data-url=\"/", "copy URL data attribute"},
		{"copy-hint", "copy hint span"},
		{"copied-toast", "copy toast element"},
		{"id=\"stat-groups\"", "group stat ID for JS update"},
		{"id=\"stat-schemas\"", "schema stat ID for JS update"},
		{"id=\"toggle-all\"", "expand/collapse all button"},
		{"#q=", "URL hash deep-link support"},
		{"id=\"back-to-top\"", "back to top button"},
		{"focus-visible", "keyboard focus outlines"},
	}
	for _, c := range checks {
		if !strings.Contains(html, c.substr) {
			t.Errorf("index should contain %s (looked for %q)", c.desc, c.substr)
		}
	}
}

func TestGenerate_SkipsMasterStandalone(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "example.io"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "example.io", "test_v1.json"), []byte(`{}`), 0o644)
	os.MkdirAll(filepath.Join(tmpDir, "master-standalone"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "master-standalone", "example.io-test-stable-v1.json"), []byte(`{}`), 0o644)

	err := Generate(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "index.html"))
	html := string(data)
	if strings.Contains(html, "master-standalone") {
		t.Fatal("index should not list master-standalone directory")
	}
}

func TestGenerate_ManySchemasFewGroups(t *testing.T) {
	tmpDir := t.TempDir()
	// 1 group with 5 schemas — tests that group count and schema count diverge correctly
	os.MkdirAll(filepath.Join(tmpDir, "flux.io"), 0o755)
	for _, s := range []string{"a_v1.json", "b_v1.json", "c_v1.json", "d_v1.json", "e_v1.json"} {
		os.WriteFile(filepath.Join(tmpDir, "flux.io", s), []byte(`{}`), 0o644)
	}

	err := Generate(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "index.html"))
	html := string(data)
	if !strings.Contains(html, ">1</strong> API groups") {
		t.Error("should show 1 API group")
	}
	if !strings.Contains(html, ">5</strong> schemas") {
		t.Error("should show 5 total schemas")
	}
}

func TestGenerate_EmptyOutputDir(t *testing.T) {
	tmpDir := t.TempDir()

	err := Generate(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "index.html"))
	if err != nil {
		t.Fatalf("index.html not created: %v", err)
	}

	html := string(data)
	if !strings.Contains(html, ">0</strong> API groups") {
		t.Error("empty dir should show 0 API groups")
	}
	if !strings.Contains(html, ">0</strong> schemas") {
		t.Error("empty dir should show 0 schemas")
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("should produce valid HTML document")
	}
}

func TestGenerate_SkipsNonJsonFiles(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "example.io"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "example.io", "thing_v1.json"), []byte(`{}`), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "example.io", "README.md"), []byte(`# hello`), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "example.io", ".gitkeep"), []byte(``), 0o644)

	err := Generate(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "index.html"))
	html := string(data)
	if strings.Contains(html, "README.md") {
		t.Error("should not list non-JSON files")
	}
	if !strings.Contains(html, ">1</strong> schemas") {
		t.Error("should count only JSON files")
	}
}
