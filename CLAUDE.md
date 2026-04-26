# CLAUDE.md - Project Context for crd-schema-publisher

## What This Is

A statically-compiled Go binary that extracts CRD JSON schemas from a Kubernetes cluster and builds a browsable static site. It can publish directly to Cloudflare Pages or run in extract-only mode for sidecar consumers such as local web servers, git sync, or object-storage sync. Runs as a long-lived Deployment (watch mode) or CronJob on Kubernetes. Deployed in a nonroot distroless container.

## Repository Layout

```text
charts/         Helm chart (OCI-distributed via GHCR, cosign-signed)
cmd/            Entrypoint and subcommand dispatch (run/extract/convert/upload/watch/preview)
converter/      OpenAPI v3 -> JSON Schema transforms (ported from openapi2jsonschema.py)
extractor/      client-go CRD listing, schema extraction, file writing, config builder, file/YAML parsing
index/          HTML index generation (deepspace theme, client-side search, starfield/flare effects)
publisher/      Cloudflare Pages direct upload API client + BLAKE3 hashing
renderer/       HTML schema page renderer (collapsible property trees, type badges, constraints)
theme/          Shared CSS/HTML/JS assets (deepspace theme, hash helpers, extracted schema-search module)
metrics/        Prometheus metrics (stdlib-only, atomic counters/gauges, text exposition format)
watcher/        CRD informer watch loop, debounce, leader election, health server, metrics wiring
```

## Build & Test

```bash
# Run all tests
go test ./...

# If you change the extracted schema search module or its tests
node --test theme/schema_search.test.js

# Vet
go vet ./...

# Lint (requires golangci-lint)
golangci-lint run

# Enable pre-commit hook
git config core.hooksPath .githooks

# Cross-compile release binaries locally (matches release workflow targets)
for pair in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  GOOS="${pair%/*}" GOARCH="${pair#*/}"
  CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" \
    go build -ldflags="-s -w" -o "crd-schema-publisher-${GOOS}-${GOARCH}" ./cmd/
done

# Generate release-style checksums
sha256sum crd-schema-publisher-* > checksums-sha256.txt

# Local extract (requires kubeconfig)
KUBECTL_CONTEXT=my-context OUTPUT_DIR=./output go run ./cmd/ extract

# Preview index UI locally (no cluster needed)
go run ./cmd/ preview

# Local subcommands must use the package path so Go compiles all cmd/*.go files.
# Single-file invocation is supported only for top-level help/version smoke checks.
go run ./cmd/main.go --help
```

## Architecture

### Subcommands

- `run` (default) — extract + optional upload. Degrades gracefully: skips upload when Cloudflare credentials are missing, prints guidance when no kubeconfig is available. Accepts `--output-dir`; the output root must already exist.
- `extract` — extract CRDs and build a new site generation, exposed at `OUTPUT_DIR/current`. Requires an explicit output directory via `--output-dir` or `OUTPUT_DIR`; it does not fall back to `/output`. Supports `--kind`, `--group`, `--version` filters and CLI flags (`--output-dir`, `--context`, `--base-path`, `--skip-render`) that override env vars.
- `convert` — convert CRD YAML files to JSON Schema without a cluster connection. Requires `--output-dir`. Reads from `--file` (comma-separated, `-` for stdin) and/or `--dir`. Writes flat output (no generation lifecycle). Optional `--render` for HTML docs. Supports the same `--kind`, `--group`, `--version` filters.
- `upload` — upload the active site from `OUTPUT_DIR/current` to Cloudflare Pages. Accepts `--output-dir`; the output root must already exist.
- `watch` — long-lived process: informer watches CRDs, debounces events, and runs extract plus optional upload cycles. Leader election for multi-replica safety. Accepts `--output-dir`; the output root must already exist.
- `preview` — generate sample data by default, or with explicit `--output-dir` copy the active site into an isolated temp generation and serve it on localhost. Ambient `OUTPUT_DIR` is ignored. No cluster or credentials needed. Handles signal cleanup of temp directories.

### Configuration (env vars)

| Var | Required | Default | Purpose |
| --- | -------- | ------- | ------- |
| `CLOUDFLARE_API_TOKEN` | Upload only | — | CF API token |
| `CLOUDFLARE_ACCOUNT_ID` | Upload only | — | CF account ID |
| `CF_PAGES_PROJECT` | No | `kubernetes-schemas` | CF Pages project name |
| `OUTPUT_DIR` | No | `/output` | Site output root. Stable read path is `OUTPUT_DIR/current` |
| `KUBECTL_CONTEXT` | No | — | K8s context (local dev only) |
| `DEBOUNCE_SECONDS` | No | `15` | Seconds to wait after last CRD event before publishing (watch mode) |
| `POD_NAME` | Yes (watch) | — | Pod identity for leader election (set via downward API) |
| `POD_NAMESPACE` | Yes (watch) | — | Namespace for leader lease (set via downward API) |
| `LEASE_NAME` | No | `crd-schema-publisher` | Name of the Lease resource (watch mode) |
| `HEALTH_PORT` | No | `8080` | Port for liveness/readiness probes (watch mode) |
| `SKIP_RENDER` | No | — | Set to `true` to skip HTML schema page rendering |
| `BASE_PATH` | No | — | URL path prefix for subpath deployments (e.g., `/iac` for GitHub Pages) |
| `PREVIEW_ADDR` | No | `127.0.0.1:8989` | Listen address (preview mode) |

### CLI Flags

`extract`, `convert`, `run`, `watch`, `upload`, and `preview` all support command-specific flags. `extract` requires `--output-dir` or `OUTPUT_DIR` and does not default to `/output`. `convert` requires `--output-dir` and does not read `OUTPUT_DIR`. For runtime-oriented commands (`run`, `watch`, `upload`), `--output-dir` overrides `OUTPUT_DIR` but must point at a pre-created directory. `preview` ignores ambient `OUTPUT_DIR` and only uses real extracted output when `--output-dir` is passed explicitly.

| Flag | Subcommands | Default | Purpose |
| --- | --- | --- | --- |
| `--output-dir` | run, extract, convert, upload, watch, preview | `convert`: required flag; `extract`: `$OUTPUT_DIR` (required if unset); `run`/`upload`/`watch`: `$OUTPUT_DIR` or `/output`; `preview`: none | Output directory |
| `--context` | extract | `$KUBECTL_CONTEXT` | Kubernetes context |
| `--base-path` | extract, convert | `$BASE_PATH` | URL path prefix |
| `--skip-render` | extract | `$SKIP_RENDER` | Skip HTML rendering |
| `--file` | convert | — | CRD YAML file(s), comma-separated. `-` for stdin |
| `--dir` | convert | — | Directory of CRD YAML files (non-recursive) |
| `--render` | convert | `false` | Render HTML docs |
| `--kind` | extract, convert | — | Filter by kind (comma-separated, case-insensitive) |
| `--group` | extract, convert | — | Filter by group (comma-separated, case-insensitive) |
| `--version` | extract, convert | — | Filter by version (comma-separated, case-insensitive) |

### Key Design Decisions

- **No CGO.** Binary is statically linked. BLAKE3 uses `github.com/zeebo/blake3` (pure Go).
- **Cloudflare Pages direct upload API** is undocumented. Implementation reverse-engineered from wrangler source (`cloudflare/workers-sdk`). The upload flow uses JWT auth for asset operations and API token auth for deployment creation. See `publisher/publisher.go` for the full 6-step flow.
- **BLAKE3 file hashing** exactly matches wrangler's `hashFile`: `hex(blake3(base64(content) + extension))[0:32]`. Do not change this algorithm without verifying against wrangler source.
- **OpenAPI v3 to JSON Schema conversion** is an improved port of `openapi2jsonschema.py` from datreeio/CRDs-catalog (via yannh/kubeconform). Three transforms applied in order: additionalProperties, replaceIntOrString, allowNullOptionalFields. Known divergences from the Python original, all intentional improvements for kubeconform/IDE validation correctness: (1) `additionalProperties` uses schema-aware traversal, closing structural child object schemas while skipping same-instance validation overlays (`oneOf`/`anyOf`/`allOf`/`not`/dependencies/conditionals) and literal data-valued keywords such as `default` and `enum`; it also recurses into each property sub-schema individually — Python recurses into the `properties` map itself, so CRD fields named `properties` (or other JSON Schema keywords) get a spurious `additionalProperties: false` injected into the map, corrupting the schema; (2) nullable applies only to fields *not* in the required list — Python disables nullable for *all* siblings when any sibling is required; (3) `replaceIntOrString` preserves safe metadata but removes/replaces conflicting parent type and distributes type-specific assertions into the string or integer oneOf branch for Kubernetes int-or-string markers — Python discards the entire dict; (4) root object and array items are not made nullable — Python makes them nullable unnecessarily. Golden E2E tests (`extractor/testdata/golden_certificate_v1.json`, `golden_edgecase_v1.json`) freeze the converter output and catch regressions.
- **Schema renderer** generates interactive HTML documentation pages (collapsible `<details>`/`<summary>` property trees, type/required badges, YAML boilerplate). Uses `html/template` with recursive `{{define "properties"}}` for nested schemas. Schema-page path search behavior lives in the extracted `theme/schema_search.js` asset, which `RenderAll` emits into the output root as `schema-search.js` and the page bootstrap loads at runtime. Enabled by default; disable with `SKIP_RENDER=true`.
- **Theme package** (`theme/`) holds shared CSS, HTML fragments, small JS helpers used by both index and renderer templates, and emitted static assets such as `schema-search.js`. CSS custom properties are the union of both pages' needs. The deepspace theme (starfield, flare, light/dark toggle) is defined once here.
- **Output format** builds immutable site generations under `OUTPUT_DIR/.generations/<generation>/` and atomically switches `OUTPUT_DIR/current` to the active generation. Each generation contains both directory formats: `<group>/<kind>_<version>.json` (primary) and `master-standalone/<group>-<kind>-stable-<version>.json` (kubeval compatibility), plus rendered `.html` documentation pages, `index.html`, static assets, and an internal `_meta/kinds.json` manifest used to preserve exact CRD Kind casing for re-render/preview paths. Direct-volume consumers should read from `OUTPUT_DIR/current`, not the flat root. First-party Cloudflare, git, S3, and Caddy examples exclude `_meta/` from public output.
- **Convert output cleanup** uses `_meta/convert-manifest.json` to remove only files generated by prior `convert` runs, then prunes empty generated directories. Files that existed before `convert` ran are preserved, even under generated directories. If the manifest exists but is corrupt, `convert` aborts instead of risking mixed stale-and-fresh output.
- **Concurrency**: extractor uses 10 goroutines (buffered channel semaphore), publisher uses 3 concurrent upload workers, renderer uses 10 goroutines. These match the original tools' behavior.
- **Watch mode uses first-trigger-immediate debounce.** The informer's initial List fires AddFunc for all existing CRDs, which signals the debounce loop. The first trigger fires immediately (zero delay), subsequent triggers are debounced. This produces exactly one publish cycle on startup — no explicit initial publish in runLeader.
- **Watch mode uses full re-extract.** Simpler than incremental. Upload is already incremental via check-missing. Extraction of <200 CRDs is sub-second.
- **Leader election uses standard client-go leaderelection.** LeaseLock with 15s/10s/2s timings. Leader exits on lease loss (standard controller pattern).
- **All replicas report ready.** Readiness is not gated on leadership. Standard controller pattern — leadership is an internal concern.
- **Linting** uses golangci-lint with strict linters (gocritic, gocyclo, misspell, prealloc, nolintlint) plus defaults. Config in `.golangci.yml`. Pre-commit hook in `.githooks/pre-commit`.
- **Image signing** uses cosign keyless mode via GitHub OIDC. Production images on main are signed; PR images are not. Base images (golang, distroless) are verified before every build.
- **Supply chain hardening**: all GitHub Actions pinned by commit SHA (not version tag), Dockerfile base images pinned by digest, `go mod verify` runs in CI.
- **Prometheus metrics** use stdlib-only text exposition format (no `prometheus/client_golang` dependency). Atomic counters and gauges in `metrics/` package, served at `/metrics` on the health server port. Metrics are always registered in watch mode — zero overhead when not scraped. Recording methods are nil-receiver safe so callers don't need nil checks. Float gauges use `math.Float64bits`/`math.Float64frombits` with `atomic.Int64` for lock-free storage.
- **Helm chart** in `charts/crd-schema-publisher/` distributed as OCI artifact via GHCR. Two modes (`controller`/`cronjob`) with mode-isolated templates. Two-tier optional secret management: `existingSecret`, `externalSecret` (ESO). Both controller and CronJob modes gracefully degrade to extract-only without Cloudflare creds: controller runs `watch` and omits the publisher, while CronJob runs the default `run` command and skips upload after extraction. `values.schema.json` enforces secret mutual exclusivity via `if/then` (not `oneOf`, which breaks yaml-language-server with `additionalProperties: false`); template precedence in `_helpers.tpl` handles runtime resolution (`existingSecret.name` wins, else chart fullname). NetworkPolicy and CiliumNetworkPolicy are mutually exclusive (enforced via `fail` guard in `_helpers.tpl`). PrometheusRule only renders in controller mode. Chart version is CalVer SemVer: `YYYY.MDD.HMMSS` (no leading zeros on month or hour — e.g. `2026.413.65435`). Image and chart are always released together with the same CalVer version — both `version` and `appVersion` match the image tag. `Chart.yaml` version/appVersion fields are placeholder `0.0.0` — both overridden by the release workflow at package time. Dashboard embedded via `.Files.Get`. Pod anti-affinity presets in `_helpers.tpl`.

### Dependencies (direct only)

| Dependency | Purpose |
| --------- | ------- |
| `github.com/zeebo/blake3` | BLAKE3 hashing (pure Go) |
| `gopkg.in/yaml.v3` | Streaming YAML document decoding for file/stdin conversion |
| `k8s.io/apiextensions-apiserver` | CRD typed client and API types |
| `k8s.io/apimachinery` | Shared Kubernetes API machinery used by clients and watchers |
| `k8s.io/client-go` | Kubernetes API access, informer/watch plumbing, leader election |
| `sigs.k8s.io/yaml` | YAML-to-Kubernetes-struct decoding for CRD conversion |

Everything else in `go.mod` is transitive. The project still leans heavily on the standard library for HTTP, JSON, HTML templates, MIME types, and the preview server.

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
| `binaries` | After `test` and `build` pass | Cross-compiles static binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, generates `checksums-sha256.txt`, signs the checksum manifest with cosign keyless blob signing, uploads `dist/` as artifacts for the `release` job, and publishes GitHub/Sigstore build provenance for the binaries via the attestations service. |
| `build` | After `test` passes | Multi-arch Docker build (amd64 + arm64), pushes `vYYYY.MDD.HMMSS` + `latest` to GHCR. Verifies distroless base image digest with cosign before building. |
| `sign` | After `build` | Cosign keyless signing via GitHub OIDC |
| `helm-package` | After `helm-lint`, `build`, and `sign` | Package chart with CalVer SemVer matching the image tag. Push OCI to GHCR, cosign sign. Image and chart always share the same version — no desync possible. |
| `release` | After `build`, `sign`, `helm-package`, `binaries` | Creates git tag, GitHub Release with auto-generated notes, image digest, chart OCI reference, signed checksum manifest, binary provenance link, and standalone binaries. Note: tag push will fail if the tagged commit includes workflow file changes — create the tag manually in that case. |

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
- When modifying shared CSS, hash-state helpers, or emitted frontend assets, update the `theme/` package source of truth — do not duplicate changes across `index/index.go` and `renderer/renderer.go`
- If you change `theme/schema_search.js`, keep `theme/schema_search.test.js` in sync and run `node --test theme/schema_search.test.js`
- The index template uses a deepspace-inspired theme (starfield via coprime-tiled radial gradients, light flare via stripe/rainbow interference). Both effects are pure CSS, dark-mode only, hidden in light mode via `.light body::before, .light .flare { display: none }` (`.light` class is on `<html>`, set in a `<head>` script to prevent FOUC)
- The flare uses `filter: opacity(50%)` AND `opacity: 0.25` intentionally — these multiply to ~12.5% effective opacity. Do not "simplify" by removing one.
- When updating GitHub Actions, always pin by commit SHA with a version comment (e.g., `actions/checkout@<sha> # v4`). Never use floating version tags.
- When upgrading Go or distroless base images, update the digest in `Dockerfile` and verify the new digest with `cosign verify` before committing.
