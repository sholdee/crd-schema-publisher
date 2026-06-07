---
title: "Commands"
description: "CLI commands, output directory behavior, filters, and command-specific flags."
weight: 160
---

## Command Behavior

```text
crd-schema-publisher [command]

Commands:
  run       Extract schemas and upload to Cloudflare Pages when credentials are configured (default)
  extract   Extract schemas from a Kubernetes cluster
  convert   Convert CRD YAML files and Kubernetes OpenAPI built-ins to JSON Schema
  upload    Upload the active site from OUTPUT_DIR/current to Cloudflare Pages
  watch     Watch for CRD changes and upload when credentials are configured
  preview   Serve a local preview of the documentation site
```

| Command(s)               | Output directory behavior                                                                                    |
| ------------------------ | ------------------------------------------------------------------------------------------------------------ |
| `extract`                | Requires explicit `--output-dir`/`-o` or `OUTPUT_DIR`; does not fall back to `/output`.                      |
| `convert`                | Requires `--output-dir`/`-o`; does not read `OUTPUT_DIR`.                                                    |
| `run`, `watch`, `upload` | Accept `--output-dir`/`-o`; output root must already exist.                                                  |
| `preview`                | Uses sample data by default; reads real extracted output only when `--output-dir`/`-o` is passed explicitly. |

| Command(s)                | Filters and command-specific flags                                                                                                                                                                   |
| ------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `run`, `extract`, `watch` | Support comma-separated, case-insensitive `--kind`, `--group`, and `--version` filters. Defaults can also come from `SCHEMA_FILTER_KIND`, `SCHEMA_FILTER_GROUP`, and `SCHEMA_FILTER_VERSION`.        |
| `run`, `extract`, `watch` | Support `--include-builtins`/`SCHEMA_INCLUDE_BUILTINS` to include built-ins from the API server OpenAPI v2 document.                                                                                 |
| `run`, `extract`, `watch` | Support `--include-kustomize`/`SCHEMA_INCLUDE_KUSTOMIZE` to include kustomize's client-side config schemas.                                                                                          |
| `extract`                 | Supports `--context`, `--base-path`, and `--skip-render`.                                                                                                                                            |
| `convert`                 | Supports comma-separated, case-insensitive `--kind`, `--group`, and `--version` filters for CRD YAML and OpenAPI inputs.                                                                             |
| `convert`                 | Supports `--file`/`-f`, non-recursive `--dir`/`-d` YAML loading, optional `--render`, and `--base-path` for rendered links.                                                                          |
| `convert`                 | `--openapi` converts a Kubernetes OpenAPI v2 (swagger) document of built-in types into self-contained per-kind schemas, combinable with `--file`/`--dir` to render CRDs and built-ins into one site. |
| `convert`                 | `--kustomize` explicitly publishes kustomize's `Kustomization` and `Component` schemas, reflected from the pinned `sigs.k8s.io/kustomize/api` types. It is not filtered.                             |
