---
title: "Artifact Verification"
description: "Verify release checksums, binary provenance, Helm chart signatures, and container images."
weight: 40
---

Use release checksums, Sigstore bundles, GitHub artifact attestations, and cosign signatures to verify the artifacts you install or deploy.

## Verify Release Artifacts

```bash
# Verify the signed checksum manifest
curl -LO https://github.com/sholdee/crd-schema-publisher/releases/latest/download/checksums-sha256.txt
curl -LO https://github.com/sholdee/crd-schema-publisher/releases/latest/download/checksums-sha256.txt.sigstore.json
cosign verify-blob checksums-sha256.txt \
  --bundle checksums-sha256.txt.sigstore.json \
  --certificate-identity 'https://github.com/sholdee/crd-schema-publisher/.github/workflows/release.yaml@refs/heads/main' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com'

# Verify the binary against the trusted checksum manifest
sha256sum -c --ignore-missing checksums-sha256.txt

# Optional: verify build provenance for the binary
gh attestation verify ./crd-schema-publisher-linux-amd64 \
  --repo sholdee/crd-schema-publisher \
  --signer-workflow sholdee/crd-schema-publisher/.github/workflows/release.yaml \
  --source-ref refs/heads/main
```

## Verify the Helm Chart

The chart is distributed as an OCI artifact and signed with cosign. Substitute the version you installed. You can find it with `helm list`.

```bash
cosign verify ghcr.io/sholdee/charts/crd-schema-publisher:<VERSION> \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp github.com/sholdee/crd-schema-publisher
```

## Verify the Container Image

Pre-built multi-arch images for amd64 and arm64 are published to GHCR:

```text
ghcr.io/sholdee/crd-schema-publisher:latest
```

Releases are triggered manually via the release workflow, producing a date-based tag such as `vYYYY.MDD.HMMSS` and `latest`. Release notes include the image digest, OCI Helm chart reference, signed checksum manifest, binary provenance link, and standalone binary attachments.

Images use `gcr.io/distroless/static:nonroot` as the runtime base. The image has no shell or package manager and runs as UID 65534. Production images are signed with cosign keyless signing via GitHub Actions OIDC:

```bash
cosign verify ghcr.io/sholdee/crd-schema-publisher:latest \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp github.com/sholdee/crd-schema-publisher
```
