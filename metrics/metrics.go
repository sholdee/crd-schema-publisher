package metrics

import (
	"fmt"
	"math"
	"net/http"
	"sync/atomic"
	"time"
)

// Metrics holds Prometheus-compatible counters and gauges for the watcher.
// All fields use atomic operations for lock-free concurrent access.
// A nil *Metrics is safe to use — all recording methods are no-ops on nil receiver.
type Metrics struct {
	publishCycleSuccess atomic.Int64
	publishCycleError   atomic.Int64
	publishSkipped      atomic.Int64
	durationSeconds     atomic.Int64 // float64 bits via math.Float64bits
	crdsDiscovered      atomic.Int64
	schemasWritten      atomic.Int64
	lastSuccessTime     atomic.Int64 // Unix epoch seconds
	watchdogTime        atomic.Int64 // Unix epoch seconds, updated every debounce tick
	leader              atomic.Int64 // 0 or 1

	builtinsRefreshUnchanged atomic.Int64
	builtinsRefreshChanged   atomic.Int64
	builtinsRefreshError     atomic.Int64
}

// New creates a zeroed Metrics instance.
func New() *Metrics {
	return &Metrics{}
}

// RecordPublishCycle records the outcome of a publish cycle.
func (m *Metrics) RecordPublishCycle(duration time.Duration, err error) {
	if m == nil {
		return
	}
	m.durationSeconds.Store(int64(math.Float64bits(duration.Seconds())))
	if err != nil {
		m.publishCycleError.Add(1)
	} else {
		m.publishCycleSuccess.Add(1)
		m.lastSuccessTime.Store(time.Now().Unix())
	}
}

// RecordDiscovery records CRD and schema counts from the latest cycle.
func (m *Metrics) RecordDiscovery(crds, schemas int) {
	if m == nil {
		return
	}
	m.crdsDiscovered.Store(int64(crds))
	m.schemasWritten.Store(int64(schemas))
}

// RecordSkip records a debounce skip (publish already in progress).
func (m *Metrics) RecordSkip() {
	if m == nil {
		return
	}
	m.publishSkipped.Add(1)
}

// Heartbeat updates the watchdog timestamp to the current time.
// Call on every debounce loop iteration to prove the watcher is alive.
func (m *Metrics) Heartbeat() {
	if m == nil {
		return
	}
	m.watchdogTime.Store(time.Now().Unix())
}

// RecordBuiltinsRefresh records the outcome of one built-in schema refresh
// check against the API server's OpenAPI document.
func (m *Metrics) RecordBuiltinsRefresh(changed bool, err error) {
	if m == nil {
		return
	}
	switch {
	case err != nil:
		m.builtinsRefreshError.Add(1)
	case changed:
		m.builtinsRefreshChanged.Add(1)
	default:
		m.builtinsRefreshUnchanged.Add(1)
	}
}

// SetLeader sets the leader gauge to 1 (true) or 0 (false).
func (m *Metrics) SetLeader(isLeader bool) {
	if m == nil {
		return
	}
	if isLeader {
		m.leader.Store(1)
	} else {
		m.leader.Store(0)
	}
}

// Handler returns an http.Handler that writes metrics in Prometheus text exposition format.
func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		dur := math.Float64frombits(uint64(m.durationSeconds.Load()))
		success := m.publishCycleSuccess.Load()
		errCount := m.publishCycleError.Load()
		skipped := m.publishSkipped.Load()
		crds := m.crdsDiscovered.Load()
		schemas := m.schemasWritten.Load()
		lastSuccess := m.lastSuccessTime.Load()
		watchdog := m.watchdogTime.Load()
		leader := m.leader.Load()
		refreshUnchanged := m.builtinsRefreshUnchanged.Load()
		refreshChanged := m.builtinsRefreshChanged.Load()
		refreshError := m.builtinsRefreshError.Load()

		_, _ = fmt.Fprintf(w, "# HELP crdpublisher_publish_cycle_duration_seconds Duration of the most recent publish cycle.\n")
		_, _ = fmt.Fprintf(w, "# TYPE crdpublisher_publish_cycle_duration_seconds gauge\n")
		_, _ = fmt.Fprintf(w, "crdpublisher_publish_cycle_duration_seconds %g\n", dur)

		_, _ = fmt.Fprintf(w, "# HELP crdpublisher_publish_cycle_total Total number of publish cycles.\n")
		_, _ = fmt.Fprintf(w, "# TYPE crdpublisher_publish_cycle_total counter\n")
		_, _ = fmt.Fprintf(w, "crdpublisher_publish_cycle_total{result=\"success\"} %d\n", success)
		_, _ = fmt.Fprintf(w, "crdpublisher_publish_cycle_total{result=\"error\"} %d\n", errCount)

		_, _ = fmt.Fprintf(w, "# HELP crdpublisher_crds_discovered Number of CRDs found in the most recent publish cycle.\n")
		_, _ = fmt.Fprintf(w, "# TYPE crdpublisher_crds_discovered gauge\n")
		_, _ = fmt.Fprintf(w, "crdpublisher_crds_discovered %d\n", crds)

		_, _ = fmt.Fprintf(w, "# HELP crdpublisher_schemas_written Number of schemas written in the most recent publish cycle.\n")
		_, _ = fmt.Fprintf(w, "# TYPE crdpublisher_schemas_written gauge\n")
		_, _ = fmt.Fprintf(w, "crdpublisher_schemas_written %d\n", schemas)

		_, _ = fmt.Fprintf(w, "# HELP crdpublisher_last_successful_publish_timestamp Unix timestamp of the last successful publish cycle.\n")
		_, _ = fmt.Fprintf(w, "# TYPE crdpublisher_last_successful_publish_timestamp gauge\n")
		_, _ = fmt.Fprintf(w, "crdpublisher_last_successful_publish_timestamp %d\n", lastSuccess)

		_, _ = fmt.Fprintf(w, "# HELP crdpublisher_watchdog_timestamp Unix timestamp of the last debounce loop tick.\n")
		_, _ = fmt.Fprintf(w, "# TYPE crdpublisher_watchdog_timestamp gauge\n")
		_, _ = fmt.Fprintf(w, "crdpublisher_watchdog_timestamp %d\n", watchdog)

		_, _ = fmt.Fprintf(w, "# HELP crdpublisher_publish_skipped_total Total debounce skips due to publish already in progress.\n")
		_, _ = fmt.Fprintf(w, "# TYPE crdpublisher_publish_skipped_total counter\n")
		_, _ = fmt.Fprintf(w, "crdpublisher_publish_skipped_total %d\n", skipped)

		_, _ = fmt.Fprintf(w, "# HELP crdpublisher_leader Whether this pod is the current leader.\n")
		_, _ = fmt.Fprintf(w, "# TYPE crdpublisher_leader gauge\n")
		_, _ = fmt.Fprintf(w, "crdpublisher_leader %d\n", leader)

		_, _ = fmt.Fprintf(w, "# HELP crdpublisher_builtins_refresh_total Total built-in schema refresh checks against the API server OpenAPI document, by outcome.\n")
		_, _ = fmt.Fprintf(w, "# TYPE crdpublisher_builtins_refresh_total counter\n")
		_, _ = fmt.Fprintf(w, "crdpublisher_builtins_refresh_total{result=\"unchanged\"} %d\n", refreshUnchanged)
		_, _ = fmt.Fprintf(w, "crdpublisher_builtins_refresh_total{result=\"changed\"} %d\n", refreshChanged)
		_, _ = fmt.Fprintf(w, "crdpublisher_builtins_refresh_total{result=\"error\"} %d\n", refreshError)
	})
}
