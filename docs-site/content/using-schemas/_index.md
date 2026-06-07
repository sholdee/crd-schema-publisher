---
title: "Using Schemas"
description: "Use published schemas with yaml-language-server, VS Code, and kubeconform."
weight: 140
---

Once published, your schemas are available at `https://<your-pages-domain>/<apigroup>/<kind>_<version>.json`. Core built-ins use the `core` group path, such as `core/pod_v1.json`. The published site also includes a browsable index with search and interactive HTML documentation for each schema.

## IDE Validation (yaml-language-server)

Add a modeline to any YAML file. Works in VS Code, Neovim, Helix, and any editor with yaml-language-server:

```yaml
# yaml-language-server: $schema=https://kube-schemas.example.com/cert-manager.io/certificate_v1.json
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example
```

Or configure schemas globally in VS Code:

```jsonc
// .vscode/settings.json
{
  "yaml.schemas": {
    "https://kube-schemas.example.com/cert-manager.io/certificate_v1.json": ["**/certificates/*.yaml"],
  },
}
```

## CI Validation (kubeconform)

If your registry includes built-ins from runtime `--include-builtins` or offline `convert --openapi`, point kubeconform at both the `core` path and the grouped path:

```bash
kubeconform \
  -strict \
  -ignore-missing-schemas \
  -schema-location 'https://kube-schemas.example.com/core/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' \
  -schema-location 'https://kube-schemas.example.com/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' \
  manifests/*.yaml
```

This project validates its own Helm chart manifests against its published schema registry in CI - see the `helm-lint` job in [`.github/workflows/ci.yaml`](https://github.com/sholdee/crd-schema-publisher/blob/main/.github/workflows/ci.yaml) for a working example.

This validates Kubernetes built-ins and CRDs against the same published schema snapshot. If you publish only CRDs, keep `-schema-location default` before your custom registry path so kubeconform still validates built-ins.

> **Note:** Schema files are written as lowercase (e.g., `certificate_v1.json`) while `{{.ResourceKind}}` expands to the original case (e.g., `Certificate`). This works on Cloudflare Pages because it serves paths case-insensitively - the same convention used by [datreeio/CRDs-catalog](https://github.com/datreeio/CRDs-catalog). If serving schemas from a case-sensitive host, use lowercase kind names in your template paths.
