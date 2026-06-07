---
title: "Installation"
description: "Install the standalone CLI with the installer, direct binary downloads, or mise."
weight: 30
---

The quick installer is the recommended path for local CLI use. Static binaries for Linux and macOS on amd64 and arm64 are attached to each [GitHub Release](https://github.com/sholdee/crd-schema-publisher/releases).

The installer detects Linux/macOS and amd64/arm64, verifies the selected binary against the release checksum manifest, optionally verifies the checksum Sigstore bundle when `cosign` is available, and installs to an existing `crd-schema-publisher` path or `/usr/local/bin/crd-schema-publisher`.

## Quick Install

Install or update to the latest release:

```bash
curl -fsSL https://crdsp.shold.io | bash
```

## Non-interactive Install

Use `--yes` for automation:

```bash
curl -fsSL https://crdsp.shold.io | bash -s -- --yes
```

## Install a Specific Release

Pin a release version:

```bash
curl -fsSL https://crdsp.shold.io | bash -s -- --version vYYYY.MDD.HMMSS
```

## Manual Download

Download a release binary directly when you do not want to run the installer:

```bash
# Download the latest release (example: Linux amd64)
curl -LO https://github.com/sholdee/crd-schema-publisher/releases/latest/download/crd-schema-publisher-linux-amd64
chmod +x crd-schema-publisher-linux-amd64
```

## Optional mise-managed CLI Install

If you manage project CLIs with [mise](https://mise.jdx.dev/), install the released CLI through the aqua backend:

```bash
mise use aqua:sholdee/crd-schema-publisher@latest
```

Or pin a release in `mise.toml`:

```toml
[tools]
"aqua:sholdee/crd-schema-publisher" = "2026.519.317"
```

## Verification

See [Artifact Verification](verification/) for release checksum, Sigstore bundle, binary attestation, Helm chart signature, and container image verification commands.
