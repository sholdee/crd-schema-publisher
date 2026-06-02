package extractor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
