---
title: "Deploying"
description: "Choose between Helm, raw manifests, watch mode, and CronJob mode."
weight: 50
---

The Helm chart is the recommended deployment path. It installs controller mode by default for real-time CRD watching and can run extract-only when Cloudflare credentials are omitted.

## Deployment Choices

| Path | Use When |
| --- | --- |
| [Helm chart](helm/) | You want the supported, signed, configurable Kubernetes deployment. |
| [Raw manifests](raw-manifests/) | You prefer plain YAML and can manage changes yourself. |
| Watch mode | You want schemas updated soon after CRD changes. |
| CronJob mode | You want scheduled extraction and can tolerate stale schemas between runs. |

## Publishing Choices

Cloudflare Pages is built in. Extract-only mode writes the generated site under `OUTPUT_DIR/current` for a built-in server, sidecar, object storage sync, or git push workflow.

See [Publishing Backends](../publishing-backends/) for backend patterns.
