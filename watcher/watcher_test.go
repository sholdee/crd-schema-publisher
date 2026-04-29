package watcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sholdee/crd-schema-publisher/extractor"
	"github.com/sholdee/crd-schema-publisher/metrics"
	"github.com/sholdee/crd-schema-publisher/publisher"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// --- helpers ---

type fakeLister struct {
	crds []apiextensionsv1.CustomResourceDefinition
	err  error
}

func (f *fakeLister) List(_ context.Context, _ metav1.ListOptions) (*apiextensionsv1.CustomResourceDefinitionList, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &apiextensionsv1.CustomResourceDefinitionList{Items: f.crds}, nil
}

func testCRD() apiextensionsv1.CustomResourceDefinition {
	return apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "tests.example.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: "Test"},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name: "v1",
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"spec": {Type: "object"},
						},
					},
				},
			}},
		},
	}
}

// --- debounce tests ---

func TestDebounce_FirstTriggerFiresImmediately(t *testing.T) {
	var fired atomic.Bool
	trigger := make(chan struct{}, 1)
	done := make(chan struct{})

	go debounceLoop(trigger, 200*time.Millisecond, func() error {
		fired.Store(true)
		return nil
	}, nil, done)

	trigger <- struct{}{}

	// First trigger should fire well before the 200ms debounce duration
	time.Sleep(50 * time.Millisecond)
	if !fired.Load() {
		t.Fatal("expected first trigger to fire immediately, but it did not")
	}
	close(done)
}

func TestDebounce_SubsequentTriggerIsDebounced(t *testing.T) {
	var count atomic.Int32
	trigger := make(chan struct{}, 1)
	done := make(chan struct{})

	go debounceLoop(trigger, 200*time.Millisecond, func() error {
		count.Add(1)
		return nil
	}, nil, done)

	// First trigger — fires immediately
	trigger <- struct{}{}
	time.Sleep(50 * time.Millisecond)
	if c := count.Load(); c != 1 {
		t.Fatalf("expected 1 publish after first trigger, got %d", c)
	}

	// Second trigger — should be debounced
	trigger <- struct{}{}
	time.Sleep(50 * time.Millisecond)
	if c := count.Load(); c != 1 {
		t.Fatalf("expected second trigger to be debounced (still 1), got %d", c)
	}

	// Wait for debounce to fire
	time.Sleep(300 * time.Millisecond)
	if c := count.Load(); c != 2 {
		t.Fatalf("expected 2 publishes after debounce, got %d", c)
	}
	close(done)
}

func TestDebounce_CoalescesRapidEvents(t *testing.T) {
	var count atomic.Int32
	trigger := make(chan struct{}, 10)
	done := make(chan struct{})

	go debounceLoop(trigger, 100*time.Millisecond, func() error {
		count.Add(1)
		return nil
	}, nil, done)

	// First trigger fires immediately
	trigger <- struct{}{}
	time.Sleep(50 * time.Millisecond)

	// Send 4 more events in rapid succession — these should coalesce into 1 debounced publish
	for range 4 {
		trigger <- struct{}{}
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for debounce to fire
	time.Sleep(300 * time.Millisecond)
	close(done)

	// 1 immediate + 1 debounced = 2 total (not 5)
	if c := count.Load(); c != 2 {
		t.Fatalf("expected 2 publish cycles (1 immediate + 1 debounced), got %d", c)
	}
}

func TestDebounce_SkipsWhenPublishInProgress(t *testing.T) {
	var count atomic.Int32
	m := metrics.New()
	trigger := make(chan struct{}, 10)
	done := make(chan struct{})

	go debounceLoop(trigger, 50*time.Millisecond, func() error {
		count.Add(1)
		// Simulate slow publish
		time.Sleep(300 * time.Millisecond)
		return nil
	}, m, done)

	// First event triggers publish
	trigger <- struct{}{}
	time.Sleep(100 * time.Millisecond) // debounce fires, publish starts (takes 300ms)

	// Second event during publish — debounce fires but publish in progress, skip
	trigger <- struct{}{}
	time.Sleep(100 * time.Millisecond) // debounce fires while first publish still running

	// Wait for everything to settle
	time.Sleep(500 * time.Millisecond)
	close(done)

	// Only 1 publish should have run (second was skipped)
	if c := count.Load(); c != 1 {
		t.Fatalf("expected 1 publish cycle (second skipped), got %d", c)
	}

	// Verify skip was recorded in metrics
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "crdpublisher_publish_skipped_total 1") {
		t.Fatalf("expected publish_skipped_total=1 in:\n%s", rec.Body.String())
	}
}

func TestDebounce_DrainsInFlightPublishOnShutdown(t *testing.T) {
	var count atomic.Int32
	trigger := make(chan struct{}, 10)
	done := make(chan struct{})

	var loopDone sync.WaitGroup
	loopDone.Add(1)

	go func() {
		defer loopDone.Done()
		debounceLoop(trigger, 50*time.Millisecond, func() error {
			count.Add(1)
			// Simulate slow publish
			time.Sleep(500 * time.Millisecond)
			return nil
		}, nil, done)
	}()

	// Trigger a publish
	trigger <- struct{}{}
	time.Sleep(100 * time.Millisecond) // debounce fires, publish starts

	// Shut down while publish is in progress
	close(done)

	// debounceLoop should block until publish completes, then return
	loopDone.Wait()

	// The in-flight publish must have completed (not been killed)
	if c := count.Load(); c != 1 {
		t.Fatalf("expected 1 publish cycle to complete during drain, got %d", c)
	}
}

// --- publishCycle tests ---

func TestPublishCycle_HappyPath(t *testing.T) {
	t.Setenv("SKIP_RENDER", "true")
	dir := t.TempDir()

	cfg := Config{
		OutputDir: dir,
		CRDLister: &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{testCRD()}},
	}

	if err := publishCycle(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify schema files exist
	schemaPath := filepath.Join(dir, "current", "example.io", "test_v1.json")
	if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
		t.Fatalf("expected schema file at %s", schemaPath)
	}

	// Verify index.html exists
	indexPath := filepath.Join(dir, "current", "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Fatalf("expected index.html at %s", indexPath)
	}
}

func TestPublishCycle_AppliesConfiguredFilter(t *testing.T) {
	t.Setenv("SKIP_RENDER", "true")
	dir := t.TempDir()
	keepGen := filepath.Join(dir, ".generations", "seed")
	if err := os.MkdirAll(filepath.Join(keepGen, "example.io"), 0o755); err != nil {
		t.Fatalf("mkdir seed generation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(keepGen, "index.html"), []byte("old index"), 0o644); err != nil {
		t.Fatalf("write seed index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(keepGen, "example.io", "test_v1.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write seed schema: %v", err)
	}
	if err := os.Symlink(filepath.Join(".generations", "seed"), filepath.Join(dir, "current")); err != nil {
		t.Fatalf("symlink current: %v", err)
	}

	cfg := Config{
		OutputDir: dir,
		CRDLister: &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{testCRD()}},
		Filter:    extractor.ParseFilter("missing", "", ""),
	}

	if err := publishCycle(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "current", "index.html")); err != nil {
		t.Fatalf("expected empty filtered generation index: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "current", "example.io", "test_v1.json")); !os.IsNotExist(err) {
		t.Fatalf("expected stale schema to be absent after filtered publish cycle, got err=%v", err)
	}
}

func TestPublishCycle_ExtractError(t *testing.T) {
	t.Setenv("SKIP_RENDER", "true")
	dir := t.TempDir()
	keepGen := filepath.Join(dir, ".generations", "seed")
	if err := os.MkdirAll(keepGen, 0o755); err != nil {
		t.Fatalf("mkdir seed generation: %v", err)
	}
	keepPath := filepath.Join(keepGen, "index.html")
	if err := os.WriteFile(keepPath, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep file: %v", err)
	}
	if err := os.Symlink(filepath.Join(".generations", "seed"), filepath.Join(dir, "current")); err != nil {
		t.Fatalf("symlink current: %v", err)
	}

	cfg := Config{
		OutputDir: dir,
		CRDLister: &fakeLister{err: fmt.Errorf("API unavailable")},
	}

	err := publishCycle(cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "listing CRDs") {
		t.Fatalf("expected error containing 'listing CRDs', got: %s", got)
	}

	if _, err := os.Stat(filepath.Join(dir, "current", "index.html")); err != nil {
		t.Fatalf("expected existing output to be preserved: %v", err)
	}
}

func TestPublishCycle_EmptyCRDs(t *testing.T) {
	t.Setenv("SKIP_RENDER", "true")
	dir := t.TempDir()
	keepGen := filepath.Join(dir, ".generations", "seed")
	if err := os.MkdirAll(keepGen, 0o755); err != nil {
		t.Fatalf("mkdir seed generation: %v", err)
	}
	keepPath := filepath.Join(keepGen, "index.html")
	if err := os.WriteFile(keepPath, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep file: %v", err)
	}
	if err := os.Symlink(filepath.Join(".generations", "seed"), filepath.Join(dir, "current")); err != nil {
		t.Fatalf("symlink current: %v", err)
	}

	cfg := Config{
		OutputDir: dir,
		CRDLister: &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{}},
	}

	if err := publishCycle(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "current", "index.html")); err != nil {
		t.Fatalf("expected existing output to remain for zero CRDs: %v", err)
	}
}

func TestPublishCycle_UploadError(t *testing.T) {
	t.Setenv("SKIP_RENDER", "true")
	dir := t.TempDir()

	// Mock server that returns 500 for all requests
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"message":"server error"}]}`))
	}))
	defer srv.Close()

	cfg := Config{
		OutputDir: dir,
		CRDLister: &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{testCRD()}},
		Publisher: &publisher.Publisher{
			BaseURL:     srv.URL,
			AssetsURL:   srv.URL,
			APIToken:    "t",
			AccountID:   "a",
			ProjectName: "p",
			SleepFunc:   func(time.Duration) {},
		},
	}

	err := publishCycle(cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "publishing") {
		t.Fatalf("expected error containing 'publishing', got: %s", got)
	}
}

func TestActiveSiteReadyRequiresCurrentIndex(t *testing.T) {
	dir := t.TempDir()
	if activeSiteReady(dir) {
		t.Fatal("expected site to be unready before current/index.html exists")
	}

	generationDir := filepath.Join(dir, ".generations", "ready")
	if err := os.MkdirAll(generationDir, 0o755); err != nil {
		t.Fatalf("mkdir generation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(generationDir, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.Symlink(filepath.Join(".generations", "ready"), filepath.Join(dir, "current")); err != nil {
		t.Fatalf("symlink current: %v", err)
	}

	if !activeSiteReady(dir) {
		t.Fatal("expected site to be ready when current/index.html exists")
	}
}

func TestSiteReadyCheckerLogsOnceWhenSiteBecomesReady(t *testing.T) {
	var logs bytes.Buffer
	orig := slog.Default()
	defer slog.SetDefault(orig)
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))

	dir := t.TempDir()
	checkReady := newSiteReadyChecker(dir)
	if checkReady() {
		t.Fatal("expected site to be unready before current/index.html exists")
	}
	if logs.Len() != 0 {
		t.Fatalf("expected no readiness log before site is ready, got %q", logs.String())
	}

	generationDir := filepath.Join(dir, ".generations", "ready")
	if err := os.MkdirAll(generationDir, 0o755); err != nil {
		t.Fatalf("mkdir generation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(generationDir, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.Symlink(filepath.Join(".generations", "ready"), filepath.Join(dir, "current")); err != nil {
		t.Fatalf("symlink current: %v", err)
	}

	if !checkReady() {
		t.Fatal("expected site to be ready when current/index.html exists")
	}
	if !checkReady() {
		t.Fatal("expected site to stay ready")
	}

	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly one readiness log, got %d: %q", len(lines), logs.String())
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("decode readiness log: %v", err)
	}
	if entry["msg"] != "site ready" {
		t.Fatalf("expected site ready log message, got %#v", entry["msg"])
	}
	if entry["dir"] != filepath.Join(dir, "current") {
		t.Fatalf("expected active dir in log, got %#v", entry["dir"])
	}
}

// --- metrics integration tests ---

func TestHealthServer_MetricsEndpoint(t *testing.T) {
	m := metrics.New()
	m.RecordPublishCycle(2*time.Second, nil)
	m.RecordDiscovery(5, 12)
	m.SetLeader(true)

	mux := http.NewServeMux()
	mux.Handle("/metrics", m.Handler())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("unexpected Content-Type: %s", ct)
	}
}

func TestPublishCycle_RecordsMetrics(t *testing.T) {
	t.Setenv("SKIP_RENDER", "true")
	dir := t.TempDir()
	m := metrics.New()

	cfg := Config{
		OutputDir: dir,
		CRDLister: &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{testCRD()}},
		Metrics:   m,
	}

	if err := publishCycle(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify metrics were recorded
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()

	if !strings.Contains(body, "crdpublisher_crds_discovered 1") {
		t.Errorf("expected crds_discovered=1 in:\n%s", body)
	}
	if !strings.Contains(body, `crdpublisher_publish_cycle_total{result="success"} 1`) {
		t.Errorf("expected success=1 in:\n%s", body)
	}
}

// --- cleanDir tests ---

func TestCleanDir_RemovesContents(t *testing.T) {
	dir := t.TempDir()

	// Create some files and subdirs
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "nested.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cleanDir(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Dir should exist but be empty
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty dir, got %d entries", len(entries))
	}
}

func TestCleanDir_CreatesIfNotExist(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent", "nested")

	if err := cleanDir(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("expected dir to be created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected a directory")
	}
}
