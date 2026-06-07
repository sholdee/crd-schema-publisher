---
title: "Monitoring"
description: "Prometheus metrics, PodMonitor setup, and Grafana dashboard options."
weight: 170
---

In watch mode, the health server exposes a `/metrics` endpoint on `HEALTH_PORT` (default 8080) in Prometheus text format.

| Metric                                           | Type    | Description                                   |
| ------------------------------------------------ | ------- | --------------------------------------------- |
| `crdpublisher_publish_cycle_duration_seconds`    | gauge   | Duration of the most recent publish cycle     |
| `crdpublisher_publish_cycle_total`               | counter | Publish cycles by result (`success`, `error`) |
| `crdpublisher_crds_discovered`                   | gauge   | CRDs found in the most recent cycle           |
| `crdpublisher_schemas_written`                   | gauge   | Schemas written in the most recent cycle      |
| `crdpublisher_last_successful_publish_timestamp` | gauge   | Unix epoch of the last successful publish     |
| `crdpublisher_watchdog_timestamp`                | gauge   | Unix epoch of the last debounce loop tick     |
| `crdpublisher_publish_skipped_total`             | counter | Debounce skips (publish already in progress)  |
| `crdpublisher_leader`                            | gauge   | Whether this pod is the current leader        |

The watchdog timestamp enables dead man's switch alerting - it updates on every debounce loop tick (regardless of whether a publish occurs), so `time() - crdpublisher_watchdog_timestamp` staying fresh proves the watcher is alive. The publish timestamp separately tracks when content was last pushed.

The Helm chart includes a PodMonitor - enable it with `--set metrics.podMonitor.enabled=true`. For raw manifests or vanilla Prometheus Operator, create a PodMonitor:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: crd-schema-publisher
spec:
  selector:
    matchLabels:
      app: crd-schema-publisher
  podMetricsEndpoints:
    - port: health
      path: /metrics
```

For vanilla Prometheus, add `prometheus.io/*` annotations to the pod template or configure a static scrape target.

## Grafana Dashboard

The Helm chart can install the included Grafana dashboard in either sidecar-discovery mode or Grafana Operator mode.

For Grafana sidecars that discover dashboard `ConfigMap` objects:

```bash
helm upgrade --install crd-schema-publisher oci://ghcr.io/sholdee/charts/crd-schema-publisher \
  --namespace crd-schema-publisher \
  --set grafana.dashboard.enabled=true
```

For Grafana Operator:

```bash
helm upgrade --install crd-schema-publisher oci://ghcr.io/sholdee/charts/crd-schema-publisher \
  --namespace crd-schema-publisher \
  --set grafana.dashboard.operator.enabled=true
```

Operator mode renders a `GrafanaDashboard` that references the embedded dashboard `ConfigMap`. It selects Grafana instances with `dashboards: grafana` by default. Custom `grafana.dashboard.operator.instanceSelector` values replace that fallback instead of being merged with it:

```yaml
grafana:
  dashboard:
    operator:
      enabled: true
      instanceSelector:
        matchLabels:
          grafana.internal/instance: home
```

To intentionally match all Grafana instances visible to the operator, set `grafana.dashboard.operator.defaultInstanceSelector.enabled=false`.

Use `grafana.dashboard.operator.folder` for a Grafana folder title, `grafana.dashboard.operator.folderRef` for a `GrafanaFolder` resource reference, or `grafana.dashboard.operator.folderUID` for an existing Grafana folder UID. Set only one folder option.

If your Grafana datasource name is not resolved automatically, map the bundled dashboard input with:

```yaml
grafana:
  dashboard:
    operator:
      datasources:
        - inputName: DS_PROMETHEUS
          datasourceName: Prometheus
```

Grafana Operator and its CRDs must already be installed in the cluster.
