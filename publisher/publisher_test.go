package publisher

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fullFlowHandler(t *testing.T, calls *[]string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/projects/test-project"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": map[string]interface{}{"name": "test-project"}})
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/upload-token"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": map[string]interface{}{"jwt": "fake-jwt-token"}})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/check-missing"):
			var body struct {
				Hashes []string `json:"hashes"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": body.Hashes})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/upload"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": nil})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/upsert-hashes"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": nil})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/deployments"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": map[string]interface{}{"id": "deploy-123", "url": "https://test-project.pages.dev"}})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	})
}

func TestPublish_FullFlow(t *testing.T) {
	var calls []string
	server := httptest.NewServer(fullFlowHandler(t, &calls))
	defer server.Close()

	tmpDir := t.TempDir()
	genDir := filepath.Join(tmpDir, ".generations", "gen1")
	_ = os.MkdirAll(filepath.Join(genDir, "example.io"), 0o755)
	_ = os.WriteFile(filepath.Join(genDir, "example.io", "test_v1.json"), []byte(`{"type":"object"}`), 0o644)
	_ = os.WriteFile(filepath.Join(genDir, "index.html"), []byte(`<html></html>`), 0o644)
	_ = os.Symlink(filepath.Join(".generations", "gen1"), filepath.Join(tmpDir, "current"))

	p := &Publisher{
		APIToken: "fake-token", AccountID: "fake-account", ProjectName: "test-project",
		BaseURL: server.URL + "/client/v4", AssetsURL: server.URL + "/client/v4",
	}
	err := p.Publish(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedCalls := []string{"GET", "upload-token", "check-missing", "upload", "upsert-hashes", "deployments"}
	callStr := fmt.Sprintf("%v", calls)
	for _, expected := range expectedCalls {
		if !strings.Contains(callStr, expected) {
			t.Errorf("expected call containing %q, calls were: %v", expected, calls)
		}
	}
}

func TestPublish_CreatesProjectIfMissing(t *testing.T) {
	projectCreated := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/projects/new-project"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "errors": []interface{}{map[string]interface{}{"code": 8000007}}})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/projects") && !strings.Contains(r.URL.Path, "deployments"):
			projectCreated = true
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": map[string]interface{}{"name": "new-project"}})
		case strings.HasSuffix(r.URL.Path, "/upload-token"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": map[string]interface{}{"jwt": "fake-jwt"}})
		case strings.HasSuffix(r.URL.Path, "/check-missing"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": []string{}})
		case strings.HasSuffix(r.URL.Path, "/upsert-hashes"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": nil})
		case strings.HasSuffix(r.URL.Path, "/deployments"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": map[string]interface{}{"id": "d1", "url": "https://new-project.pages.dev"}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": nil})
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	genDir := filepath.Join(tmpDir, ".generations", "gen1")
	_ = os.MkdirAll(filepath.Join(genDir, "example.io"), 0o755)
	_ = os.WriteFile(filepath.Join(genDir, "example.io", "test_v1.json"), []byte(`{}`), 0o644)
	_ = os.Symlink(filepath.Join(".generations", "gen1"), filepath.Join(tmpDir, "current"))

	p := &Publisher{
		APIToken: "fake-token", AccountID: "fake-account", ProjectName: "new-project",
		BaseURL: server.URL + "/client/v4", AssetsURL: server.URL + "/client/v4",
	}
	err := p.Publish(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !projectCreated {
		t.Fatal("expected project to be created")
	}
}

func tempFileEntry(t *testing.T) *fileEntry {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	_ = os.WriteFile(path, []byte(`{"test":true}`), 0o644)
	return &fileEntry{
		relPath: "test.json", absPath: path,
		hash: "abc123", size: 13, contentType: "application/json",
	}
}

func TestUploadBucket_RetryOn5xx(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "errors": []interface{}{map[string]interface{}{"message": "bad gateway"}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	}))
	defer server.Close()

	p := &Publisher{
		APIToken: "fake-token", AccountID: "fake-account", ProjectName: "test-project",
		AssetsURL: server.URL,
		SleepFunc: func(time.Duration) {},
	}
	err := p.uploadBucket("fake-jwt", []*fileEntry{tempFileEntry(t)})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got := count.Load(); got != 3 {
		t.Fatalf("expected 3 requests, got %d", got)
	}
}

func TestUploadBucket_NoRetryOn4xx(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "errors": []interface{}{map[string]interface{}{"message": "bad request"}}})
	}))
	defer server.Close()

	p := &Publisher{
		APIToken: "fake-token", AccountID: "fake-account", ProjectName: "test-project",
		AssetsURL: server.URL,
		SleepFunc: func(time.Duration) {},
	}
	err := p.uploadBucket("fake-jwt", []*fileEntry{tempFileEntry(t)})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := count.Load(); got != 1 {
		t.Fatalf("expected 1 request (no retries), got %d", got)
	}
}

func TestUploadBucket_MaxRetriesExceeded(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "errors": []interface{}{map[string]interface{}{"message": "bad gateway"}}})
	}))
	defer server.Close()

	p := &Publisher{
		APIToken: "fake-token", AccountID: "fake-account", ProjectName: "test-project",
		AssetsURL: server.URL,
		SleepFunc: func(time.Duration) {},
	}
	err := p.uploadBucket("fake-jwt", []*fileEntry{tempFileEntry(t)})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "after 5 retries") {
		t.Fatalf("expected error to contain 'after 5 retries', got: %v", err)
	}
	if got := count.Load(); got != 5 {
		t.Fatalf("expected 5 requests, got %d", got)
	}
}

func TestCreateDeployment_RetryOn5xx(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "errors": []interface{}{map[string]interface{}{"message": "bad gateway"}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": map[string]interface{}{"url": "https://test.pages.dev"}})
	}))
	defer server.Close()

	p := &Publisher{
		APIToken: "fake-token", AccountID: "fake-account", ProjectName: "test-project",
		BaseURL:   server.URL,
		SleepFunc: func(time.Duration) {},
	}
	url, err := p.createDeployment(map[string]string{"/index.html": "abc123"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if url != "https://test.pages.dev" {
		t.Fatalf("expected URL https://test.pages.dev, got %s", url)
	}
	if got := count.Load(); got != 3 {
		t.Fatalf("expected 3 requests, got %d", got)
	}
}

func TestCreateDeployment_MaxRetriesExceeded(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "errors": []interface{}{map[string]interface{}{"message": "bad gateway"}}})
	}))
	defer server.Close()

	p := &Publisher{
		APIToken: "fake-token", AccountID: "fake-account", ProjectName: "test-project",
		BaseURL:   server.URL,
		SleepFunc: func(time.Duration) {},
	}
	_, err := p.createDeployment(map[string]string{"/index.html": "abc123"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "after 3 retries") {
		t.Fatalf("expected error to contain 'after 3 retries', got: %v", err)
	}
	if got := count.Load(); got != 3 {
		t.Fatalf("expected 3 requests, got %d", got)
	}
}

func TestPublish_MalformedUploadToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/projects/test-project"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": map[string]interface{}{"name": "test-project"}})
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/upload-token"):
			_, _ = w.Write([]byte("not json at all"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	genDir := filepath.Join(tmpDir, ".generations", "gen1")
	_ = os.MkdirAll(genDir, 0o755)
	_ = os.WriteFile(filepath.Join(genDir, "test.json"), []byte(`{}`), 0o644)
	_ = os.Symlink(filepath.Join(".generations", "gen1"), filepath.Join(tmpDir, "current"))

	p := &Publisher{
		APIToken: "fake-token", AccountID: "fake-account", ProjectName: "test-project",
		BaseURL: server.URL, AssetsURL: server.URL,
		SleepFunc: func(time.Duration) {},
	}
	err := p.Publish(tmpDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "decoding response") {
		t.Fatalf("expected error to contain 'decoding response', got: %v", err)
	}
}

func newManifestCaptureServer(t *testing.T, manifest *map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/projects/test-project"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": map[string]interface{}{"name": "test-project"}})
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/upload-token"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": map[string]interface{}{"jwt": "fake-jwt-token"}})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/check-missing"):
			var body struct {
				Hashes []string `json:"hashes"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": body.Hashes})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/upload"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": nil})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/upsert-hashes"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": nil})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/deployments"):
			decodeManifest(t, r, manifest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": map[string]interface{}{"url": "https://test.pages.dev"}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
}

func decodeManifest(t *testing.T, r *http.Request, manifest *map[string]string) {
	t.Helper()
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		t.Fatalf("ParseMultipartForm: %v", err)
	}
	if err := json.Unmarshal([]byte(r.FormValue("manifest")), manifest); err != nil {
		t.Fatalf("manifest unmarshal: %v", err)
	}
}

func seedCurrentGeneration(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	generationDir := filepath.Join(tmpDir, ".generations", "gen1")
	if err := os.MkdirAll(filepath.Join(generationDir, "example.io"), 0o755); err != nil {
		t.Fatalf("mkdir generation: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(generationDir, "_meta"), 0o755); err != nil {
		t.Fatalf("mkdir metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(generationDir, "index.html"), []byte(`<html></html>`), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(generationDir, "example.io", "test_v1.json"), []byte(`{"type":"object"}`), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	if err := os.WriteFile(filepath.Join(generationDir, "_meta", "kinds.json"), []byte(`{"example.io/test_v1.json":"Test"}`), 0o644); err != nil {
		t.Fatalf("write metadata manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "flat-root.json"), []byte(`{"wrong":"path"}`), 0o644); err != nil {
		t.Fatalf("write flat root file: %v", err)
	}
	if err := os.Symlink(filepath.Join(".generations", "gen1"), filepath.Join(tmpDir, "current")); err != nil {
		t.Fatalf("create current symlink: %v", err)
	}
	return tmpDir
}

func assertCurrentManifest(t *testing.T, manifest map[string]string) {
	t.Helper()
	if _, ok := manifest["/index.html"]; !ok {
		t.Fatal("expected index.html from current generation in manifest")
	}
	if _, ok := manifest["/example.io/test_v1.json"]; !ok {
		t.Fatal("expected schema file from current generation in manifest")
	}
	if _, ok := manifest["/_meta/kinds.json"]; ok {
		t.Fatal("did not expect metadata manifest in manifest")
	}
	if _, ok := manifest["/flat-root.json"]; ok {
		t.Fatal("did not expect flat root files outside current generation in manifest")
	}
}

func TestPublish_UsesCurrentAndSkipsKindFiles(t *testing.T) {
	var manifest map[string]string
	server := newManifestCaptureServer(t, &manifest)
	defer server.Close()

	tmpDir := seedCurrentGeneration(t)

	p := &Publisher{
		APIToken: "fake-token", AccountID: "fake-account", ProjectName: "test-project",
		BaseURL: server.URL + "/client/v4", AssetsURL: server.URL + "/client/v4",
	}
	if err := p.Publish(tmpDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertCurrentManifest(t, manifest)
}
