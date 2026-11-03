package watcher

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sholdee/crd-schema-publisher/extractor"
	"github.com/sholdee/crd-schema-publisher/metrics"
)

// scriptedDetector returns a fixed sequence of results, then repeats the last.
type scriptedDetector struct {
	mu      sync.Mutex
	results []scriptedResult
	calls   int
}

type scriptedResult struct {
	changed bool
	err     error
}

func (d *scriptedDetector) OpenAPIV2Changed(context.Context) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	i := d.calls
	if i >= len(d.results) {
		i = len(d.results) - 1
	}
	d.calls++
	return d.results[i].changed, d.results[i].err
}

// plainSource satisfies OpenAPISource without change detection.
type plainSource struct{}

func (plainSource) FetchOpenAPIV2(context.Context) ([]byte, error) { return nil, nil }

// detectingSource satisfies both interfaces for startBuiltinsRefresh.
type detectingSource struct {
	plainSource
	*scriptedDetector
}

func TestCheckBuiltinsRefresh_SignalsOnlyOnChange(t *testing.T) {
	trigger := make(chan struct{}, 1)
	m := metrics.New()
	detector := &scriptedDetector{results: []scriptedResult{
		{changed: false},
		{err: errors.New("boom")},
		{changed: true},
	}}

	checkBuiltinsRefresh(context.Background(), detector, trigger, m)
	checkBuiltinsRefresh(context.Background(), detector, trigger, m)
	select {
	case <-trigger:
		t.Fatal("unchanged or failed checks must not signal the trigger")
	default:
	}

	checkBuiltinsRefresh(context.Background(), detector, trigger, m)
	select {
	case <-trigger:
	default:
		t.Fatal("a detected change must signal the trigger")
	}
	assertMetricsContain(t, m,
		`crdpublisher_builtins_refresh_total{result="unchanged"} 1`,
		`crdpublisher_builtins_refresh_total{result="error"} 1`,
		`crdpublisher_builtins_refresh_total{result="changed"} 1`,
	)
}

func TestRunBuiltinsRefresh_PollsUntilCancelled(t *testing.T) {
	trigger := make(chan struct{}, 1)
	detector := &scriptedDetector{results: []scriptedResult{{changed: true}}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runBuiltinsRefresh(ctx, detector, 5*time.Millisecond, trigger, nil)
		close(done)
	}()

	select {
	case <-trigger:
	case <-time.After(5 * time.Second):
		t.Fatal("expected the ticker to drive a change check and signal the trigger")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh loop did not stop on context cancellation")
	}
}

func TestStartBuiltinsRefresh_Gating(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	trigger := make(chan struct{}, 1)
	detector := &scriptedDetector{results: []scriptedResult{{changed: false}}}

	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"disabled without built-ins", Config{IncludeBuiltins: false, BuiltinsRefreshInterval: time.Minute, OpenAPISource: detectingSource{scriptedDetector: detector}}, false},
		{"disabled with zero interval", Config{IncludeBuiltins: true, BuiltinsRefreshInterval: 0, OpenAPISource: detectingSource{scriptedDetector: detector}}, false},
		{"disabled when source cannot detect changes", Config{IncludeBuiltins: true, BuiltinsRefreshInterval: time.Minute, OpenAPISource: plainSource{}}, false},
		{"disabled with nil source", Config{IncludeBuiltins: true, BuiltinsRefreshInterval: time.Minute, OpenAPISource: nil}, false},
		{"enabled", Config{IncludeBuiltins: true, BuiltinsRefreshInterval: time.Hour, OpenAPISource: detectingSource{scriptedDetector: detector}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := startBuiltinsRefresh(ctx, tc.cfg, trigger); got != tc.want {
				t.Fatalf("startBuiltinsRefresh = %v, want %v", got, tc.want)
			}
		})
	}
}

func assertMetricsContain(t *testing.T, m *metrics.Metrics, lines ...string) {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()
	for _, line := range lines {
		if !strings.Contains(body, line) {
			t.Errorf("metrics output missing %q:\n%s", line, body)
		}
	}
}

// Compile-time check that the real source implements the detector interface
// the watcher gates on, so a refactor cannot silently disable refresh.
var _ extractor.OpenAPIChangeDetector = (*extractor.APIServerOpenAPISource)(nil)
