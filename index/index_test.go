package index

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func normalizeHTMLForContract(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return strings.Join(out, "\n")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func writeIndexFixtureSchemas(t *testing.T, outputDir string) {
	t.Helper()
	groupDir := filepath.Join(outputDir, "example.io")
	if err := os.MkdirAll(groupDir, 0o755); err != nil {
		t.Fatalf("mkdir group dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(groupDir, "test_v1.json"), []byte(`{"type":"object"}`), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	if err := os.WriteFile(filepath.Join(groupDir, "test_v1.html"), []byte(`<html></html>`), 0o644); err != nil {
		t.Fatalf("write schema html: %v", err)
	}
}

func TestGenerate_CreatesIndexHTML(t *testing.T) {
	tmpDir := t.TempDir()
	// 2 groups, 3 total schemas — distinct counts so we can verify each stat independently
	_ = os.MkdirAll(filepath.Join(tmpDir, "cert-manager.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "cert-manager.io", "certificate_v1.json"), []byte(`{}`), 0o644)
	_ = os.WriteFile(filepath.Join(tmpDir, "cert-manager.io", "issuer_v1.json"), []byte(`{}`), 0o644)
	_ = os.MkdirAll(filepath.Join(tmpDir, "monitoring.coreos.com"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "monitoring.coreos.com", "servicemonitor_v1.json"), []byte(`{}`), 0o644)

	err := Generate(tmpDir, "")
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
		{"JSON schemas extracted from live CustomResourceDefinitions", "precise index subtitle"},
		{"id=\"search\"", "search input"},
		{`aria-controls="groups"`, "index search controls result groups"},
		{"class=\"search-input-wrap\"", "shared search input wrapper"},
		{`type="button" class="search-clear" id="search-clear" aria-label="Clear search" title="Clear search" hidden></button>`, "index search clear button"},
		{`id="search-status" role="status" aria-live="polite" aria-atomic="true"`, "index search live status"},
		{"min-height: 1.25rem; overflow-wrap: anywhere;", "visible search status wraps on narrow screens"},
		{"--surface-background: linear-gradient(var(--bg-surface), var(--bg-surface)), var(--bg);", "opaque surface background token"},
		{"background: var(--surface-background);", "text surfaces hide starfield"},
		{"background: var(--surface-hover-background);", "text surface hover hides starfield"},
		{".search-clear {", "shared search clear button style"},
		{"background: var(--accent-dim);", "subtle search clear hover background"},
		{"toggleTheme", "theme toggle JS"},
		{"documentElement.className", "FOUC prevention script in head"},
		{`e.key === '/'`, "slash keyboard shortcut"},
		{"yaml-language-server", "usage section"},
		{"data-url=\"/", "copy URL data attribute"},
		{`class="schema-row" data-schema="certificate_v1.json"`, "schema row wrapper for link and copy action"},
		{`type="button" class="schema-copy" data-url="/cert-manager.io/certificate_v1.json" aria-label="Copy schema URL for certificate_v1.json" title="Copy schema URL">copy URL</button>`, "keyboard-accessible schema copy button"},
		{".schemas a {\n    flex: 0 1 auto;", "schema link keeps copy button adjacent instead of pushing it to the row edge"},
		{"min-height: 1.5rem;", "schema links meet 24px target size"},
		{".group-name { flex: 1; min-width: 0; overflow-wrap: anywhere; }", "group names wrap on narrow screens"},
		{"function copyURLWithToast(url)", "shared copy toast helper"},
		{`<div class="copied-toast" id="toast" role="status" aria-live="polite" aria-atomic="true"></div>`, "empty live copy toast element"},
		{"id=\"stat-groups\"", "group stat ID for JS update"},
		{"id=\"stat-schemas\"", "schema stat ID for JS update"},
		{"id=\"toggle-all\"", "expand/collapse all button"},
		{"#q=", "URL hash deep-link support"},
		{"history.replaceState(null, '', q ? '#q=' + encodeURIComponent(q) : location.pathname);", "hash-based URL sync"},
		{"try {\n      return decodeURIComponent(hash.slice(3));\n    } catch (err) {\n      return '';\n    }", "malformed hash decoding is guarded"},
		{"function hasHashSearchQuery(){\n  return (location.hash || '').indexOf('#q=') === 0;\n}", "shared helper detects explicit hash search state"},
		{"if (!hasHashSearchQuery() && saved) {", "saved index state does not override explicit hash deep links"},
		{"input.dispatchEvent(new Event('input'));\n      } else if (document.activeElement === input) {\n        input.blur();", "escape clears search before blurring focused input"},
		{"id=\"back-to-top\"", "back to top button"},
		{"focus-visible", "keyboard focus outlines"},
		{"favicon.svg", "favicon link tag"},
	}
	for _, c := range checks {
		if !strings.Contains(html, c.substr) {
			t.Errorf("index should contain %s (looked for %q)", c.desc, c.substr)
		}
	}
	if strings.Contains(html, "copy-hint") {
		t.Error("index should use real copy buttons instead of hover-only copy hint spans")
	}
}

func TestGenerate_IndexPageContract(t *testing.T) {
	tmpDir := t.TempDir()
	writeIndexFixtureSchemas(t, tmpDir)
	if err := Generate(tmpDir, "/docs"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	page := readFile(t, filepath.Join(tmpDir, "index.html"))
	contract := normalizeHTMLForContract(page)
	for _, needle := range []string{
		`href="/docs/example.io/test_v1.html"`,
		`readHashSearchQuery`,
		`writeHashSearchQuery`,
		`copyURLWithToast`,
		`localStorage.getItem('theme')`,
		`history.replaceState`,
	} {
		if !strings.Contains(contract, needle) {
			t.Fatalf("index page contract missing %q", needle)
		}
	}
}

func TestGenerate_SkipsMasterStandalone(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "example.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "example.io", "test_v1.json"), []byte(`{}`), 0o644)
	_ = os.MkdirAll(filepath.Join(tmpDir, "master-standalone"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "master-standalone", "example.io-test-stable-v1.json"), []byte(`{}`), 0o644)

	err := Generate(tmpDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "index.html"))
	html := string(data)
	if strings.Contains(html, "master-standalone") {
		t.Fatal("index should not list master-standalone directory")
	}
}

func TestGenerate_SkipsMetadataDir(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "example.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "example.io", "test_v1.json"), []byte(`{}`), 0o644)
	_ = os.MkdirAll(filepath.Join(tmpDir, "_meta"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "_meta", "kinds.json"), []byte(`{"example.io/test_v1.json":"Test"}`), 0o644)

	err := Generate(tmpDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "index.html"))
	html := string(data)
	if strings.Contains(html, "_meta") {
		t.Fatal("index should not list metadata directory")
	}
	if !strings.Contains(html, ">1</strong> API groups") {
		t.Fatal("metadata directory should not affect group count")
	}
	if !strings.Contains(html, ">1</strong> schemas") {
		t.Fatal("metadata directory should not affect schema count")
	}
}

func TestGenerate_ManySchemasFewGroups(t *testing.T) {
	tmpDir := t.TempDir()
	// 1 group with 5 schemas — tests that group count and schema count diverge correctly
	_ = os.MkdirAll(filepath.Join(tmpDir, "flux.io"), 0o755)
	for _, s := range []string{"a_v1.json", "b_v1.json", "c_v1.json", "d_v1.json", "e_v1.json"} {
		_ = os.WriteFile(filepath.Join(tmpDir, "flux.io", s), []byte(`{}`), 0o644)
	}

	err := Generate(tmpDir, "")
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

	err := Generate(tmpDir, "")
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

func TestGenerate_CreatesFavicon(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "example.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "example.io", "test_v1.json"), []byte(`{}`), 0o644)

	err := Generate(tmpDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "favicon.svg"))
	if err != nil {
		t.Fatalf("favicon.svg not created: %v", err)
	}

	svg := string(data)
	checks := []struct {
		substr string
		desc   string
	}{
		{"<svg", "SVG root element"},
		{"viewBox", "viewBox attribute"},
		{"<circle", "vertex circles"},
		{"<line", "edge lines"},
		{"#6bc1fe", "accent blue color"},
		{"#fff", "white vertex fill"},
	}
	for _, c := range checks {
		if !strings.Contains(svg, c.substr) {
			t.Errorf("favicon should contain %s (looked for %q)", c.desc, c.substr)
		}
	}
}

func TestGenerate_LinksToHTMLWhenPresent(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "example.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "example.io", "thing_v1.json"), []byte(`{}`), 0o644)
	_ = os.WriteFile(filepath.Join(tmpDir, "example.io", "thing_v1.html"), []byte(`<html></html>`), 0o644)

	err := Generate(tmpDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "index.html"))
	html := string(data)

	if !strings.Contains(html, `href="/example.io/thing_v1.html"`) {
		t.Error("schema link should point to .html when HTML file exists")
	}
	if !strings.Contains(html, `data-url="/example.io/thing_v1.json"`) {
		t.Error("data-url should always point to .json for copy behavior")
	}
}

func TestGenerate_FallsBackToJSONWhenNoHTML(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "example.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "example.io", "thing_v1.json"), []byte(`{}`), 0o644)

	err := Generate(tmpDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "index.html"))
	html := string(data)

	if !strings.Contains(html, `href="/example.io/thing_v1.json"`) {
		t.Error("schema link should fall back to .json when no HTML file exists")
	}
}

func TestGenerate_SkipsNonJsonFiles(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "example.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "example.io", "thing_v1.json"), []byte(`{}`), 0o644)
	_ = os.WriteFile(filepath.Join(tmpDir, "example.io", "README.md"), []byte(`# hello`), 0o644)
	_ = os.WriteFile(filepath.Join(tmpDir, "example.io", ".gitkeep"), []byte(``), 0o644)

	err := Generate(tmpDir, "")
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

func TestGenerate_BasePath(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "cert-manager.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "cert-manager.io", "certificate_v1.json"), []byte(`{}`), 0o644)
	_ = os.WriteFile(filepath.Join(tmpDir, "cert-manager.io", "certificate_v1.html"), []byte(`<html></html>`), 0o644)

	err := Generate(tmpDir, "/iac")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "index.html"))
	html := string(data)

	checks := []struct {
		substr string
		desc   string
	}{
		{`href="/iac/favicon.svg"`, "favicon with base path"},
		{`href="/iac/cert-manager.io/certificate_v1.html"`, "schema link with base path"},
		{`data-url="/iac/cert-manager.io/certificate_v1.json"`, "data-url with base path"},
		{`data-base-path="/iac"`, "body data-base-path attribute"},
		{`document.body.dataset.basePath`, "usage example URL includes base path via data attr"},
	}
	for _, c := range checks {
		if !strings.Contains(html, c.substr) {
			t.Errorf("index should contain %s (looked for %q)", c.desc, c.substr)
		}
	}
}

func TestGenerate_EmptyBasePath(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "example.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "example.io", "thing_v1.json"), []byte(`{}`), 0o644)

	err := Generate(tmpDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "index.html"))
	html := string(data)

	if !strings.Contains(html, `href="/favicon.svg"`) {
		t.Error("empty base path should produce root-relative favicon")
	}
	if !strings.Contains(html, `href="/example.io/thing_v1.json"`) {
		t.Error("empty base path should produce root-relative schema links")
	}
}
