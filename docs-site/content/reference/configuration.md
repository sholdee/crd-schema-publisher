---
title: "Configuration"
description: "Environment variables and runtime configuration behavior."
weight: 150
---

## Environment Variables

Deployment/runtime configuration is primarily via environment variables. For local CLI use, `extract`, `convert`, `run`, `watch`, `upload`, and `preview` also expose command-specific flags such as `--output-dir`/`-o`. Variables marked _(watch)_ apply only to watch mode deployment.

| Variable                   | Required    | Default                | Description                                                                                                           |
| -------------------------- | ----------- | ---------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `CLOUDFLARE_API_TOKEN`     | Upload only | -                      | Cloudflare API token with Pages permissions                                                                           |
| `CLOUDFLARE_ACCOUNT_ID`    | Upload only | -                      | Cloudflare account ID                                                                                                 |
| `CF_PAGES_PROJECT`         | No          | `kubernetes-schemas`   | Cloudflare Pages project name                                                                                         |
| `OUTPUT_DIR`               | No          | `/output`              | Site output root. The active snapshot is exposed at `OUTPUT_DIR/current`                                              |
| `KUBECTL_CONTEXT`          | No          | -                      | Kubernetes context name (local development only)                                                                      |
| `DEBOUNCE_SECONDS`         | No          | `15`                   | Seconds to wait after last CRD event before publishing (watch mode)                                                   |
| `POD_NAME`                 | Yes (watch) | -                      | Pod identity for leader election (set via downward API)                                                               |
| `POD_NAMESPACE`            | Yes (watch) | -                      | Namespace for leader lease (set via downward API)                                                                     |
| `LEASE_NAME`               | No          | `crd-schema-publisher` | Name of the Lease resource (watch mode)                                                                               |
| `HEALTH_PORT`              | No          | `8080`                 | Port for liveness/readiness probes (watch mode)                                                                       |
| `SERVE_SITE`               | No          | -                      | Set to `true` to serve `OUTPUT_DIR/current` from watch mode                                                           |
| `SITE_PORT`                | No          | `8081`                 | Non-privileged static site server port when `SERVE_SITE=true`                                                         |
| `SERVE_ACCESS_LOG`         | No          | -                      | Set to `true` to log each request served by the built-in static site server                                           |
| `PREVIEW_ADDR`             | No          | `127.0.0.1:8989`       | Listen address for preview server (preview mode)                                                                      |
| `SKIP_RENDER`              | No          | -                      | Set to `true` to skip HTML schema page rendering                                                                      |
| `UPLOAD_BUCKET_SIZE_BYTES` | No          | `41943040`             | Cloudflare upload bucket size in bytes. Lower values reduce peak upload memory at the cost of more requests           |
| `UPLOAD_CONCURRENCY`       | No          | `3`                    | Concurrent Cloudflare upload buckets. Lower values reduce peak upload memory at the cost of slower cache-miss uploads |
| `BASE_PATH`                | No          | -                      | URL path prefix for subpath deployments (e.g., `/iac` for GitHub Pages at `user.github.io/iac/`)                      |
| `SCHEMA_INCLUDE_BUILTINS`  | No          | -                      | Set to `true` to include Kubernetes built-ins from API server OpenAPI v2 (`run`, `extract`, `watch`)                  |
| `SCHEMA_INCLUDE_KUSTOMIZE` | No          | -                      | Set to `true` to include kustomize's client-side config schemas (`run`, `extract`, `watch`)                           |
| `SCHEMA_FILTER_KIND`       | No          | -                      | Restrict generated schemas to matching CRD kinds, comma-separated and case-insensitive (`run`, `extract`, `watch`)    |
| `SCHEMA_FILTER_GROUP`      | No          | -                      | Restrict generated schemas to matching API groups, comma-separated and case-insensitive (`run`, `extract`, `watch`)   |
| `SCHEMA_FILTER_VERSION`    | No          | -                      | Restrict generated schemas to matching API versions, comma-separated and case-insensitive (`run`, `extract`, `watch`) |

Schema filters limit generated output only. In watch mode, the controller still watches all cluster CRDs and applies the filters during each publish cycle. If active filters match no CRDs or built-ins and Kustomize is not enabled, runtime builds publish an empty catalog instead of leaving stale schemas in place.

Runtime modes stay CRD-only unless opt-ins are enabled. `--include-builtins` reads `/openapi/v2` from the API server and publishes built-ins into the same generation. `--include-kustomize` adds kustomize's client-side config schemas. Filters apply to CRDs and built-ins; Kustomize is explicit and unfiltered.
