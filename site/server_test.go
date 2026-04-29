package site

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func writeSiteFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestStaticHandlerServesFilesAndHidesMeta(t *testing.T) {
	dir := t.TempDir()
	writeSiteFile(t, dir, "index.html", "index")
	writeSiteFile(t, dir, "_meta/kinds.json", "{}")

	handler := NewStaticHandler(dir, "")

	ok := httptest.NewRecorder()
	handler.ServeHTTP(ok, httptest.NewRequest(http.MethodGet, "/", nil))
	if ok.Code != http.StatusOK {
		t.Fatalf("expected / status 200, got %d", ok.Code)
	}
	if got := ok.Body.String(); got != "index" {
		t.Fatalf("expected index body, got %q", got)
	}

	hidden := httptest.NewRecorder()
	handler.ServeHTTP(hidden, httptest.NewRequest(http.MethodGet, "/_meta/kinds.json", nil))
	if hidden.Code != http.StatusNotFound {
		t.Fatalf("expected /_meta/kinds.json status 404, got %d", hidden.Code)
	}
}

func TestStaticHandlerBasePathRedirectsRootAndServesPrefixedFiles(t *testing.T) {
	dir := t.TempDir()
	writeSiteFile(t, dir, "index.html", "index")

	handler := NewStaticHandler(dir, "/iac")

	redirect := httptest.NewRecorder()
	handler.ServeHTTP(redirect, httptest.NewRequest(http.MethodGet, "/", nil))
	if redirect.Code != http.StatusFound {
		t.Fatalf("expected root redirect status 302, got %d", redirect.Code)
	}
	if got := redirect.Header().Get("Location"); got != "/iac/" {
		t.Fatalf("expected redirect to /iac/, got %q", got)
	}

	ok := httptest.NewRecorder()
	handler.ServeHTTP(ok, httptest.NewRequest(http.MethodGet, "/iac/", nil))
	if ok.Code != http.StatusOK {
		t.Fatalf("expected /iac/ status 200, got %d", ok.Code)
	}

	hidden := httptest.NewRecorder()
	handler.ServeHTTP(hidden, httptest.NewRequest(http.MethodGet, "/iac/_meta/kinds.json", nil))
	if hidden.Code != http.StatusNotFound {
		t.Fatalf("expected /iac/_meta/kinds.json status 404, got %d", hidden.Code)
	}
}

func TestAccessLogHandlerRecordsRequestMetadata(t *testing.T) {
	var logs bytes.Buffer
	orig := slog.Default()
	defer slog.SetDefault(orig)
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))

	handler := WithAccessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("body"))
	}))
	req := httptest.NewRequest(http.MethodGet, "/schemas/cert_v1.json?ignored=true", nil)
	req.RemoteAddr = "192.0.2.10:54321"
	req.Header.Set("User-Agent", "schema-client/1.0")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatalf("decode access log: %v", err)
	}
	if entry["msg"] != "site request" {
		t.Fatalf("expected site request log message, got %#v", entry["msg"])
	}
	if entry["method"] != http.MethodGet {
		t.Fatalf("expected method GET, got %#v", entry["method"])
	}
	if entry["path"] != "/schemas/cert_v1.json" {
		t.Fatalf("expected request path, got %#v", entry["path"])
	}
	if entry["status"] != float64(http.StatusTeapot) {
		t.Fatalf("expected status 418, got %#v", entry["status"])
	}
	if entry["bytes"] != float64(4) {
		t.Fatalf("expected 4 response bytes, got %#v", entry["bytes"])
	}
	if entry["remote_addr"] != "192.0.2.10:54321" {
		t.Fatalf("expected remote addr, got %#v", entry["remote_addr"])
	}
	if entry["user_agent"] != "schema-client/1.0" {
		t.Fatalf("expected user agent, got %#v", entry["user_agent"])
	}
	if _, ok := entry["duration"]; !ok {
		t.Fatal("expected duration in access log")
	}
}

func TestStartServerReturnsListenError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	server, err := StartServer(listener.Addr().String(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if err == nil {
		if server != nil {
			_ = server.Close()
		}
		t.Fatal("expected StartServer to return a listen error")
	}
	if server != nil {
		t.Fatalf("expected nil server on listen error, got %#v", server)
	}
}
