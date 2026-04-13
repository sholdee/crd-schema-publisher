package metrics

import (
	"errors"
	"math"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRecordPublishCycle_Success(t *testing.T) {
	m := New()
	m.RecordPublishCycle(2*time.Second, nil)

	if got := m.publishCycleSuccess.Load(); got != 1 {
		t.Fatalf("expected success=1, got %d", got)
	}
	if got := m.publishCycleError.Load(); got != 0 {
		t.Fatalf("expected error=0, got %d", got)
	}
	dur := math.Float64frombits(uint64(m.durationSeconds.Load()))
	if dur != 2.0 {
		t.Fatalf("expected duration=2.0, got %g", dur)
	}
}

func TestRecordPublishCycle_Error(t *testing.T) {
	m := New()
	m.RecordPublishCycle(500*time.Millisecond, errors.New("fail"))

	if got := m.publishCycleSuccess.Load(); got != 0 {
		t.Fatalf("expected success=0, got %d", got)
	}
	if got := m.publishCycleError.Load(); got != 1 {
		t.Fatalf("expected error=1, got %d", got)
	}
}

func TestRecordDiscovery(t *testing.T) {
	m := New()
	m.RecordDiscovery(42, 100)

	if got := m.crdsDiscovered.Load(); got != 42 {
		t.Fatalf("expected crds=42, got %d", got)
	}
	if got := m.schemasWritten.Load(); got != 100 {
		t.Fatalf("expected schemas=100, got %d", got)
	}
}

func TestRecordSkip(t *testing.T) {
	m := New()
	m.RecordSkip()
	m.RecordSkip()

	if got := m.publishSkipped.Load(); got != 2 {
		t.Fatalf("expected skipped=2, got %d", got)
	}
}

func TestSetLeader(t *testing.T) {
	m := New()
	if got := m.leader.Load(); got != 0 {
		t.Fatalf("expected leader=0 initially, got %d", got)
	}

	m.SetLeader(true)
	if got := m.leader.Load(); got != 1 {
		t.Fatalf("expected leader=1, got %d", got)
	}

	m.SetLeader(false)
	if got := m.leader.Load(); got != 0 {
		t.Fatalf("expected leader=0 after unset, got %d", got)
	}
}

func TestNilReceiver(t *testing.T) {
	var m *Metrics
	// Must not panic
	m.RecordPublishCycle(time.Second, nil)
	m.RecordPublishCycle(time.Second, errors.New("x"))
	m.RecordDiscovery(1, 1)
	m.RecordSkip()
	m.SetLeader(true)
	m.Heartbeat()
}

func TestHeartbeat(t *testing.T) {
	m := New()
	if got := m.watchdogTime.Load(); got != 0 {
		t.Fatalf("expected watchdog=0 initially, got %d", got)
	}

	before := time.Now().Unix()
	m.Heartbeat()
	after := time.Now().Unix()

	got := m.watchdogTime.Load()
	if got < before || got > after {
		t.Fatalf("expected watchdog between %d and %d, got %d", before, after, got)
	}
}

func TestHandler_PrometheusFormat(t *testing.T) {
	m := New()
	m.RecordPublishCycle(1500*time.Millisecond, nil)
	m.RecordPublishCycle(200*time.Millisecond, errors.New("fail"))
	m.RecordDiscovery(10, 25)
	m.RecordSkip()
	m.SetLeader(true)
	m.Heartbeat()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler().ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("unexpected Content-Type: %s", ct)
	}

	body := rec.Body.String()

	checks := []string{
		"# TYPE crdpublisher_publish_cycle_duration_seconds gauge",
		"crdpublisher_publish_cycle_duration_seconds 0.2",
		"# TYPE crdpublisher_publish_cycle_total counter",
		`crdpublisher_publish_cycle_total{result="success"} 1`,
		`crdpublisher_publish_cycle_total{result="error"} 1`,
		"# TYPE crdpublisher_crds_discovered gauge",
		"crdpublisher_crds_discovered 10",
		"# TYPE crdpublisher_schemas_written gauge",
		"crdpublisher_schemas_written 25",
		"# TYPE crdpublisher_last_successful_publish_timestamp gauge",
		"# TYPE crdpublisher_watchdog_timestamp gauge",
		"# TYPE crdpublisher_publish_skipped_total counter",
		"crdpublisher_publish_skipped_total 1",
		"# TYPE crdpublisher_leader gauge",
		"crdpublisher_leader 1",
	}

	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("missing in output: %q\nbody:\n%s", want, body)
		}
	}

	// Verify timestamp is a recent Unix epoch (set by the successful RecordPublishCycle call)
	if !strings.Contains(body, "crdpublisher_last_successful_publish_timestamp") {
		t.Error("missing last_successful_publish_timestamp in output")
	}
}

func TestHandler_ZeroValues(t *testing.T) {
	m := New()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler().ServeHTTP(rec, req)

	body := rec.Body.String()

	checks := []string{
		"crdpublisher_publish_cycle_duration_seconds 0",
		`crdpublisher_publish_cycle_total{result="success"} 0`,
		`crdpublisher_publish_cycle_total{result="error"} 0`,
		"crdpublisher_crds_discovered 0",
		"crdpublisher_schemas_written 0",
		"crdpublisher_last_successful_publish_timestamp 0",
		"crdpublisher_watchdog_timestamp 0",
		"crdpublisher_publish_skipped_total 0",
		"crdpublisher_leader 0",
	}

	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("missing in output: %q\nbody:\n%s", want, body)
		}
	}
}

func TestConcurrentRecording(t *testing.T) {
	m := New()
	var wg sync.WaitGroup
	const n = 100

	wg.Add(4)
	go func() {
		defer wg.Done()
		for range n {
			m.RecordPublishCycle(time.Millisecond, nil)
		}
	}()
	go func() {
		defer wg.Done()
		for range n {
			m.RecordPublishCycle(time.Millisecond, errors.New("x"))
		}
	}()
	go func() {
		defer wg.Done()
		for range n {
			m.RecordSkip()
		}
	}()
	go func() {
		defer wg.Done()
		for range n {
			m.RecordDiscovery(1, 2)
			m.SetLeader(true)
			m.Heartbeat()
		}
	}()

	wg.Wait()

	if got := m.publishCycleSuccess.Load(); got != n {
		t.Fatalf("expected success=%d, got %d", n, got)
	}
	if got := m.publishCycleError.Load(); got != n {
		t.Fatalf("expected error=%d, got %d", n, got)
	}
	if got := m.publishSkipped.Load(); got != n {
		t.Fatalf("expected skipped=%d, got %d", n, got)
	}
}
