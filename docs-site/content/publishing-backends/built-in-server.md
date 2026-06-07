---
title: "Built-in Server"
description: "Serve the active generated site directly from the controller container."
weight: 90
---

For simple in-cluster deployments, the controller can serve the active generated site directly from `OUTPUT_DIR/current`.

```bash
helm upgrade --install crd-schema-publisher oci://ghcr.io/sholdee/charts/crd-schema-publisher \
  --namespace crd-schema-publisher \
  --set serve.enabled=true
```

The site is exposed on the chart Service port named `site` and defaults to non-privileged port `8081`. Health and metrics stay on the `health` port.

Built-in serving is controller-only, requires `replicaCount=1`, and switches the Deployment strategy to `Recreate` so traffic is not routed to a new pod before it has published its first site.

Use the complete [built-in server values file](https://github.com/sholdee/crd-schema-publisher/blob/main/examples/built-in-server/values.yaml) for persistence and Gateway API `HTTPRoute` setup. Customize the `HTTPRoute` `parentRefs` and `hostnames` for your Gateway.
