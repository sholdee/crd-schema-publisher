package extractor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"k8s.io/client-go/rest"
)

type OpenAPISource interface {
	FetchOpenAPIV2(ctx context.Context) ([]byte, error)
}

// OpenAPIChangeDetector reports whether the API server's OpenAPI v2 document
// differs from the one most recently fetched through FetchOpenAPIV2. Watch
// mode polls it so built-in schemas are republished after a control-plane
// upgrade even when no CRD event fires.
type OpenAPIChangeDetector interface {
	OpenAPIV2Changed(ctx context.Context) (bool, error)
}

type APIServerOpenAPISource struct {
	client  *http.Client
	baseURL *url.URL

	mu sync.Mutex
	// lastTag identifies the document returned by the most recent successful
	// FetchOpenAPIV2: the server's ETag when it sends one, else a hash of the
	// body. Empty until the first fetch.
	lastTag string
}

func NewAPIServerOpenAPISource(cfg *rest.Config) (*APIServerOpenAPISource, error) {
	if cfg == nil {
		return nil, fmt.Errorf("kubernetes rest config is required")
	}
	client, err := rest.HTTPClientFor(rest.CopyConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("building OpenAPI HTTP client: %w", err)
	}
	baseURL, err := url.Parse(cfg.Host)
	if err != nil {
		return nil, fmt.Errorf("parsing Kubernetes API server URL: %w", err)
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("kubernetes API server URL %q must include scheme and host", cfg.Host)
	}
	return &APIServerOpenAPISource{client: client, baseURL: baseURL}, nil
}

func (s *APIServerOpenAPISource) FetchOpenAPIV2(ctx context.Context) ([]byte, error) {
	req, err := s.newOpenAPIRequest(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching /openapi/v2: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("fetching /openapi/v2: status %d: %s", resp.StatusCode, string(body))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading /openapi/v2 response: %w", err)
	}

	s.mu.Lock()
	s.lastTag = documentTag(resp.Header.Get("Etag"), raw)
	s.mu.Unlock()
	return raw, nil
}

// OpenAPIV2Changed sends a conditional GET for /openapi/v2 carrying the tag of
// the last fetched document. The aggregator serves the endpoint with an ETag
// and honors If-None-Match, so an unchanged document costs a 304 with no body.
// Servers that ignore the precondition still work: the body is hashed and
// compared. Before any fetch there is nothing to compare, so it reports no
// change.
func (s *APIServerOpenAPISource) OpenAPIV2Changed(ctx context.Context) (bool, error) {
	s.mu.Lock()
	lastTag := s.lastTag
	s.mu.Unlock()
	if lastTag == "" {
		return false, nil
	}

	req, err := s.newOpenAPIRequest(ctx)
	if err != nil {
		return false, err
	}
	req.Header.Set("If-None-Match", lastTag)

	resp, err := s.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("checking /openapi/v2: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return false, fmt.Errorf("checking /openapi/v2: status %d: %s", resp.StatusCode, string(body))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("reading /openapi/v2 response: %w", err)
	}
	return documentTag(resp.Header.Get("Etag"), raw) != lastTag, nil
}

func (s *APIServerOpenAPISource) newOpenAPIRequest(ctx context.Context) (*http.Request, error) {
	endpoint := s.baseURL.JoinPath("openapi", "v2")
	endpoint.RawQuery = ""
	endpoint.Fragment = ""

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating OpenAPI request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// documentTag prefers the server's ETag, which lets change checks use
// If-None-Match, and falls back to a content hash so servers without ETags
// are still compared by content rather than treated as always changed.
func documentTag(etag string, body []byte) string {
	if etag != "" {
		return etag
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
