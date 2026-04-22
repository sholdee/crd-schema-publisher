[![Go Report Card](https://goreportcard.com/badge/github.com/sholdee/crd-schema-publisher)](https://goreportcard.com/report/github.com/sholdee/crd-schema-publisher)
[![CI](https://github.com/sholdee/crd-schema-publisher/actions/workflows/ci.yaml/badge.svg)](https://github.com/sholdee/crd-schema-publisher/actions/workflows/ci.yaml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/sholdee/crd-schema-publisher)](go.mod)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/crd-schema-publisher)](https://artifacthub.io/packages/helm/crd-schema-publisher/crd-schema-publisher)

# crd-schema-publisher — CRD docs and IDE validation, straight from the cluster

Reads CRD JSON schemas directly from your Kubernetes API server and publishes a browsable documentation site with search and interactive schema pages. Runs as a Kubernetes-native controller with real-time CRD watching, or as a CronJob for scheduled extraction. Exports schemas for IDE validation (yaml-language-server) and CI linting (kubeconform). Cloudflare Pages is the built-in backend; S3, git repos, and local serving supported via sidecar.

> **Direct-volume consumers:** the active site now lives at `OUTPUT_DIR/current`. This is a breaking change for sidecars or scripts that read the shared output volume directly. Cloudflare Pages users do not need to change anything.

**[Live demo →](https://kube-schemas.shold.io)**

## 💡 Why

Most CRD schema solutions rely on static catalogs — community-maintained repositories that scrape schemas from popular Helm charts. Schemas go stale, internal CRDs are missing, and your validation pipeline depends on third-party infrastructure.

- **Always accurate** — schemas reflect exactly what's installed in your cluster, including custom and internal CRDs, updated automatically when CRDs change
- **Self-hosted** — run in extract-only mode and serve schemas however you like, or publish directly to Cloudflare Pages
- **Single static binary** — no runtime dependencies, no interpreters, no package managers. One binary in a distroless nonroot container with no shell
- **Controller-grade runtime** — watch mode uses informers, leader election, debounced publish cycles, and health probes. It's a proper workload, not a script on a timer
- **No glue pipelines** — replaces multi-tool chains (CI runners, shell scripts, kubectl, CLI wrappers) with a single in-cluster binary. No external CI dependency, no cluster-admin runner pods, no scheduled workflow orchestration

The JSON Schema conversion improves upon the widely-used [openapi2jsonschema.py](https://github.com/yannh/kubeconform/blob/master/scripts/openapi2jsonschema.py) — see [How It Works](#%EF%B8%8F-how-it-works) for details.

## 🚀 Deploying

### Helm Chart (recommended)

The chart is distributed as an OCI artifact and signed with cosign:

```bash
helm install crd-schema-publisher oci://ghcr.io/sholdee/charts/crd-schema-publisher \
  --namespace crd-schema-publisher \
  --create-namespace \
  --set existingSecret.name=crd-schema-publisher-cloudflare
```

This installs in **controller mode** by default (real-time watch with leader election). For scheduled runs, set `--set mode=cronjob`.

#### Credentials

Cloudflare credentials are **optional in controller mode**. Without them, the Deployment runs in extract-only mode — site generations are written under the output directory and the active snapshot is exposed at `OUTPUT_DIR/current`, but nothing is uploaded. This is useful when serving schemas locally (e.g., via a sidecar web server) instead of Cloudflare Pages.

CronJob mode still expects Cloudflare credentials because it runs the default `run` command (`extract` + `upload`). Extract-only cronjobs are not chart-supported yet.

To publish to Cloudflare Pages, provide an API token with **Cloudflare Pages: Edit** permission and your account ID. Two secret management options are supported:

- **`existingSecret`** — reference a pre-existing Secret containing `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID`
- **`externalSecret`** — create an [ExternalSecret](https://external-secrets.io) CR that syncs credentials from an external provider (Vault, AWS Secrets Manager, 1Password, etc.)

```bash
# Using External Secrets Operator
helm install crd-schema-publisher oci://ghcr.io/sholdee/charts/crd-schema-publisher \
  --namespace crd-schema-publisher \
  --create-namespace \
  --set externalSecret.enabled=true \
  --set externalSecret.secretStoreRef.name=my-store \
  --set externalSecret.secretStoreRef.kind=ClusterSecretStore
```

The default remote ref points to a `crd-schema-publisher-cloudflare` key with `api-token` and `account-id` properties — override via `externalSecret.data` if your provider uses different paths.

#### Optional features

Persistent output volume (`persistence`), extra volumes/volume mounts/containers (`extraVolumes`, `extraVolumeMounts`, `extraContainers`), PodMonitor, PrometheusRule, Grafana dashboard (sidecar ConfigMap), NetworkPolicy, CiliumNetworkPolicy, PodDisruptionBudget, pod anti-affinity presets, topology spread constraints, and templated extra objects. See [`values.yaml`](charts/crd-schema-publisher/values.yaml) for all options.

#### Examples: Alternative backends via sidecar pattern

The chart's `extraContainers` and `extraObjects` values let you wire up any backend without changes to the tool. Each example runs in extract-only mode (no Cloudflare credentials) — schemas are written to generation snapshots under the output directory and the active site is exposed at `OUTPUT_DIR/current` for the sidecar to serve or sync. Examples that push to external storage run stateless with an emptyDir; the caddy example uses a persistent volume since it serves directly from the cluster.

```bash
helm install crd-schema-publisher oci://ghcr.io/sholdee/charts/crd-schema-publisher \
  --namespace crd-schema-publisher --create-namespace \
  -f examples/<example>/values.yaml
```

| Example | Backend | Description |
| ------- | ------- | ----------- |
| [`caddy-sidecar`](examples/caddy-sidecar/values.yaml) | Local HTTP | Caddy serves schemas directly from the cluster with directory browsing and a Gateway API HTTPRoute. Adaptable to nginx or any web server. |
| [`rclone-s3`](examples/rclone-s3/values.yaml) | S3-compatible storage | rclone syncs schemas to any S3-compatible provider (AWS S3, Backblaze B2, MinIO, Cloudflare R2, GCS) on a 60-second interval. Provider-specific configuration documented in the file header. |
| [`git-push`](examples/git-push/values.yaml) | Git repository | Commits and pushes schema changes to a GitHub repository for GitHub Pages hosting. Works with any git host (GitLab, Gitea, Bitbucket) by adjusting the remote URL. |

Each example is a self-contained values file — copy it, fill in your credentials, and install. See the comments in each file for what to customize.

If you already use the first-party `git-push` or `rclone-s3` examples, update them to read `/data/current`. Older example configs fail closed after upgrading to a new image: syncing stops, but existing remote content is not deleted or overwritten. Cloudflare Pages users do not need to change anything.

#### Verification

Verify the chart signature (substitute the version you installed — find it with `helm list`):

```bash
cosign verify ghcr.io/sholdee/charts/crd-schema-publisher:<VERSION> \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp github.com/sholdee/crd-schema-publisher
```

### Raw Manifests

For users who prefer raw YAML without Helm, deploy manifests are available in [`deploy/`](deploy/).

### Watch Mode (recommended)

Reacts to CRD changes in real-time with debounced publish cycles. Supports leader election for safe rolling updates. The container runs with `args: ["watch"]` — see [`deploy/deployment.yaml`](deploy/deployment.yaml).

```bash
kubectl apply -f deploy/common.yaml -f deploy/deployment.yaml
```

### CronJob Mode

Runs extract + upload on a schedule. Simpler, but schemas are only updated when the job runs. The example uses a daily schedule — adjust the `schedule` field as needed. Uses the default `run` command — see [`deploy/cronjob.yaml`](deploy/cronjob.yaml).

```bash
kubectl apply -f deploy/common.yaml -f deploy/cronjob.yaml
```

Both modes share [`deploy/common.yaml`](deploy/common.yaml) which provides namespace, ServiceAccount, RBAC (ClusterRole for CRD read access), and a hardened security context (nonroot, read-only rootfs, dropped capabilities).

The deploy manifests include a placeholder Secret named `crd-schema-publisher-cloudflare`. Edit the placeholder values in `common.yaml` directly, or replace the Secret with your own secrets management (e.g., ExternalSecret, Sealed Secret). If the Secret is omitted, the Deployment can still run in extract-only mode (site generations written under `OUTPUT_DIR/.generations` with the active snapshot exposed at `OUTPUT_DIR/current`, but not uploaded). The CronJob manifest still requires Cloudflare credentials because it uses the default `run` command.

### Container Image

Pre-built multi-arch images (amd64 + arm64) are published to GHCR:

```text
ghcr.io/sholdee/crd-schema-publisher:latest
```

Releases are triggered manually via the release workflow, producing a date-based tag (`vYYYY.MDD.HMMSS` — e.g. `v2026.413.65435`) and `latest`, with auto-generated release notes including the image digest and OCI Helm chart reference. PR builds get `pr-N` tags for testing.

Images use `gcr.io/distroless/static:nonroot` as the runtime base — no shell, no package manager, runs as UID 65534. Production images are signed with [cosign](https://docs.sigstore.dev/cosign/overview/) keyless signing via GitHub Actions OIDC:

```bash
cosign verify ghcr.io/sholdee/crd-schema-publisher:latest \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp github.com/sholdee/crd-schema-publisher
```

### Configuration

All configuration is via environment variables. Variables marked *(watch)* apply only to watch mode deployment.

| Variable | Required | Default | Description |
| -------- | -------- | ------- | ----------- |
| `CLOUDFLARE_API_TOKEN` | Yes (run/upload) | — | Cloudflare API token with Pages permissions |
| `CLOUDFLARE_ACCOUNT_ID` | Yes (run/upload) | — | Cloudflare account ID |
| `CF_PAGES_PROJECT` | No | `kubernetes-schemas` | Cloudflare Pages project name |
| `OUTPUT_DIR` | No | `/output` | Site output root. The active snapshot is exposed at `OUTPUT_DIR/current` |
| `KUBECTL_CONTEXT` | No | — | Kubernetes context name (local development only) |
| `DEBOUNCE_SECONDS` | No | `15` | Seconds to wait after last CRD event before publishing (watch mode) |
| `POD_NAME` | Yes (watch) | — | Pod identity for leader election (set via downward API) |
| `POD_NAMESPACE` | Yes (watch) | — | Namespace for leader lease (set via downward API) |
| `LEASE_NAME` | No | `crd-schema-publisher` | Name of the Lease resource (watch mode) |
| `HEALTH_PORT` | No | `8080` | Port for liveness/readiness probes (watch mode) |
| `PREVIEW_ADDR` | No | `127.0.0.1:8989` | Listen address for preview server (preview mode) |
| `SKIP_RENDER` | No | — | Set to `true` to skip HTML schema page rendering |
| `BASE_PATH` | No | — | URL path prefix for subpath deployments (e.g., `/iac` for GitHub Pages at `user.github.io/iac/`) |

### Subpath Deployments (GitHub Pages)

When hosting schemas at a URL subpath (e.g., `https://user.github.io/iac/`), set `BASE_PATH` to the subpath:

```bash
BASE_PATH=/iac go run ./cmd/ extract
```

Or in the Helm chart:

```yaml
config:
  basePath: "/iac"
```

All generated HTML links will include the base path prefix. The preview server also respects `BASE_PATH`, mounting content at the subpath for local testing:

```bash
BASE_PATH=/iac go run ./cmd/ preview
# → http://127.0.0.1:8989/iac/
```

### Monitoring

In watch mode, the health server exposes a `/metrics` endpoint on `HEALTH_PORT` (default 8080) in Prometheus text format.

| Metric | Type | Description |
| ------ | ---- | ----------- |
| `crdpublisher_publish_cycle_duration_seconds` | gauge | Duration of the most recent publish cycle |
| `crdpublisher_publish_cycle_total` | counter | Publish cycles by result (`success`, `error`) |
| `crdpublisher_crds_discovered` | gauge | CRDs found in the most recent cycle |
| `crdpublisher_schemas_written` | gauge | Schemas written in the most recent cycle |
| `crdpublisher_last_successful_publish_timestamp` | gauge | Unix epoch of the last successful publish |
| `crdpublisher_watchdog_timestamp` | gauge | Unix epoch of the last debounce loop tick |
| `crdpublisher_publish_skipped_total` | counter | Debounce skips (publish already in progress) |
| `crdpublisher_leader` | gauge | Whether this pod is the current leader |

The watchdog timestamp enables dead man's switch alerting — it updates on every debounce loop tick (regardless of whether a publish occurs), so `time() - crdpublisher_watchdog_timestamp` staying fresh proves the watcher is alive. The publish timestamp separately tracks when content was last pushed.

The Helm chart includes a PodMonitor — enable it with `--set metrics.podMonitor.enabled=true`. For raw manifests or vanilla Prometheus Operator, create a PodMonitor:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: crd-schema-publisher
spec:
  selector:
    matchLabels:
      app: crd-schema-publisher
  podMetricsEndpoints:
    - port: health
      path: /metrics
```

For vanilla Prometheus, add `prometheus.io/*` annotations to the pod template or configure a static scrape target.

## 📋 Using Your Schemas

Once published, your schemas are available at `https://<your-pages-domain>/<apigroup>/<kind>_<version>.json`. The published site also includes a browsable index with search and interactive HTML documentation for each schema.

### IDE Validation (yaml-language-server)

Add a modeline to any YAML file. Works in VS Code, Neovim, Helix, and any editor with yaml-language-server:

```yaml
# yaml-language-server: $schema=https://kube-schemas.example.com/cert-manager.io/certificate_v1.json
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example
```

Or configure schemas globally in VS Code:

```jsonc
// .vscode/settings.json
{
  "yaml.schemas": {
    "https://kube-schemas.example.com/cert-manager.io/certificate_v1.json": ["**/certificates/*.yaml"]
  }
}
```

### CI Validation (kubeconform)

Point kubeconform at your schema registry for CRD validation in CI:

```bash
kubeconform \
  -schema-location default \
  -schema-location 'https://kube-schemas.example.com/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' \
  manifests/*.yaml
```

This project validates its own Helm chart manifests against its published schema registry in CI — see the `helm-lint` job in [`.github/workflows/ci.yaml`](.github/workflows/ci.yaml) for a working example.

This validates built-in Kubernetes resources against the default schemas and CRDs against your published schemas.

> **Note:** Schema files are written as lowercase (e.g., `certificate_v1.json`) while `{{.ResourceKind}}` expands to the original case (e.g., `Certificate`). This works on Cloudflare Pages because it serves paths case-insensitively — the same convention used by [datreeio/CRDs-catalog](https://github.com/datreeio/CRDs-catalog). If serving schemas from a case-sensitive host, use lowercase kind names in your template paths.

## Output Structure

```text
<output-dir>/
  .generations/
    <generation>/
      <apigroup>/
        <kind>_<version>.json          # JSON schema
        <kind>_<version>.html          # Interactive documentation page
      _meta/
        kinds.json                     # Internal renderer metadata manifest
      master-standalone/
        <apigroup>-<kind>-stable-<version>.json  # kubeval-compatible format
      index.html                       # Browsable schema index
      favicon.svg                      # Constellation icon
  current -> .generations/<generation> # Stable read path for sidecars and local servers
```

Direct-volume consumers should read or serve `OUTPUT_DIR/current`, not the flat root of `OUTPUT_DIR`. First-party Cloudflare, git, S3, and Caddy examples exclude `_meta/` from published or served output.

## ⚙️ How It Works

```text
crd-schema-publisher [command]

Commands:
  run       Extract schemas and upload to Cloudflare Pages (default)
  extract   Extract schemas and generate the active site under OUTPUT_DIR/current
  upload    Upload the active site from OUTPUT_DIR/current to Cloudflare Pages
  watch     Watch for CRD changes and publish on each change (extract-only if no credentials)
  preview   Generate a sample or active site snapshot and serve it locally
```

1. Connects to the Kubernetes API (in-cluster or via kubeconfig)
2. Lists all CRDs and extracts `.spec.versions[].schema.openAPIV3Schema`
3. Applies three JSON Schema transforms (improved from [openapi2jsonschema.py](https://github.com/yannh/kubeconform/blob/master/scripts/openapi2jsonschema.py)):
   - Adds `additionalProperties: false` to objects with `properties`
   - Replaces `int-or-string` format with `oneOf [string, integer]` while preserving existing keys
   - Allows null for optional fields (per-field precision, not per-parent)

   These improve on the Python original's handling of nullable fields, int-or-string types, and root objects. A frozen golden test locks converter output to prevent regressions.
4. Writes schemas to both primary and kubeval-compatible directory formats inside a new generation snapshot
5. Renders an interactive HTML documentation page for each schema with collapsible property trees
6. Generates an HTML index grouped by API group with client-side search, schema statistics, and yaml-language-server usage examples
7. Atomically switches `OUTPUT_DIR/current` to the completed generation so sidecars read a stable snapshot
8. Uploads the active generation to Cloudflare Pages via the direct upload API (BLAKE3 content hashing, batched uploads with retry)

## 🔧 Development

### Run Locally

```bash
# Preview the index UI (no cluster or credentials needed)
go run ./cmd/ preview
# open http://127.0.0.1:8989

# Preview with real extracted schemas
OUTPUT_DIR=./output go run ./cmd/ preview
# Serves the active snapshot from ./output/current

# Extract schemas from a local cluster (no upload)
KUBECTL_CONTEXT=my-cluster OUTPUT_DIR=./output go run ./cmd/ extract
# Writes the active snapshot under ./output/current

# Full run with upload
KUBECTL_CONTEXT=my-cluster \
  CLOUDFLARE_API_TOKEN=xxx \
  CLOUDFLARE_ACCOUNT_ID=xxx \
  go run ./cmd/
```

### Build

```bash
# Native build
go build -o crd-schema-publisher ./cmd/

# Static cross-compile (matches CI)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o crd-schema-publisher ./cmd/

# Docker (multi-arch)
docker buildx build --platform linux/amd64,linux/arm64 -t crd-schema-publisher .
```

### Linting

This project uses [golangci-lint](https://golangci-lint.run/) with strict linters enabled:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
golangci-lint run
```

Enable the pre-commit hook to enforce linting before each commit:

```bash
git config core.hooksPath .githooks
```

### Renovate

Dependencies are managed by [Renovate](https://docs.renovatebot.com/). Minor and patch updates for Go modules, GitHub Actions, Docker images, and CI tools are automerged after required status checks pass.

See [CONTRIBUTING.md](CONTRIBUTING.md) for full contributor setup and guidelines.

## 👥 Community

- [Home Operations Discord](https://discord.gg/home-operations)
- [Contributing](CONTRIBUTING.md)
- [Code of Conduct](CODE_OF_CONDUCT.md)
- [Security Policy](SECURITY.md)
