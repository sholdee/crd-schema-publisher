# CLAUDE.md - Project Context for crd-schema-publisher

## What This Is

A statically-compiled Go binary that extracts CRD JSON schemas from a Kubernetes cluster and publishes them to a Cloudflare Pages website. Runs as a long-lived Deployment (watch mode) or CronJob in a K3s cluster. Deployed in a nonroot distroless container.

## Repository Layout

```text
charts/         Helm chart (OCI-distributed via GHCR, cosign-signed)
cmd/            Entrypoint and subcommand dispatch (run/extract/upload/watch/preview)
converter/      OpenAPI v3 -> JSON Schema transforms (ported from openapi2jsonschema.py)
extractor/      client-go CRD listing, schema extraction, file writing, config builder
index/          HTML index generation (deepspace theme, client-side search, starfield/flare effects)
publisher/      Cloudflare Pages direct upload API client + BLAKE3 hashing
renderer/       HTML schema page renderer (collapsible property trees, type badges, constraints)
theme/          Shared CSS/HTML/JS constants (deepspace theme, starfield, flare, toggle, toast, footer)
metrics/        Prometheus metrics (stdlib-only, atomic counters/gauges, text exposition format)
watcher/        CRD informer watch loop, debounce, leader election, health server, metrics wiring
```

## Build & Test

```bash
# Run all tests
go test ./...

# Vet
go vet ./...

# Lint (requires golangci-lint)
golangci-lint run

# Enable pre-commit hook
git config core.hooksPath .githooks

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
| --- | -------- | ------- | ------- |
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
| `SKIP_RENDER` | No | — | Set to `true` to skip HTML schema page rendering |
| `PREVIEW_ADDR` | No | `127.0.0.1:8989` | Listen address (preview mode) |

### Key Design Decisions

- **No CGO.** Binary is statically linked. BLAKE3 uses `github.com/zeebo/blake3` (pure Go).
- **Cloudflare Pages direct upload API** is undocumented. Implementation reverse-engineered from wrangler source (`cloudflare/workers-sdk`). The upload flow uses JWT auth for asset operations and API token auth for deployment creation. See `publisher/publisher.go` for the full 6-step flow.
- **BLAKE3 file hashing** exactly matches wrangler's `hashFile`: `hex(blake3(base64(content) + extension))[0:32]`. Do not change this algorithm without verifying against wrangler source.
- **OpenAPI v3 to JSON Schema conversion** is an improved port of `openapi2jsonschema.py` from datreeio/CRDs-catalog (via yannh/kubeconform). Three transforms applied in order: additionalProperties, replaceIntOrString, allowNullOptionalFields. Known divergences from the Python original, all intentional improvements for kubeconform/IDE validation correctness: (1) nullable applies only to fields *not* in the required list — Python disables nullable for *all* siblings when any sibling is required; (2) `replaceIntOrString` preserves existing keys alongside oneOf — Python discards the entire dict; (3) root object and array items are not made nullable — Python makes them nullable unnecessarily. A golden E2E test (`extractor/testdata/golden_certificate_v1.json`) freezes the converter output and catches any regression.
- **Schema renderer** generates interactive HTML documentation pages (collapsible `<details>`/`<summary>` property trees, type/required badges, YAML boilerplate). Uses `html/template` with recursive `{{define "properties"}}` for nested schemas. Enabled by default; disable with `SKIP_RENDER=true`.
- **Theme package** (`theme/`) holds shared CSS, HTML fragments, and JS used by both index and renderer templates. CSS custom properties are the union of both pages' needs. The deepspace theme (starfield, flare, light/dark toggle) is defined once here.
- **Output format** produces two directory structures: `<group>/<kind>_<version>.json` (primary) and `master-standalone/<group>-<kind>-stable-<version>.json` (kubeval compatibility). Each JSON schema also gets a sibling `.html` documentation page.
- **Concurrency**: extractor uses 10 goroutines (buffered channel semaphore), publisher uses 3 concurrent upload workers, renderer uses 10 goroutines. These match the original tools' behavior.
- **Watch mode uses first-trigger-immediate debounce.** The informer's initial List fires AddFunc for all existing CRDs, which signals the debounce loop. The first trigger fires immediately (zero delay), subsequent triggers are debounced. This produces exactly one publish cycle on startup — no explicit initial publish in runLeader.
- **Watch mode uses full re-extract.** Simpler than incremental. Upload is already incremental via check-missing. Extraction of <200 CRDs is sub-second.
- **Leader election uses standard client-go leaderelection.** LeaseLock with 15s/10s/2s timings. Leader exits on lease loss (standard controller pattern).
- **All replicas report ready.** Readiness is not gated on leadership. Standard controller pattern — leadership is an internal concern.
- **Linting** uses golangci-lint with strict linters (gocritic, gocyclo, misspell, prealloc, nolintlint) plus defaults. Config in `.golangci.yml`. Pre-commit hook in `.githooks/pre-commit`.
- **Image signing** uses cosign keyless mode via GitHub OIDC. Production images on main are signed; PR images are not. Base images (golang, distroless) are verified before every build.
- **Supply chain hardening**: all GitHub Actions pinned by commit SHA (not version tag), Dockerfile base images pinned by digest, `go mod verify` runs in CI.
- **Prometheus metrics** use stdlib-only text exposition format (no `prometheus/client_golang` dependency). Atomic counters and gauges in `metrics/` package, served at `/metrics` on the health server port. Metrics are always registered in watch mode — zero overhead when not scraped. Recording methods are nil-receiver safe so callers don't need nil checks. Float gauges use `math.Float64bits`/`math.Float64frombits` with `atomic.Int64` for lock-free storage.
- **Helm chart** in `charts/crd-schema-publisher/` distributed as OCI artifact via GHCR. Two modes (`controller`/`cronjob`) with mode-isolated templates. Two-tier secret management: `existingSecret`, `externalSecret` (ESO) — or no secret at all for extract-only mode (watch gracefully degrades without Cloudflare creds). `values.schema.json` enforces secret mutual exclusivity via `if/then` (not `oneOf`, which breaks yaml-language-server with `additionalProperties: false`); template precedence in `_helpers.tpl` handles runtime resolution (`existingSecret.name` wins, else chart fullname). NetworkPolicy and CiliumNetworkPolicy are mutually exclusive (enforced via `fail` guard in `_helpers.tpl`). PrometheusRule only renders in controller mode. Chart version is CalVer SemVer: `YYYY.MDD.HMMSS` (no leading zeros on month or hour — e.g. `2026.413.65435`). Image and chart are always released together with the same CalVer version — both `version` and `appVersion` match the image tag. `Chart.yaml` version/appVersion fields are placeholder `0.0.0` — both overridden by the release workflow at package time. Dashboard embedded via `.Files.Get`. Pod anti-affinity presets in `_helpers.tpl`.

### Dependencies (direct only)

| Dependency | Purpose |
| --------- | ------- |
| `k8s.io/client-go` | Kubernetes API access |
| `k8s.io/apiextensions-apiserver` | CRD typed client |
| `github.com/zeebo/blake3` | BLAKE3 hashing (pure Go) |

No other external dependencies. Standard library for HTTP, JSON, HTML templates, MIME types.

## CI/CD

### Pipeline Architecture

Four workflow files in `.github/workflows/`:

| File | Trigger | Purpose |
| --- | ------- | ------- |
| `test.yml` | `workflow_call` | Reusable: actionlint, markdownlint-cli2, golangci-lint, go mod verify/tidy, go test, go vet, govulncheck |
| `helm-lint.yml` | `workflow_call` | Reusable: `helm lint`, `helm template` in controller/cronjob/all-features modes, mode isolation checks, schema validation, dashboard JSON embedding, kubeconform |
| `ci.yaml` | PR + push to `main` | Orchestrator: calls `test.yml` and `helm-lint.yml`, runs detect/build/renovate/gate |
| `release.yaml` | `workflow_dispatch` | Orchestrator: calls `test.yml` and `helm-lint.yml`, runs build/sign/helm-package/release |

**`ci.yaml`** jobs:

| Job | Runs when | Purpose |
| --- | --------- | ------- |
| `detect` | Always | `dorny/paths-filter` classifies changes: `app` (Go, go.mod/sum, Dockerfile), `chart` (charts/**), `renovate` (config only). |
| `test` | Always | Calls `test.yml` — safety net, ensures Go code compiles on every PR, even docs-only |
| `build` | PR + `app == true` | Multi-arch Docker build (amd64 + arm64), pushes `pr-N` tag to GHCR. Verifies distroless base image digest with cosign before building. |
| `renovate` | `renovate == true` | Validates `.github/renovate.json5` with `renovate-config-validator --strict` |
| `helm-lint` | `chart == true` | Calls `helm-lint.yml` |
| `gate` | Always | Evaluates all job results — only `success` and `skipped` pass. Single required status check for branch protection. |

Pushes to main run `test` and `helm-lint` only (no Docker build, no release). PR builds produce `pr-N` images for testing.

**`release.yaml`** jobs:

| Job | Runs when | Purpose |
| --- | --------- | ------- |
| `test` | Always | Calls `test.yml` — re-runs all linting and Go tests as a safety net before building |
| `helm-lint` | Always | Calls `helm-lint.yml` — re-runs all Helm validation before packaging |
| `build` | After `test` passes | Multi-arch Docker build (amd64 + arm64), pushes `vYYYY.MDD.HMMSS` + `latest` to GHCR. Verifies distroless base image digest with cosign before building. |
| `sign` | After `build` | Cosign keyless signing via GitHub OIDC |
| `helm-package` | After `helm-lint`, `build`, and `sign` | Package chart with CalVer SemVer matching the image tag. Push OCI to GHCR, cosign sign. Image and chart always share the same version — no desync possible. |
| `release` | After `build`, `sign`, `helm-package` | Creates git tag, GitHub Release with auto-generated notes, image digest, and chart OCI reference. Note: tag push will fail if the tagged commit includes workflow file changes — create the tag manually in that case. |

App image and Helm chart are always released together with the same CalVer version. Releases are decoupled from CI — trigger the release workflow when changes warrant a new version. The release workflow re-runs all tests before building as a safety net. A concurrency group prevents simultaneous releases from racing.

### Tags

- **Production:** `vYYYY.MDD.HMMSS` (UTC, no leading zeros on month/hour — e.g. `v2026.413.65435`) + `latest` — created by manual release workflow
- **PR:** `pr-N` — created on every PR with app code changes

### Renovate (`.github/renovate.json5`)

Automated dependency management with platform automerge.

- **Presets:** `config:recommended`, `docker:enableMajor`, `:semanticCommits`, `helpers:pinGitHubActionDigests`
- **Automerge (minor/patch):** GitHub Actions, Docker images (including digest), Go modules, CI tools
- **Custom manager:** Regex manager matches `go install github.com/<org>/<repo>/...@v<version>` in workflow files, updates via `github-releases` datasource
- **Major updates:** require manual review (all dependency types)

### Dependency Pinning

| Dependency type | Pinning strategy | Example |
| -------------- | --------------- | ------- |
| GitHub Actions | Commit SHA + version comment | `actions/checkout@<sha> # v4` |
| Dockerfile base images | Tag + manifest digest | `golang:1.26.2@sha256:...` |
| Go modules | `go.mod` + `go.sum`, verified with `go mod verify` | Standard |
| CI tools (`go install`) | Semver tag, tracked by Renovate custom manager | `actionlint@v1.7.12` |

### OCI Labels

Static labels in Dockerfile: `source`, `description`, `licenses`. Build-time labels injected by CI: `revision` (commit SHA), `version` (date tag or `dev`), `created` (timestamp).

## Companion Repo

Kubernetes manifests live in the `home-ops` repo under `apps/kubernetes-schemas/`. That repo has its own CLAUDE.md with cluster conventions, RBAC, ExternalSecret, and CronJob configuration.

## Common Mistakes to Avoid

- Changing the BLAKE3 hash algorithm without verifying against wrangler — uploads will fail with hash mismatches
- Adding CGO dependencies — breaks static compilation and distroless compatibility
- Modifying the CF Pages upload flow without checking current wrangler source — the API is undocumented and may change
- Forgetting to update both output directory formats (primary + master-standalone) when changing schema file naming
- When modifying shared CSS or HTML fragments, edit `theme/theme.go` — do not duplicate changes across `index/index.go` and `renderer/renderer.go`
- The index template uses a deepspace-inspired theme (starfield via coprime-tiled radial gradients, light flare via stripe/rainbow interference). Both effects are pure CSS, dark-mode only, hidden in light mode via `.light body::before, .light .flare { display: none }` (`.light` class is on `<html>`, set in a `<head>` script to prevent FOUC)
- The flare uses `filter: opacity(50%)` AND `opacity: 0.25` intentionally — these multiply to ~12.5% effective opacity. Do not "simplify" by removing one.
- When updating GitHub Actions, always pin by commit SHA with a version comment (e.g., `actions/checkout@<sha> # v4`). Never use floating version tags.
- When upgrading Go or distroless base images, update the digest in `Dockerfile` and verify the new digest with `cosign verify` before committing.
