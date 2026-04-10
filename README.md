# crd-schema-publisher

Extracts CRD JSON schemas from a Kubernetes cluster and publishes them to a Cloudflare Pages website. Runs as a Kubernetes Deployment (watch mode) or CronJob in a distroless nonroot container.

## Features

- Extracts OpenAPI v3 schemas from all CustomResourceDefinitions in a cluster
- Converts to standard JSON Schema (ports [openapi2jsonschema.py](https://github.com/yannh/kubeconform/blob/master/scripts/openapi2jsonschema.py) transforms)
- Generates a browsable HTML index page with search, dark/light mode, and visual effects inspired by the Scalar deepspace theme
- Uploads directly to Cloudflare Pages using the direct upload API
- Creates the CF Pages project automatically if it doesn't exist
- Produces kubeval-compatible output in `master-standalone/` directory
- Zero external runtime dependencies — single static binary in a distroless container

## Usage

```
crd-schema-publisher [command]

Commands:
  run       Extract schemas and upload to Cloudflare Pages (default)
  extract   Extract schemas and generate index to OUTPUT_DIR
  upload    Upload OUTPUT_DIR contents to Cloudflare Pages
  watch     Watch for CRD changes and publish on each change (long-lived)
  preview   Generate index with sample data and serve locally for UI development
```

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `CLOUDFLARE_API_TOKEN` | Yes (run/upload) | — | Cloudflare API token with Pages permissions |
| `CLOUDFLARE_ACCOUNT_ID` | Yes (run/upload) | — | Cloudflare account ID |
| `CF_PAGES_PROJECT` | No | `kubernetes-schemas` | Cloudflare Pages project name |
| `OUTPUT_DIR` | No | `/output` | Directory for schema output |
| `KUBECTL_CONTEXT` | No | — | Kubernetes context name (local development only) |
| `DEBOUNCE_SECONDS` | No | `15` | Seconds to wait after last CRD event before publishing (watch mode) |
| `POD_NAME` | Yes (watch) | — | Pod identity for leader election (set via downward API) |
| `POD_NAMESPACE` | Yes (watch) | — | Namespace for leader lease (set via downward API) |
| `LEASE_NAME` | No | `crd-schema-publisher` | Name of the Lease resource (watch mode) |
| `HEALTH_PORT` | No | `8080` | Port for liveness/readiness probes (watch mode) |
| `PREVIEW_ADDR` | No | `127.0.0.1:8989` | Listen address for preview server (preview mode) |

### Run in Kubernetes

Two deployment modes are available:

**Watch mode (recommended)** — reacts to CRD changes in real-time with debounced publish cycles. Supports leader election for safe rolling updates.

```bash
kubectl apply -f deploy/common.yaml -f deploy/deployment.yaml
```

**CronJob mode** — runs on a daily schedule. Simpler, but schemas are only updated once per day.

```bash
kubectl apply -f deploy/common.yaml -f deploy/cronjob.yaml
```

Both modes include:
- Namespace, ServiceAccount, RBAC (ClusterRole for CRD read access)
- Secret placeholder for Cloudflare credentials
- Hardened security context (nonroot, read-only rootfs, dropped capabilities)

If Cloudflare credentials are omitted, both modes run extract-only (schemas written to `OUTPUT_DIR` but not uploaded).

### Preview the Index UI

No cluster or credentials required — serves sample data on localhost:

```bash
go run ./cmd/ preview
# open http://127.0.0.1:8989

# Or preview with real extracted schemas
OUTPUT_DIR=./output go run ./cmd/ preview
```

### Run Locally

```bash
# Extract schemas from a local cluster (no upload)
KUBECTL_CONTEXT=my-cluster OUTPUT_DIR=./output go run ./cmd/ extract

# Full run with upload
KUBECTL_CONTEXT=my-cluster \
  CLOUDFLARE_API_TOKEN=xxx \
  CLOUDFLARE_ACCOUNT_ID=xxx \
  go run ./cmd/
```

## Container Image

Pre-built multi-arch images (amd64 + arm64) are published to GHCR on every push to `main`:

```
ghcr.io/sholdee/crd-schema-publisher:latest
```

Images use `gcr.io/distroless/static:nonroot` as the runtime base — no shell, no package manager, runs as UID 65534.

## Build

```bash
# Native build
go build -o crd-schema-publisher ./cmd/

# Static cross-compile (matches CI)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o crd-schema-publisher ./cmd/

# Docker (multi-arch)
docker buildx build --platform linux/amd64,linux/arm64 -t crd-schema-publisher .
```

## Output Structure

```
<output-dir>/
  <apigroup>/
    <kind>_<version>.json          # Primary format
  master-standalone/
    <apigroup>-<kind>-stable-<version>.json  # kubeval-compatible format
  index.html                       # Browsable schema index
```

## How It Works

1. Connects to the Kubernetes API (in-cluster or via kubeconfig)
2. Lists all CRDs and extracts `.spec.versions[].schema.openAPIV3Schema`
3. Applies three JSON Schema transforms (matching [openapi2jsonschema.py](https://github.com/yannh/kubeconform/blob/master/scripts/openapi2jsonschema.py)):
   - Adds `additionalProperties: false` to objects with `properties`
   - Replaces `int-or-string` format with `oneOf [string, integer]`
   - Allows null for optional fields
4. Writes schemas to both primary and kubeval-compatible directory formats
5. Generates an HTML index grouped by API group with client-side search, stats, and yaml-language-server usage examples
6. Uploads to Cloudflare Pages via the direct upload API (BLAKE3 content hashing, batched uploads with retry)

## License

MIT
