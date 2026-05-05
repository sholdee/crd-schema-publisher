package watcher

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sholdee/crd-schema-publisher/metrics"
)

func signalTrigger(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
		// Channel already has a pending signal
	}
}

// drainTimeout bounds how long we wait for an in-flight publish to finish
// during shutdown. Must be less than terminationGracePeriodSeconds (default 30s)
// to leave time for health server shutdown and process cleanup.
const drainTimeout = 25 * time.Second

type debounceClock interface {
	NewTimer(time.Duration) debounceTimer
	NewTicker(time.Duration) debounceTicker
	After(time.Duration) <-chan time.Time
}

type debounceTimer interface {
	Stop() bool
	Reset(time.Duration) bool
	C() <-chan time.Time
}

type debounceTicker interface {
	Stop()
	C() <-chan time.Time
}

type realDebounceClock struct{}

func (realDebounceClock) NewTimer(d time.Duration) debounceTimer {
	return realDebounceTimer{timer: time.NewTimer(d)}
}

func (realDebounceClock) NewTicker(d time.Duration) debounceTicker {
	return realDebounceTicker{ticker: time.NewTicker(d)}
}

func (realDebounceClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

type realDebounceTimer struct {
	timer *time.Timer
}

func (t realDebounceTimer) Stop() bool {
	return t.timer.Stop()
}

func (t realDebounceTimer) Reset(d time.Duration) bool {
	return t.timer.Reset(d)
}

func (t realDebounceTimer) C() <-chan time.Time {
	return t.timer.C
}

type realDebounceTicker struct {
	ticker *time.Ticker
}

func (t realDebounceTicker) Stop() {
	t.ticker.Stop()
}

func (t realDebounceTicker) C() <-chan time.Time {
	return t.ticker.C
}

func debounceLoop(trigger <-chan struct{}, duration time.Duration, publish func() error, m *metrics.Metrics, done <-chan struct{}) {
	debounceLoopWithClock(trigger, duration, publish, m, done, realDebounceClock{})
}

func debounceLoopWithClock(trigger <-chan struct{}, duration time.Duration, publish func() error, m *metrics.Metrics, done <-chan struct{}, clock debounceClock) {
	var timer debounceTimer
	var timerC <-chan time.Time
	var publishing atomic.Bool
	var wg sync.WaitGroup
	first := true
	if clock == nil {
		clock = realDebounceClock{}
	}
	heartbeat := clock.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	heartbeatC := heartbeat.C()
	m.Heartbeat() // initial heartbeat on loop entry

	for {
		select {
		case <-done:
			if timer != nil {
				timer.Stop()
			}
			// Wait for in-flight publish to complete, bounded by drainTimeout
			drained := make(chan struct{})
			go func() {
				wg.Wait()
				close(drained)
			}()
			select {
			case <-drained:
			case <-clock.After(drainTimeout):
				slog.Warn("drain timeout exceeded, abandoning in-flight publish")
			}
			return
		case <-trigger:
			m.Heartbeat()
			d := duration
			if first {
				d = 0
				first = false
			}
			if timer == nil {
				timer = clock.NewTimer(d)
				timerC = timer.C()
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C():
					default:
					}
				}
				timer.Reset(d)
			}
		case <-heartbeatC:
			m.Heartbeat()
		case <-timerC:
			m.Heartbeat()
			timer = nil
			timerC = nil
			startPublishIfIdle(&publishing, &wg, publish, m)
		}
	}
}

func startPublishIfIdle(publishing *atomic.Bool, wg *sync.WaitGroup, publish func() error, m *metrics.Metrics) {
	if !publishing.CompareAndSwap(false, true) {
		slog.Warn("publish already in progress, skipping")
		m.RecordSkip()
		return
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer publishing.Store(false)
		slog.Info("running publish cycle")
		if err := publish(); err != nil {
			slog.Error("publish cycle failed", "error", err)
		}
	}()
}
