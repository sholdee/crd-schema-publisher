---
title: "crd-schema-publisher"
linkTitle: "Overview"
description: "Generate CRD schema docs and IDE validation catalogs directly from Kubernetes."
weight: 10
---

`crd-schema-publisher` extracts CRD schemas from Kubernetes or YAML, converts Kubernetes built-in resource schemas from `/openapi/v2`, and publishes a searchable static site with interactive schema pages.

<figure class="media-frame">
  <img src="screenshots/overview.gif" alt="Installing crd-schema-publisher, extracting CRD schemas, and browsing the generated schema site">
</figure>

```bash
helm install crd-schema-publisher oci://ghcr.io/sholdee/charts/crd-schema-publisher \
  --namespace crd-schema-publisher \
  --create-namespace \
  --set existingSecret.name=crd-schema-publisher-cloudflare
```

```bash
curl -fsSL https://crdsp.shold.io | bash
crd-schema-publisher extract -o ./schemas
```

<div class="home-grid">
  <a href="getting-started/">Get started</a>
  <a href="deploying/helm/">Deploy with Helm</a>
  <a href="using-schemas/">Use published schemas</a>
  <a href="https://kube-schemas.shold.io">View live demo</a>
</div>

## Why

Static CRD catalogs go stale, miss internal CRDs, and depend on external infrastructure. `crd-schema-publisher` publishes schemas from your own cluster.

- Always accurate for installed CRDs and optional built-in Kubernetes resources.
- Self-hosted output that works with Cloudflare Pages, local serving, S3-compatible storage, git, or any static web server.
- Single static binary with no runtime interpreters or package managers.
- Controller-grade watch mode with informers, leader election, debounced refreshes, health probes, and metrics.

The JSON Schema conversion is built for kubeconform and IDE compatibility.

## Main workflows

- Run a Kubernetes-native controller for real-time CRD watching.
- Run a CronJob for scheduled extraction.
- Run the local CLI to extract from a live cluster, include Kubernetes built-ins, or convert CRD YAML.
