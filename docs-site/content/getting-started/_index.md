---
title: "Getting Started"
description: "Deploy the chart, install the CLI, and point tools at your published schemas."
weight: 20
---

Use `crd-schema-publisher` as a Kubernetes controller, a scheduled CronJob, or a local CLI. The quickest path is Helm for clusters and the installer for local extraction or conversion.

## Deploy to a Cluster

Install the Helm chart in controller mode for real-time CRD watching. Provide Cloudflare credentials to publish directly to Cloudflare Pages, or omit credentials to run extract-only and serve `OUTPUT_DIR/current` yourself.

```bash
helm install crd-schema-publisher oci://ghcr.io/sholdee/charts/crd-schema-publisher \
  --namespace crd-schema-publisher \
  --create-namespace \
  --set existingSecret.name=crd-schema-publisher-cloudflare
```

See [Helm Chart](../deploying/helm/) for credentials, CronJob mode, alternative backends, and chart verification.

## Install and Run the CLI

Install the standalone CLI:

```bash
curl -fsSL https://crdsp.shold.io | bash
```

Extract schemas from a kubeconfig context, include Kubernetes built-ins from the API server, or convert CRD YAML:

```bash
# Extract from the current kubeconfig context
crd-schema-publisher extract -o ./schemas

# Extract from a specific context
crd-schema-publisher extract --context my-cluster -o ./schemas

# Extract CRDs and Kubernetes built-ins from the current cluster
crd-schema-publisher extract -o ./schemas --include-builtins

# Convert CRD YAML without a cluster
crd-schema-publisher convert -f crd.yaml -o ./schemas
```

For source builds, use `go run ./cmd/` in place of `crd-schema-publisher`. See [Installation](../installation/) for installer options and manual downloads, and [Commands](../reference/commands/) for flags and command behavior.

## Use Published Schemas

Once published, schemas are available at `https://<your-pages-domain>/<apigroup>/<kind>_<version>.json`, for example `cert-manager.io/certificate_v1.json` or `core/pod_v1.json`.

```yaml
# yaml-language-server: $schema=https://kube-schemas.example.com/cert-manager.io/certificate_v1.json
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example
```

See [Using Schemas](../using-schemas/) for IDE and kubeconform examples.

## Next Steps

- [Deploy with Helm](../deploying/helm/)
- [Install the standalone CLI](../installation/)
- [Use published schemas](../using-schemas/)
- [Choose a publishing backend](../publishing-backends/)
