# CLAUDE.md - Project Context for crd-schema-publisher

## What This Is

A statically-compiled Go binary that extracts CRD JSON schemas from a Kubernetes cluster and publishes them to a Cloudflare Pages website. Runs as a long-lived Deployment (watch mode) or CronJob in a K3s cluster. Deployed in a nonroot distroless container.

## Repository Layout

```text
cmd/            Entrypoint and subcommand dispatch (run/extract/upload/watch/preview)
converter/      OpenAPI v3 -> JSON Schema transforms (ported from openapi2jsonschema.py)
extractor/      client-go CRD listing, schema extraction, file writing, config builder
index/          HTML index generation (deepspace theme, client-side search, starfield/flare effects)
publisher/      Cloudflare Pages direct upload API client + BLAKE3 hashing
watcher/        CRD informer watch loop, debounce, leader election, health server
```

## Build & Test

```bash
# Run all tests
go test ./...

# Vet
go vet ./...

# Cross-compile (matches CI targets)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o crd-schema-publisher ./cmd/
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o crd-schema-publisher ./cmd/

# Local extract (requires kubeconfig)
KUBECTL_CONTEXT=my-context OUTPUT_DIR=./output go run ./cmd/ extract

# Preview index UI locally (no cluster needed)
go run ./cmd/ preview
```

## Architecture

### Subcommands

- `run` (default) — extract + upload
- `extract` — extract CRDs and write JSON schemas + index.html to OUTPUT_DIR
- `upload` — upload OUTPUT_DIR contents to Cloudflare Pages
- `watch` — long-lived process: informer watches CRDs, debounces events, runs extract+publish cycles. Leader election for multi-replica safety.
- `preview` — generate index with sample data (or real schemas via OUTPUT_DIR) and serve on localhost. No cluster or credentials needed. Handles signal cleanup of temp directories.

### Configuration (env vars)

| Var | Required | Default | Purpose |
|-----|----------|---------|---------|
| `CLOUDFLARE_API_TOKEN` | Yes (run/upload) | — | CF API token |
| `CLOUDFLARE_ACCOUNT_ID` | Yes (run/upload) | — | CF account ID |
| `CF_PAGES_PROJECT` | No | `kubernetes-schemas` | CF Pages project name |
| `OUTPUT_DIR` | No | `/output` | Schema output directory |
| `KUBECTL_CONTEXT` | No | — | K8s context (local dev only) |
| `DEBOUNCE_SECONDS` | No | `15` | Seconds to wait after last CRD event before publishing (watch mode) |
| `POD_NAME` | Yes (watch) | — | Pod identity for leader election (set via downward API) |
| `POD_NAMESPACE` | Yes (watch) | — | Namespace for leader lease (set via downward API) |
| `LEASE_NAME` | No | `crd-schema-publisher` | Name of the Lease resource (watch mode) |
| `HEALTH_PORT` | No | `8080` | Port for liveness/readiness probes (watch mode) |
| `PREVIEW_ADDR` | No | `127.0.0.1:8989` | Listen address (preview mode) |

### Key Design Decisions

- **No CGO.** Binary is statically linked. BLAKE3 uses `github.com/zeebo/blake3` (pure Go).
- **Cloudflare Pages direct upload API** is undocumented. Implementation reverse-engineered from wrangler source (`cloudflare/workers-sdk`). The upload flow uses JWT auth for asset operations and API token auth for deployment creation. See `publisher/publisher.go` for the full 6-step flow.
- **BLAKE3 file hashing** exactly matches wrangler's `hashFile`: `hex(blake3(base64(content) + extension))[0:32]`. Do not change this algorithm without verifying against wrangler source.
- **OpenAPI v3 to JSON Schema conversion** is a faithful port of `openapi2jsonschema.py` from datreeio/CRDs-catalog. Three transforms applied in order: additionalProperties, replaceIntOrString, allowNullOptionalFields.
- **Output format** produces two directory structures: `<group>/<kind>_<version>.json` (primary) and `master-standalone/<group>-<kind>-stable-<version>.json` (kubeval compatibility).
- **Concurrency**: extractor uses 10 goroutines (buffered channel semaphore), publisher uses 3 concurrent upload workers. These match the original tools' behavior.
- **Watch mode uses full re-extract.** Simpler than incremental. Upload is already incremental via check-missing. Extraction of <200 CRDs is sub-second.
- **Leader election uses standard client-go leaderelection.** LeaseLock with 15s/10s/2s timings. Leader exits on lease loss (standard controller pattern).
- **All replicas report ready.** Readiness is not gated on leadership. Standard controller pattern — leadership is an internal concern.

### Dependencies (direct only)

| Dependency | Purpose |
|-----------|---------|
| `k8s.io/client-go` | Kubernetes API access |
| `k8s.io/apiextensions-apiserver` | CRD typed client |
| `github.com/zeebo/blake3` | BLAKE3 hashing (pure Go) |

No other external dependencies. Standard library for HTTP, JSON, HTML templates, MIME types.

## CI/CD

- **GitHub Actions** (`.github/workflows/build.yaml`): builds multi-arch image (amd64 + arm64) on push to main, pushes to `ghcr.io/sholdee/crd-schema-publisher`
- **Tags**: date-based (`vYYYY.MMDD.HHMMSS` UTC) + `latest`
- **Container**: `gcr.io/distroless/static:nonroot` runtime base

## Companion Repo

Kubernetes manifests live in the `home-ops` repo under `apps/kubernetes-schemas/`. That repo has its own CLAUDE.md with cluster conventions, RBAC, ExternalSecret, and CronJob configuration.

## Common Mistakes to Avoid

- Changing the BLAKE3 hash algorithm without verifying against wrangler — uploads will fail with hash mismatches
- Adding CGO dependencies — breaks static compilation and distroless compatibility
- Modifying the CF Pages upload flow without checking current wrangler source — the API is undocumented and may change
- Forgetting to update both output directory formats (primary + master-standalone) when changing schema file naming
- The index template uses a deepspace-inspired theme (starfield via coprime-tiled radial gradients, light flare via stripe/rainbow interference). Both effects are pure CSS, dark-mode only, hidden in light mode via `.light body::before, .light .flare { display: none }` (`.light` class is on `<html>`, set in a `<head>` script to prevent FOUC)
- The flare uses `filter: opacity(50%)` AND `opacity: 0.25` intentionally — these multiply to ~12.5% effective opacity. Do not "simplify" by removing one.
