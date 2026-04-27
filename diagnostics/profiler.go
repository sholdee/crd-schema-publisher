package diagnostics

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
)

const profileDirEnv = "PROFILE_DIR"

// Snapshotter records a named diagnostic point.
type Snapshotter interface {
	Snapshot(phase string, attrs ...any)
}

type preservePathProvider interface {
	PreservePaths() []string
}

func PreservePaths(snapshotter Snapshotter) []string {
	provider, ok := snapshotter.(preservePathProvider)
	if !ok {
		return nil
	}
	return provider.PreservePaths()
}

// Profiler writes opt-in heap profiles and memory stats for short-lived jobs.
type Profiler struct {
	dir              string
	mu               sync.Mutex
	sequence         int
	readMemStats     func(*runtime.MemStats)
	writeHeapProfile func(io.Writer) error
}

func NewFromEnv() *Profiler {
	return NewProfiler(os.Getenv(profileDirEnv))
}

func NewProfiler(dir string) *Profiler {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return &Profiler{}
	}
	return &Profiler{
		dir:              dir,
		readMemStats:     runtime.ReadMemStats,
		writeHeapProfile: pprof.WriteHeapProfile,
	}
}

func (p *Profiler) Enabled() bool {
	return p != nil && p.dir != ""
}

func (p *Profiler) PreservePaths() []string {
	if !p.Enabled() {
		return nil
	}
	return []string{p.dir}
}

func (p *Profiler) Snapshot(phase string, attrs ...any) {
	if !p.Enabled() {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.sequence++
	stats := p.memStats()
	logAttrs := []any{
		"phase", phase,
		"alloc_bytes", stats.Alloc,
		"total_alloc_bytes", stats.TotalAlloc,
		"heap_alloc_bytes", stats.HeapAlloc,
		"heap_inuse_bytes", stats.HeapInuse,
		"heap_sys_bytes", stats.HeapSys,
		"sys_bytes", stats.Sys,
		"next_gc_bytes", stats.NextGC,
		"num_gc", stats.NumGC,
		"goroutines", runtime.NumGoroutine(),
		"profile_dir", p.dir,
	}
	logAttrs = append(logAttrs, attrs...)

	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		slog.Warn("memory profile snapshot failed", append(logAttrs, "error", err)...)
		return
	}

	profilePath := filepath.Join(p.dir, fmt.Sprintf("%03d-heap-%s.pprof", p.sequence, sanitizePhase(phase)))
	if err := p.writeHeap(profilePath); err != nil {
		slog.Warn("memory profile snapshot failed", append(logAttrs, "profile_path", profilePath, "error", err)...)
		return
	}

	slog.Info("memory profile snapshot", append(logAttrs, "profile_path", profilePath)...)
}

func (p *Profiler) memStats() runtime.MemStats {
	var stats runtime.MemStats
	read := p.readMemStats
	if read == nil {
		read = runtime.ReadMemStats
	}
	read(&stats)
	return stats
}

func (p *Profiler) writeHeap(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	write := p.writeHeapProfile
	if write == nil {
		write = pprof.WriteHeapProfile
	}
	writeErr := write(f)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func sanitizePhase(phase string) string {
	phase = strings.ToLower(strings.TrimSpace(phase))
	var b strings.Builder
	lastDash := false
	for _, r := range phase {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	safe := strings.Trim(b.String(), "-")
	if safe == "" {
		return "snapshot"
	}
	return safe
}
