<p align="center">
  <img src="https://raw.githubusercontent.com/sholdee/crd-schema-publisher/main/docs/assets/logo.svg" alt="crd-schema-publisher logo" width="96">
</p>

<h1 align="center">crd-schema-publisher</h1>

<p align="center">
  CRD docs and IDE validation, straight from the cluster.
</p>

<p align="center">
  <a href="https://www.codefactor.io/repository/github/sholdee/crd-schema-publisher"><img src="https://www.codefactor.io/repository/github/sholdee/crd-schema-publisher/badge" alt="CodeFactor"></a>
  <a href="https://github.com/sholdee/crd-schema-publisher/actions/workflows/ci.yaml"><img src="https://github.com/sholdee/crd-schema-publisher/actions/workflows/ci.yaml/badge.svg" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/sholdee/crd-schema-publisher" alt="Go Version"></a>
  <a href="https://artifacthub.io/packages/helm/crd-schema-publisher/crd-schema-publisher"><img src="https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/crd-schema-publisher" alt="Artifact Hub"></a>
</p>

Extracts CRD schemas from Kubernetes or YAML, converts Kubernetes built-in resource schemas from `/openapi/v2`, and publishes a searchable documentation site with interactive schema pages.

<p align="center">
  <img src="https://raw.githubusercontent.com/sholdee/crd-schema-publisher/main/docs/screenshots/overview.gif" alt="Installing crd-schema-publisher, extracting CRD schemas, and browsing the generated schema site" width="720">
</p>

<p align="center">
  <a href="https://sholdee.github.io/crd-schema-publisher/">Documentation</a> ·
  <a href="https://kube-schemas.shold.io">Live demo</a> ·
  <a href="https://artifacthub.io/packages/helm/crd-schema-publisher/crd-schema-publisher">Helm chart</a>
</p>

## Why

Static CRD catalogs go stale, miss internal CRDs, and depend on external infrastructure. `crd-schema-publisher` publishes schemas from your own cluster and can serve or sync them wherever you host static files.

- Always accurate for installed CRDs, internal CRDs, and optional Kubernetes built-ins.
- Self-hosted output for Cloudflare Pages, local serving, S3-compatible storage, git, or any static web server.
- Single static binary in a distroless nonroot container.
- Controller-grade watch mode with informers, leader election, debounced refreshes, health probes, and metrics.

## Quickstart

Install the Helm chart in controller mode:

```bash
helm install crd-schema-publisher oci://ghcr.io/sholdee/charts/crd-schema-publisher \
  --namespace crd-schema-publisher \
  --create-namespace \
  --set existingSecret.name=crd-schema-publisher-cloudflare
```

Install the standalone CLI:

```bash
curl -fsSL https://crdsp.shold.io | bash
```

Or install with mise through the aqua backend:

```bash
mise use aqua:sholdee/crd-schema-publisher@latest
```

Extract schemas from a kubeconfig context:

```bash
crd-schema-publisher extract -o ./schemas
```

Convert CRD YAML without a cluster:

```bash
crd-schema-publisher convert -f crd.yaml -o ./schemas
```

Extract CRDs and Kubernetes built-ins directly from a cluster:

```bash
crd-schema-publisher extract -o ./schemas --include-builtins
```

## Use Published Schemas

Published schemas are available at `https://<your-pages-domain>/<apigroup>/<kind>_<version>.json`, for example `cert-manager.io/certificate_v1.json` or `core/pod_v1.json`.

```yaml
# yaml-language-server: $schema=https://kube-schemas.example.com/cert-manager.io/certificate_v1.json
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example
```

## Documentation

- [Getting Started](https://sholdee.github.io/crd-schema-publisher/getting-started/)
- [Installation](https://sholdee.github.io/crd-schema-publisher/installation/)
- [Deploying with Helm](https://sholdee.github.io/crd-schema-publisher/deploying/helm/)
- [Publishing Backends](https://sholdee.github.io/crd-schema-publisher/publishing-backends/)
- [Using Schemas](https://sholdee.github.io/crd-schema-publisher/using-schemas/)
- [Configuration Reference](https://sholdee.github.io/crd-schema-publisher/reference/configuration/)
- [Operations](https://sholdee.github.io/crd-schema-publisher/operations/monitoring/)
- [How It Works](https://sholdee.github.io/crd-schema-publisher/concepts/how-it-works/)

## Community

- [Home Operations Discord](https://discord.gg/home-operations)
- [Contributing](CONTRIBUTING.md)
- [Code of Conduct](CODE_OF_CONDUCT.md)
- [Security Policy](SECURITY.md)
