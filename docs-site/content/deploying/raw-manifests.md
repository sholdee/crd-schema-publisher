---
title: "Raw Manifests"
description: "Deploy watch mode or CronJob mode with plain Kubernetes YAML."
weight: 70
---

For users who prefer raw YAML without Helm, deploy manifests are available in [deploy/](https://github.com/sholdee/crd-schema-publisher/blob/main/deploy/).

## Watch Mode

Watch mode reacts to CRD changes in real time with debounced schema refreshes, uploading only when Cloudflare credentials are configured. It supports leader election for safe rolling updates.

The container runs with `args: ["watch"]`. See [deploy/deployment.yaml](https://github.com/sholdee/crd-schema-publisher/blob/main/deploy/deployment.yaml).

```bash
kubectl apply -f deploy/common.yaml -f deploy/deployment.yaml
```

## CronJob Mode

CronJob mode runs scheduled schema extraction, uploading to Cloudflare Pages only when credentials are configured. It is simpler than watch mode, but schemas are only updated when the job runs.

The example uses a daily schedule. Adjust the `schedule` field as needed. It uses the default `run` command. See [deploy/cronjob.yaml](https://github.com/sholdee/crd-schema-publisher/blob/main/deploy/cronjob.yaml).

```bash
kubectl apply -f deploy/common.yaml -f deploy/cronjob.yaml
```

## Shared Manifests

Both modes share [deploy/common.yaml](https://github.com/sholdee/crd-schema-publisher/blob/main/deploy/common.yaml), which provides namespace, ServiceAccount, RBAC with ClusterRole access for CRD reads, and a hardened security context.

The shared security context runs as nonroot, uses a read-only root filesystem, and drops capabilities.

## Credentials and Extract-only Mode

The deploy manifests include an empty placeholder Secret named `crd-schema-publisher-cloudflare`. Fill in the values in `common.yaml` directly, or replace the Secret with your own secrets management such as ExternalSecret or Sealed Secret.

If Cloudflare credentials are empty or omitted, workloads run in extract-only mode. Site generations are written under `OUTPUT_DIR/.generations`, and the active snapshot is exposed at `OUTPUT_DIR/current`, but nothing is uploaded.

Without Cloudflare credentials, raw CronJob mode is extract-only. With the default `emptyDir` output volume, extracted schemas are discarded when the Job pod exits unless you replace it with retained storage or a backend sync.
