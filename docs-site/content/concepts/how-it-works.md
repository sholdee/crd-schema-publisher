---
title: "How It Works"
description: "Extraction, conversion, rendering, generation switching, and upload pipeline."
weight: 190
---

`crd-schema-publisher` has one core loop: collect Kubernetes schemas, normalize them for common tooling, render a static site, then publish or expose the finished snapshot.

## At a Glance

| Stage | What Happens |
| --- | --- |
| Connect | Cluster-backed commands connect to the Kubernetes API in-cluster or through kubeconfig. |
| Extract | The tool lists CRDs and reads `.spec.versions[].schema.openAPIV3Schema`. |
| Normalize | JSON Schema transforms make the output friendlier to kubeconform and IDE validation. |
| Write | Schemas are written in both primary and kubeval-compatible directory layouts. |
| Render | Each schema gets an interactive HTML page with property trees, local `$ref` expansion, path-aware search, and autocomplete. |
| Index | The site index groups schemas by source and API group, with client-side search and usage examples. |
| Activate | Runtime generations atomically switch `OUTPUT_DIR/current` to the completed snapshot. |
| Publish | When Cloudflare credentials are configured, the active generation uploads through the Cloudflare Pages API. |

## Cluster-backed Pipeline

The `run`, `extract`, and `watch` commands read from a Kubernetes cluster. `watch` adds informers, leader election, and debounced refresh cycles so CRD updates eventually produce a new active snapshot.

Runtime generations are written below `OUTPUT_DIR/.generations/<generation>`. After a generation completes, `OUTPUT_DIR/current` is switched atomically. Sidecars and local servers should read `OUTPUT_DIR/current` so they never serve a half-written site.

Cloudflare publishing happens after activation. Uploads use direct Cloudflare Pages deployment APIs with BLAKE3 content hashing, batched uploads, and retry behavior.

## Schema Transforms

| Transform | Why It Exists |
| --- | --- |
| Structural object tightening | Adds `additionalProperties: false` to child objects with `properties`, recursing only through schema-valued locations. This preserves validation overlays and literal `default` or `enum` data while avoiding keyword-collision bugs. |
| Kubernetes int-or-string support | Replaces Kubernetes int-or-string markers with a non-conflicting `oneOf` union. Metadata is preserved, and type-specific assertions move into the matching string or integer branch. |
| Optional null support | Allows null for optional fields with per-field precision, including optional `$ref` fields as ref-or-null `anyOf` wrappers. |

These transforms handle nullable fields, int-or-string types, root objects, and CRD fields named like JSON Schema keywords. A frozen golden test locks converter output to prevent regressions.

## Offline Conversion

The `convert` command skips Kubernetes access. It reads CRD YAML from `--file`/`-f`, stdin (`-f -`), and/or a non-recursive `--dir`/`-d`.

`convert` applies the same schema transforms as cluster-backed commands. It writes flat output directly to `--output-dir`/`-o`; with `--render`, it also renders schema HTML pages and an index.

## Standalone Rendering

The `render` command renders schema HTML pages and an index for an existing output directory. This decouples rendering from extraction and conversion, so schemas can be inspected or modified in between:

```sh
crd-schema-publisher extract -o ./schemas --skip-render
# modify schemas under ./schemas/current/ as needed
crd-schema-publisher render -o ./schemas
```

When `OUTPUT_DIR/current` exists (runtime layout), `render` renders the active generation in place; otherwise it renders the flat directory produced by `convert`. When a convert manifest is present, rendered files are recorded in it so a later `convert` run cleans them.

## Kubernetes Built-ins from OpenAPI

Use `--openapi <swagger.json>` to convert Kubernetes built-in, non-CRD types from an OpenAPI v2 document.

```sh
kubectl get --raw /openapi/v2 > swagger.json
crd-schema-publisher convert --openapi swagger.json -o ./schemas --render
```

Each authorable type that declares a group, version, and kind becomes a self-contained `<group>/<kind>_<version>.json`. The empty API group is written under `core/`.

Referenced definitions are bundled into each schema so validation and rendered child fields work without external references. When combined with CRD inputs, matching OpenAPI CRD definitions and their List types are skipped.

Combine `--openapi` with `--file` or `--dir` when you want one local site containing both built-ins and CRDs.

## Kustomize Schemas

`--kustomize` publishes schemas for kustomize's client-side config types:

- `kustomize.config.k8s.io/kustomization_v1beta1.json`
- `kustomize.config.k8s.io/component_v1alpha1.json`

These types do not have usable upstream schemas. The project reflects them from the `sigs.k8s.io/kustomize/api` Go types pinned in this module. Bumping that dependency updates the generated schemas.

```sh
crd-schema-publisher convert -d ./crds --openapi swagger.json --kustomize -o ./schemas --render
```

## Filtering

`--kind`, `--group`, and `--version` filters limit CRD and OpenAPI inputs. The filters are comma-separated and case-insensitive.

Kustomize is different: `--kustomize` is a single explicit opt-in and always emits the Kustomize config schemas when set. It is not filtered.

## Mixed-source Sites

Runtime modes can include optional schemas in generated snapshots:

- `--include-builtins` fetches `/openapi/v2` from the API server and writes built-ins into the same generation as CRDs.
- `--include-kustomize` adds kustomize's client-side config schemas.

When more than one schema source is present, the generated index separates CRDs, built-ins, and Kustomize schemas. CRD-only output keeps the original API-group-only index.

In the Helm chart, `config.includeBuiltins=true` adds `/openapi/v2` RBAC when `rbac.create=true`; `config.includeKustomize=true` does not require additional Kubernetes permissions.
