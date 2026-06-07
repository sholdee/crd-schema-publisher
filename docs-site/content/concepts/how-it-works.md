---
title: "How It Works"
description: "Extraction, conversion, rendering, generation switching, and upload pipeline."
weight: 190
---

For cluster-backed commands (`run`, `extract`, and `watch`), the pipeline is:

1. Connects to the Kubernetes API (in-cluster or via kubeconfig)
2. Lists all CRDs and extracts `.spec.versions[].schema.openAPIV3Schema`
3. Applies three JSON Schema transforms:
   - Adds `additionalProperties: false` to structural child objects with `properties` - recurses into schema-valued locations only, preserving validation overlays and literal `default`/`enum` data while fixing a bug in the original where CRD fields named `properties` or other JSON Schema keywords corrupt the output
   - Replaces Kubernetes int-or-string markers with a non-conflicting `oneOf` union, preserving safe metadata and moving type-specific assertions into the matching string or integer branch
   - Allows null for optional fields (per-field precision, including optional `$ref` fields as ref-or-null `anyOf` wrappers)

   These transforms handle nullable fields, int-or-string types, root objects, and keyword-colliding property names. A frozen golden test locks converter output to prevent regressions.

4. Writes schemas to both primary and kubeval-compatible directory formats inside a new generation snapshot
5. Renders an interactive HTML documentation page for each schema with collapsible property trees, local `$ref` expansion, path-aware search, and autocomplete powered by a shared emitted `schema-search.js` asset
6. Generates an HTML index grouped by schema source and API group with client-side search, schema statistics, and yaml-language-server usage examples
7. Atomically switches `OUTPUT_DIR/current` to the completed generation so sidecars read a stable snapshot
8. Uploads the active generation to Cloudflare Pages via the direct upload API (BLAKE3 content hashing, batched uploads with retry)

The `convert` command skips Kubernetes access and reads CRD YAML from `--file`/`-f`, stdin (`-f -`), and/or a non-recursive `--dir`/`-d`. It applies the same schema transforms and writes flat output directly to `--output-dir`/`-o`; with `--render`, it also renders HTML pages and an index.

Runtime modes can include optional schemas in generated snapshots. `--include-builtins` fetches `/openapi/v2` from the API server and writes authorable built-in types into the same generation as CRDs. When OpenAPI also contains CRD-backed definitions, CRD schemas take precedence and those OpenAPI duplicates are skipped. `--include-kustomize` writes kustomize's client-side config schemas. When more than one schema source is present, the index separates CRDs, built-ins, and Kustomize schemas; CRD-only output keeps the original API-group-only index. In the Helm chart, `config.includeBuiltins=true` adds `/openapi/v2` RBAC when `rbac.create=true`; `config.includeKustomize=true` does not require additional Kubernetes permissions.

`--openapi <swagger.json>` converts Kubernetes' built-in (non-CRD) types from an OpenAPI v2 document (for example `kubectl get --raw /openapi/v2`). Each authorable type that declares a group/version/kind becomes a self-contained `<group>/<kind>_<version>.json`; the empty API group is written under `core/`. Referenced definitions are bundled into each schema so validation and rendered child fields work without external references. When combined with CRD inputs, matching OpenAPI CRD definitions and their List types are skipped.

```sh
kubectl get --raw /openapi/v2 > swagger.json
crd-schema-publisher convert --openapi swagger.json -o ./schemas --render
```

Combine `--openapi` with `--file` or `--dir` when you want one local site containing both built-ins and CRDs.

`--kustomize` publishes schemas for kustomize's `Kustomization` and `Component` at `kustomize.config.k8s.io/kustomization_v1beta1.json` and `kustomize.config.k8s.io/component_v1alpha1.json`. These are client-side types with no usable upstream schemas, so they are reflected from the `sigs.k8s.io/kustomize/api` Go types pinned in this module. Bumping that dependency updates the schemas. Combine them with the other inputs in a single run:

```sh
crd-schema-publisher convert -d ./crds --openapi swagger.json --kustomize -o ./schemas --render
```

`--kind`, `--group`, and `--version` filters limit CRD and OpenAPI inputs; `--kustomize` is a single explicit opt-in and always emits the Kustomize config schemas when set.
