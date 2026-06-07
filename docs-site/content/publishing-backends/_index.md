---
title: "Publishing Backends"
description: "Publish generated schema sites with Cloudflare Pages, the built-in server, sidecars, object storage, or git."
weight: 80
---

Runtime modes can publish directly to Cloudflare Pages when credentials are present. Without credentials, the workload runs extract-only and writes a complete generated site to `OUTPUT_DIR/current`.

Use extract-only mode when another container or process should serve or sync the generated site.

## Supported Patterns

| Pattern | Backend | Example |
| --- | --- | --- |
| [Built-in server](built-in-server/) | Controller HTTP server | `serve.enabled=true`, [values file](https://github.com/sholdee/crd-schema-publisher/blob/main/examples/built-in-server/values.yaml) |
| [Caddy sidecar](caddy-sidecar/) | Local HTTP | [examples/caddy-sidecar/values.yaml](https://github.com/sholdee/crd-schema-publisher/blob/main/examples/caddy-sidecar/values.yaml) |
| [S3-compatible storage](s3/) | AWS S3, Backblaze B2, MinIO, R2, GCS | [examples/rclone-s3/values.yaml](https://github.com/sholdee/crd-schema-publisher/blob/main/examples/rclone-s3/values.yaml) |
| [Git push](git-push/) | GitHub Pages, GitLab Pages, Gitea, Bitbucket | [examples/git-push/values.yaml](https://github.com/sholdee/crd-schema-publisher/blob/main/examples/git-push/values.yaml) |
| [GitHub Pages subpaths](github-pages/) | Project Pages path prefixes | `config.basePath` |

The built-in server is the smallest in-cluster option. Sidecar examples use the chart's `extraContainers` and `extraObjects` values so you can adapt the same pattern to another web server, object storage sync tool, or git host without changing `crd-schema-publisher`.

Examples that push to external storage run stateless with an `emptyDir`. The Caddy example uses a persistent volume because it serves directly from the cluster.

Each example is a self-contained values file. Copy it, fill in your credentials, and install it. See the comments in each file for what to customize.
