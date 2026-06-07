---
title: "Helm Chart"
description: "Install and configure the signed OCI Helm chart."
weight: 60
---

The Helm chart is distributed as an OCI artifact and signed with cosign. It installs in controller mode by default for real-time CRD watching with leader election. For scheduled runs, set `--set mode=cronjob`.

## Install

```bash
helm install crd-schema-publisher oci://ghcr.io/sholdee/charts/crd-schema-publisher \
  --namespace crd-schema-publisher \
  --create-namespace \
  --set existingSecret.name=crd-schema-publisher-cloudflare
```

See [Artifact Verification](../../installation/verification/) for chart signature verification.

## Credentials

Cloudflare credentials are optional in both controller and CronJob modes. Without them, the workload runs in extract-only mode. Site generations are written under the output directory and the active snapshot is exposed at `OUTPUT_DIR/current`, but nothing is uploaded.

This is useful when serving schemas locally with a sidecar web server or another publishing backend instead of Cloudflare Pages.

To publish to Cloudflare Pages, provide an API token with Cloudflare Pages: Edit permission and your account ID. Two secret management options are supported:

- `existingSecret` references a pre-existing Secret containing `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID`.
- `externalSecret` creates an [ExternalSecret](https://external-secrets.io) CR that syncs credentials from an external provider such as Vault, AWS Secrets Manager, or 1Password.

```bash
# Using External Secrets Operator
helm install crd-schema-publisher oci://ghcr.io/sholdee/charts/crd-schema-publisher \
  --namespace crd-schema-publisher \
  --create-namespace \
  --set externalSecret.enabled=true \
  --set externalSecret.secretStoreRef.name=my-store \
  --set externalSecret.secretStoreRef.kind=ClusterSecretStore
```

The default remote ref points to a `crd-schema-publisher-cloudflare` key with `api-token` and `account-id` properties. Override via `externalSecret.data` if your provider uses different paths.

## Schema Filtering

To publish only part of the cluster CRD catalog, set `config.filter.group`, `config.filter.kind`, and/or `config.filter.version`. Values are comma-separated and case-insensitive.

```bash
helm install crd-schema-publisher oci://ghcr.io/sholdee/charts/crd-schema-publisher \
  --namespace crd-schema-publisher \
  --create-namespace \
  --set config.filter.group=cert-manager.io \
  --set-string 'config.filter.kind=Certificate\,Issuer'
```

Controller mode still watches all CRDs, then applies the filter to each generated output snapshot. If active filters match no CRDs or built-ins and Kustomize is not enabled, the next runtime build publishes an empty catalog instead of preserving a previous broader snapshot.

## Runtime Built-ins and Kustomize

Runtime modes publish CRDs only by default. Enable built-ins and Kustomize explicitly when you want one site for CRDs, Kubernetes built-in types, and kustomize's client-side `Kustomization` and `Component` schemas.

```bash
helm upgrade --install crd-schema-publisher oci://ghcr.io/sholdee/charts/crd-schema-publisher \
  --namespace crd-schema-publisher \
  --set config.includeBuiltins=true \
  --set config.includeKustomize=true
```

`config.includeBuiltins=true` reads `/openapi/v2` from the API server. With chart RBAC enabled, it also adds the required ClusterRole permission; with `rbac.create=false`, provide that permission yourself.

`config.includeKustomize=true` does not require extra Kubernetes permissions. Filters apply to CRDs and built-ins; Kustomize is an explicit unfiltered opt-in.

## Optional Features

Most installs can skip these options. Use them when you need retained output, in-cluster serving, observability resources, or stricter workload controls.

| Need | Chart values |
| --- | --- |
| Retain generated output across pod restarts | `persistence` |
| Serve the active site from the pod | `serve` |
| Expose the built-in server through Gateway API | `serve.httpRoute` |
| Add sidecars, sync containers, custom mounts, or extra objects | `extraVolumes`, `extraVolumeMounts`, `extraContainers`, `extraObjects` |
| Export Prometheus and Grafana resources | `metrics.podMonitor`, `metrics.prometheusRule`, `grafana.dashboard` |
| Restrict network traffic | `networkPolicy`, `ciliumNetworkPolicy` |
| Improve rollout availability | `podDisruptionBudget`, `affinity`, `podAntiAffinityPreset`, `topologySpreadConstraints` |

See [Monitoring](../../operations/monitoring/) for metrics and dashboard setup, and [values.yaml](https://github.com/sholdee/crd-schema-publisher/blob/main/charts/crd-schema-publisher/values.yaml) for the full option reference.
