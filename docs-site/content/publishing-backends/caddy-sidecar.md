---
title: "Caddy Sidecar"
description: "Serve schemas directly from the cluster with a sidecar web server."
weight: 100
---

The Caddy sidecar pattern runs `crd-schema-publisher` in extract-only mode and serves the active generated site from the same pod. It does not require Cloudflare credentials.

See the complete [Caddy sidecar values file](https://github.com/sholdee/crd-schema-publisher/blob/main/examples/caddy-sidecar/values.yaml).

## When to Use This

Use this when you want schemas and generated documentation served directly from the cluster through your own Gateway or ingress. The example uses Caddy, but the same `extraContainers` pattern works with nginx or another web server.

The example uses a persistent volume so generated schemas remain available across pod restarts.

## Install

```bash
helm install crd-schema-publisher oci://ghcr.io/sholdee/charts/crd-schema-publisher \
  --namespace crd-schema-publisher --create-namespace \
  -f examples/caddy-sidecar/values.yaml
```

## Customize

Set these values before installing:

| Area | What to change |
| --- | --- |
| Storage | `persistence.storageClass`, if your cluster needs a specific StorageClass |
| Gateway | `HTTPRoute` `parentRefs` for your Gateway name and namespace |
| Hostname | `HTTPRoute` `hostnames` for the public schema hostname |
| Server | Caddy image, Caddyfile, or sidecar container if you prefer another web server |

The example mounts the shared output volume at `/srv` and serves `/srv/current`. The Caddyfile enables directory browsing and hides `_meta`.

## Operational Notes

The publisher writes complete site generations under the output directory and exposes the active snapshot at `/srv/current` for Caddy to serve.

`Recreate` strategy is required with `ReadWriteOnce` persistence. A rolling update can deadlock because the new pod cannot mount the PVC until the old pod releases it.

The Caddy liveness probe checks the sidecar port instead of generated content. That keeps startup before the first successful build from becoming a restart loop.
