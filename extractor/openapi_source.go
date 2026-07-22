package extractor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"k8s.io/client-go/rest"
)

type OpenAPISource interface {
	FetchOpenAPIV2(ctx context.Context) ([]byte, error)
}

type APIServerOpenAPISource struct {
	client  *http.Client
	baseURL *url.URL
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
	endpoint := s.baseURL.JoinPath("openapi", "v2")
	endpoint.RawQuery = ""
	endpoint.Fragment = ""

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating OpenAPI request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

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
	return raw, nil
}
