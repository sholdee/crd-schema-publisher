package extractor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"k8s.io/client-go/rest"
)

func TestAPIServerOpenAPISourceFetchesOpenAPIV2(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/v2" {
			t.Fatalf("expected /openapi/v2, got %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(sampleOpenAPI))
	}))
	defer server.Close()

	source, err := NewAPIServerOpenAPISource(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatalf("NewAPIServerOpenAPISource: %v", err)
	}
	raw, err := source.FetchOpenAPIV2(context.Background())
	if err != nil {
		t.Fatalf("FetchOpenAPIV2: %v", err)
	}
	if string(raw) != sampleOpenAPI {
		t.Fatalf("unexpected OpenAPI body")
	}
}

func TestAPIServerOpenAPISourceReportsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	source, err := NewAPIServerOpenAPISource(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatalf("NewAPIServerOpenAPISource: %v", err)
	}
	_, err = source.FetchOpenAPIV2(context.Background())
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestAPIServerOpenAPISourcePreservesHostBasePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/openapi/v2" {
			t.Fatalf("expected /proxy/openapi/v2, got %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(sampleOpenAPI))
	}))
	defer server.Close()

	source, err := NewAPIServerOpenAPISource(&rest.Config{Host: server.URL + "/proxy"})
	if err != nil {
		t.Fatalf("NewAPIServerOpenAPISource: %v", err)
	}
	if _, err := source.FetchOpenAPIV2(context.Background()); err != nil {
		t.Fatalf("FetchOpenAPIV2: %v", err)
	}
}

// etagServer serves /openapi/v2 with an ETag and honors If-None-Match the way
// the Kubernetes aggregator does, and lets the test swap the document.
type etagServer struct {
	mu       sync.Mutex
	body     string
	etag     string
	requests int
}

func (s *etagServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.requests++
		if r.URL.Path != "/openapi/v2" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if s.etag != "" {
			w.Header().Set("Etag", s.etag)
			if r.Header.Get("If-None-Match") == s.etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		_, _ = w.Write([]byte(s.body))
	}
}

func (s *etagServer) set(body, etag string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.body, s.etag = body, etag
}

func (s *etagServer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests
}

func TestAPIServerOpenAPISourceChangeDetectionUsesETag(t *testing.T) {
	backend := &etagServer{body: sampleOpenAPI, etag: `"v1"`}
	server := httptest.NewServer(backend.handler(t))
	defer server.Close()

	source, err := NewAPIServerOpenAPISource(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Nothing fetched yet: nothing to compare, and no request is made.
	changed, err := source.OpenAPIV2Changed(ctx)
	if err != nil || changed {
		t.Fatalf("before any fetch expected (false, nil), got (%v, %v)", changed, err)
	}
	if backend.count() != 0 {
		t.Fatalf("expected no request before the first fetch, got %d", backend.count())
	}

	if _, err := source.FetchOpenAPIV2(ctx); err != nil {
		t.Fatal(err)
	}

	// Same document: the server answers 304 and the check reports no change.
	changed, err = source.OpenAPIV2Changed(ctx)
	if err != nil || changed {
		t.Fatalf("unchanged document: expected (false, nil), got (%v, %v)", changed, err)
	}

	// New document with a new ETag: the check reports a change but does not
	// advance the remembered tag; only a fetch does.
	backend.set(`{"definitions": {}}`, `"v2"`)
	for i := range 2 {
		changed, err = source.OpenAPIV2Changed(ctx)
		if err != nil || !changed {
			t.Fatalf("changed document check %d: expected (true, nil), got (%v, %v)", i, changed, err)
		}
	}
	if _, err := source.FetchOpenAPIV2(ctx); err != nil {
		t.Fatal(err)
	}
	changed, err = source.OpenAPIV2Changed(ctx)
	if err != nil || changed {
		t.Fatalf("after refetch expected (false, nil), got (%v, %v)", changed, err)
	}
}

func TestAPIServerOpenAPISourceChangeDetectionFallsBackToBodyHash(t *testing.T) {
	backend := &etagServer{body: sampleOpenAPI}
	server := httptest.NewServer(backend.handler(t))
	defer server.Close()

	source, err := NewAPIServerOpenAPISource(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := source.FetchOpenAPIV2(ctx); err != nil {
		t.Fatal(err)
	}

	changed, err := source.OpenAPIV2Changed(ctx)
	if err != nil || changed {
		t.Fatalf("identical body without ETag: expected (false, nil), got (%v, %v)", changed, err)
	}

	backend.set(`{"definitions": {}}`, "")
	changed, err = source.OpenAPIV2Changed(ctx)
	if err != nil || !changed {
		t.Fatalf("different body without ETag: expected (true, nil), got (%v, %v)", changed, err)
	}
}

func TestAPIServerOpenAPISourceChangeDetectionReportsHTTPError(t *testing.T) {
	backend := &etagServer{body: sampleOpenAPI, etag: `"v1"`}
	server := httptest.NewServer(backend.handler(t))
	defer server.Close()

	source, err := NewAPIServerOpenAPISource(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := source.FetchOpenAPIV2(ctx); err != nil {
		t.Fatal(err)
	}

	server.Close()
	if _, err := source.OpenAPIV2Changed(ctx); err == nil {
		t.Fatal("expected an error when the API server is unreachable")
	}
}
