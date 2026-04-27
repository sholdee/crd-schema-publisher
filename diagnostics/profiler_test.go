package diagnostics

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestProfilerSnapshotWritesSanitizedHeapProfile(t *testing.T) {
	dir := t.TempDir()
	p := NewProfiler(dir)
	p.writeHeapProfile = func(w io.Writer) error {
		_, err := w.Write([]byte("heap profile"))
		return err
	}
	p.readMemStats = func(stats *runtime.MemStats) {
		stats.Alloc = 1024
		stats.TotalAlloc = 2048
		stats.HeapAlloc = 1024
		stats.HeapInuse = 4096
		stats.HeapSys = 8192
		stats.Sys = 16384
		stats.NextGC = 32768
		stats.NumGC = 2
	}

	p.Snapshot("upload/bucket 1: after marshal", "files", 3)

	profilePath := filepath.Join(dir, "001-heap-upload-bucket-1-after-marshal.pprof")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("expected heap profile at %s: %v", profilePath, err)
	}
	if string(data) != "heap profile" {
		t.Fatalf("unexpected heap profile contents %q", string(data))
	}
}

func TestProfilerDisabledWhenDirectoryEmpty(t *testing.T) {
	p := NewProfiler("")
	if p.Enabled() {
		t.Fatal("expected empty profile dir to disable profiler")
	}

	p.Snapshot("ignored")
}
