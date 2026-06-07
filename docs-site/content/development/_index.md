---
title: "Development"
description: "Run, preview, build, lint, and contribute to crd-schema-publisher."
weight: 200
---

## Run Locally

Use the package path (`go run ./cmd/ <command>`) for local subcommands so Go compiles every file in `cmd/`. Single-file invocation (`go run ./cmd/main.go --help` or `--version`) is only kept for top-level smoke checks.

```bash
# Extract schemas from a local cluster (no upload)
KUBECTL_CONTEXT=my-cluster OUTPUT_DIR=./output go run ./cmd/ extract
# Writes the active snapshot under ./output/current

# Full run with upload
mkdir -p ./output
KUBECTL_CONTEXT=my-cluster \
  CLOUDFLARE_API_TOKEN=xxx \
  CLOUDFLARE_ACCOUNT_ID=xxx \
  go run ./cmd/ --output-dir ./output

# Convert CRD YAML files to JSON Schema (no cluster needed)
go run ./cmd/ convert -f crd.yaml -o ./schemas

# Convert Kubernetes built-ins from a cluster OpenAPI document
kubectl get --raw /openapi/v2 > swagger.json
go run ./cmd/ convert --openapi swagger.json -o ./schemas --render

# Convert all CRDs in a directory
go run ./cmd/ convert -d ./crds/ -o ./schemas

# Pipe from kubectl
kubectl get crds -o yaml | go run ./cmd/ convert -f - -o ./schemas

# Filter by kind and group
go run ./cmd/ extract --output-dir ./schemas --kind certificate,issuer --group cert-manager.io

# Filter a runtime extraction through env vars
SCHEMA_FILTER_GROUP=cert-manager.io go run ./cmd/ --output-dir ./output
```

## Preview the Site Locally

Preview is useful for UI development and local inspection. It needs no cluster or credentials when using sample data.

```bash
# Preview the index UI (no cluster or credentials needed)
go run ./cmd/ preview
# open http://127.0.0.1:8989

# Preview with real extracted schemas
go run ./cmd/ preview --output-dir ./output
# Serves the active snapshot from ./output/current

# Preview a subpath deployment locally
BASE_PATH=/iac go run ./cmd/ preview
# open http://127.0.0.1:8989/iac/
```

## Build

```bash
# Native build
go build -o crd-schema-publisher ./cmd/

# Example static cross-compile
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o crd-schema-publisher ./cmd/

# Build all release binaries locally
for pair in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  GOOS="${pair%/*}" GOARCH="${pair#*/}"
  CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" \
    go build -ldflags="-s -w" -o "crd-schema-publisher-${GOOS}-${GOARCH}" ./cmd/
done

# Docker (multi-arch)
docker buildx build --platform linux/amd64,linux/arm64 -t crd-schema-publisher .
```

## Linting

This project uses [golangci-lint](https://golangci-lint.run/) with strict linters enabled:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
golangci-lint run
```

Enable the pre-commit hook to enforce linting before each commit:

```bash
git config core.hooksPath .githooks
```

If you change the extracted schema search module or its tests, also run:

```bash
node --test theme/schema_search.test.js
```

## Renovate

Dependencies are managed by [Renovate](https://docs.renovatebot.com/). Minor and patch updates for Go modules, GitHub Actions, Docker images, and CI tools are automerged after required status checks pass.

## Community

- [Home Operations Discord](https://discord.gg/home-operations)
- [Contributing](https://github.com/sholdee/crd-schema-publisher/blob/main/CONTRIBUTING.md)
- [Code of Conduct](https://github.com/sholdee/crd-schema-publisher/blob/main/CODE_OF_CONDUCT.md)
- [Security Policy](https://github.com/sholdee/crd-schema-publisher/blob/main/SECURITY.md)
