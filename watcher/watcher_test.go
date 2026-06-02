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
	"k8s.io/client-go/tools/cache"
)

// --- helpers ---

type contextKey string

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

const watcherOpenAPI = `{
  "definitions": {
    "io.k8s.api.core.v1.Pod": {
      "type": "object",
      "x-kubernetes-group-version-kind": [{"group": "", "version": "v1", "kind": "Pod"}],
      "properties": {
        "apiVersion": {"type": "string"},
        "kind": {"type": "string"}
      }
    }
  }
}`

type recordingLister struct {
	ctx  context.Context
	crds []apiextensionsv1.CustomResourceDefinition
}

func (r *recordingLister) List(ctx context.Context, _ metav1.ListOptions) (*apiextensionsv1.CustomResourceDefinitionList, error) {
	r.ctx = ctx
	return &apiextensionsv1.CustomResourceDefinitionList{Items: r.crds}, nil
}

type recordingOpenAPISource struct {
	ctx context.Context
	raw []byte
}

func (r *recordingOpenAPISource) FetchOpenAPIV2(ctx context.Context) ([]byte, error) {
	r.ctx = ctx
	return r.raw, nil
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

func waitUntil(t *testing.T, deadline time.Duration, cond func() bool) {
	t.Helper()
	timeout := time.After(deadline)
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-timeout:
			t.Fatal("condition not met before deadline")
		case <-tick.C:
			if cond() {
				return
			}
		}
	}
}

func waitForClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("channel did not close before deadline")
	}
}

type fakeDebounceClock struct {
	mu     sync.Mutex
	timers []*fakeDebounceTimer
	afters []chan time.Time
}

func (c *fakeDebounceClock) NewTimer(d time.Duration) debounceTimer {
	timer := &fakeDebounceTimer{c: make(chan time.Time, 1), duration: d}
	c.mu.Lock()
	c.timers = append(c.timers, timer)
	c.mu.Unlock()
	return timer
}

func (c *fakeDebounceClock) NewTicker(time.Duration) debounceTicker {
	return &fakeDebounceTicker{c: make(chan time.Time, 1)}
}

func (c *fakeDebounceClock) After(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	c.mu.Lock()
	c.afters = append(c.afters, ch)
	c.mu.Unlock()
	return ch
}

func (c *fakeDebounceClock) timerCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

func (c *fakeDebounceClock) afterCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.afters)
}

func (c *fakeDebounceClock) latestTimerDuration() time.Duration {
	c.mu.Lock()
	timer := c.timers[len(c.timers)-1]
	c.mu.Unlock()
	return timer.currentDuration()
}

func (c *fakeDebounceClock) latestTimerResetCount() int {
	c.mu.Lock()
	timer := c.timers[len(c.timers)-1]
	c.mu.Unlock()
	return timer.resetCount()
}

func (c *fakeDebounceClock) fireLatestTimer() {
	c.mu.Lock()
	timer := c.timers[len(c.timers)-1]
	c.mu.Unlock()
	timer.fire()
}

type fakeDebounceTimer struct {
	mu       sync.Mutex
	c        chan time.Time
	duration time.Duration
	stopped  bool
	resets   int
}

func (t *fakeDebounceTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	active := !t.stopped
	t.stopped = true
	return active
}

func (t *fakeDebounceTimer) Reset(d time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	active := !t.stopped
	t.duration = d
	t.stopped = false
	t.resets++
	return active
}

func (t *fakeDebounceTimer) C() <-chan time.Time {
	return t.c
}

func (t *fakeDebounceTimer) currentDuration() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.duration
}

func (t *fakeDebounceTimer) resetCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.resets
}

func (t *fakeDebounceTimer) fire() {
	select {
	case t.c <- time.Now():
	default:
	}
}

type fakeDebounceTicker struct {
	c chan time.Time
}

func (t *fakeDebounceTicker) Stop() {}

func (t *fakeDebounceTicker) C() <-chan time.Time {
	return t.c
}

func startDebounceWithClock(trigger <-chan struct{}, duration time.Duration, publish func() error, m *metrics.Metrics, done <-chan struct{}, clock debounceClock) <-chan struct{} {
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		debounceLoopWithClock(trigger, duration, publish, m, done, clock)
	}()
	return loopDone
}

type syncedController struct{}

func (syncedController) Run(stopCh <-chan struct{}) {
	<-stopCh
}

func (syncedController) RunWithContext(ctx context.Context) {
	<-ctx.Done()
}

func (syncedController) HasSynced() bool {
	return true
}

func (syncedController) HasSyncedChecker() cache.DoneChecker {
	return nil
}

func (syncedController) LastSyncResourceVersion() string {
	return ""
}

// --- lifecycle tests ---

func TestRunLeader_PublishesOptionalSchemasWithZeroCRDs(t *testing.T) {
	t.Setenv("SKIP_RENDER", "true")
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := Config{
		OutputDir:        dir,
		CRDLister:        &fakeLister{},
		Debounce:         time.Hour,
		Metrics:          metrics.New(),
		IncludeBuiltins:  true,
		IncludeKustomize: true,
		OpenAPISource:    &recordingOpenAPISource{raw: []byte(watcherOpenAPI)},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		runLeaderWithController(ctx, cfg, make(chan struct{}, 1), syncedController{})
	}()

	waitUntil(t, time.Second, func() bool {
		_, podErr := os.Stat(filepath.Join(dir, "current", "core", "pod_v1.json"))
		_, kustomizeErr := os.Stat(filepath.Join(dir, "current", "kustomize.config.k8s.io", "kustomization_v1beta1.json"))
		return podErr == nil && kustomizeErr == nil
	})
	cancel()
	waitForClosed(t, done)
}

// --- debounce tests ---

func TestDebounceClock_FirstTriggerFiresImmediately(t *testing.T) {
	var count atomic.Int32
	trigger := make(chan struct{}, 1)
	done := make(chan struct{})
	clock := &fakeDebounceClock{}

	loopDone := startDebounceWithClock(trigger, time.Hour, func() error {
		count.Add(1)
		return nil
	}, nil, done, clock)

	trigger <- struct{}{}
	waitUntil(t, time.Second, func() bool { return clock.timerCount() == 1 })

	if got := clock.latestTimerDuration(); got != 0 {
		t.Fatalf("expected first trigger to use zero-duration timer, got %s", got)
	}
	clock.fireLatestTimer()
	waitUntil(t, time.Second, func() bool { return count.Load() == 1 })
	close(done)
	waitForClosed(t, loopDone)
}

func TestDebounceClock_SubsequentTriggerWaitsForTimer(t *testing.T) {
	var count atomic.Int32
	trigger := make(chan struct{}, 1)
	done := make(chan struct{})
	clock := &fakeDebounceClock{}
	debounceDuration := time.Hour

	loopDone := startDebounceWithClock(trigger, debounceDuration, func() error {
		count.Add(1)
		return nil
	}, nil, done, clock)

	// First trigger — fires immediately
	trigger <- struct{}{}
	waitUntil(t, time.Second, func() bool { return clock.timerCount() == 1 })
	clock.fireLatestTimer()
	waitUntil(t, time.Second, func() bool { return count.Load() == 1 })

	// Second trigger — should be debounced
	trigger <- struct{}{}
	waitUntil(t, time.Second, func() bool { return clock.timerCount() == 2 })
	if got := clock.latestTimerDuration(); got != debounceDuration {
		t.Fatalf("expected subsequent trigger to wait %s, got %s", debounceDuration, got)
	}
	if c := count.Load(); c != 1 {
		t.Fatalf("expected second trigger to be debounced (still 1), got %d", c)
	}

	clock.fireLatestTimer()
	waitUntil(t, time.Second, func() bool { return count.Load() == 2 })
	close(done)
	waitForClosed(t, loopDone)
}

func TestDebounceClock_CoalescesRapidEvents(t *testing.T) {
	var count atomic.Int32
	trigger := make(chan struct{}, 10)
	done := make(chan struct{})
	clock := &fakeDebounceClock{}

	loopDone := startDebounceWithClock(trigger, time.Hour, func() error {
		count.Add(1)
		return nil
	}, nil, done, clock)

	trigger <- struct{}{}
	waitUntil(t, time.Second, func() bool { return clock.timerCount() == 1 })
	clock.fireLatestTimer()
	waitUntil(t, time.Second, func() bool { return count.Load() == 1 })

	for range 3 {
		trigger <- struct{}{}
	}
	waitUntil(t, time.Second, func() bool {
		return clock.timerCount() == 2 && clock.latestTimerResetCount() == 2
	})

	clock.fireLatestTimer()
	waitUntil(t, time.Second, func() bool { return count.Load() == 2 })
	close(done)
	waitForClosed(t, loopDone)
}

func TestDebounceClock_SkipsWhenPublishInProgress(t *testing.T) {
	var count atomic.Int32
	publishStarted := make(chan struct{})
	releasePublish := make(chan struct{})
	m := metrics.New()
	trigger := make(chan struct{}, 10)
	done := make(chan struct{})
	clock := &fakeDebounceClock{}

	loopDone := startDebounceWithClock(trigger, time.Hour, func() error {
		count.Add(1)
		close(publishStarted)
		<-releasePublish
		return nil
	}, m, done, clock)

	// First trigger fires immediately
	trigger <- struct{}{}
	waitUntil(t, time.Second, func() bool { return clock.timerCount() == 1 })
	clock.fireLatestTimer()
	waitForClosed(t, publishStarted)

	// Second event during publish — debounce fires but publish in progress, skip
	trigger <- struct{}{}
	waitUntil(t, time.Second, func() bool { return clock.timerCount() == 2 })
	clock.fireLatestTimer()
	waitUntil(t, time.Second, func() bool {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/metrics", nil)
		m.Handler().ServeHTTP(rec, req)
		return strings.Contains(rec.Body.String(), "crdpublisher_publish_skipped_total 1")
	})

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
	close(releasePublish)
	close(done)
	waitForClosed(t, loopDone)
}

func TestDebounceClock_ShutdownDrainsInFlightPublish(t *testing.T) {
	var count atomic.Int32
	publishStarted := make(chan struct{})
	releasePublish := make(chan struct{})
	trigger := make(chan struct{}, 10)
	done := make(chan struct{})
	clock := &fakeDebounceClock{}

	loopDone := startDebounceWithClock(trigger, time.Hour, func() error {
		count.Add(1)
		close(publishStarted)
		<-releasePublish
		return nil
	}, nil, done, clock)

	// Trigger a publish
	trigger <- struct{}{}
	waitUntil(t, time.Second, func() bool { return clock.timerCount() == 1 })
	clock.fireLatestTimer()
	waitForClosed(t, publishStarted)

	// Shut down while publish is in progress
	close(done)
	waitUntil(t, time.Second, func() bool { return clock.afterCount() == 1 })

	select {
	case <-loopDone:
		t.Fatal("debounceLoop returned before in-flight publish completed")
	default:
	}

	close(releasePublish)
	waitForClosed(t, loopDone)

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

func TestPublishCycle_IncludesOptionalSchemasAndContext(t *testing.T) {
	t.Setenv("SKIP_RENDER", "true")
	dir := t.TempDir()
	ctx := context.WithValue(context.Background(), contextKey("publish"), "leader")
	lister := &recordingLister{}
	source := &recordingOpenAPISource{raw: []byte(watcherOpenAPI)}

	cfg := Config{
		Context:          ctx,
		OutputDir:        dir,
		CRDLister:        lister,
		IncludeBuiltins:  true,
		IncludeKustomize: true,
		OpenAPISource:    source,
	}

	if err := publishCycle(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lister.ctx != ctx {
		t.Fatal("expected publish context to flow to CRD listing")
	}
	if source.ctx != ctx {
		t.Fatal("expected publish context to flow to OpenAPI fetch")
	}
	if _, err := os.Stat(filepath.Join(dir, "current", "core", "pod_v1.json")); err != nil {
		t.Fatalf("expected Pod schema: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "current", "kustomize.config.k8s.io", "kustomization_v1beta1.json")); err != nil {
		t.Fatalf("expected Kustomization schema: %v", err)
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
