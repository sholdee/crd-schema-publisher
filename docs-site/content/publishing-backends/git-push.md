---
title: "Git Push"
description: "Commit generated schema changes to a git repository for static hosting."
weight: 120
---

The git-push sidecar pattern runs `crd-schema-publisher` in extract-only mode and periodically commits the active generated site to a git repository. It does not require Cloudflare credentials.

See the complete [git-push values file](https://github.com/sholdee/crd-schema-publisher/blob/main/examples/git-push/values.yaml).

## When to Use This

Use this when a git repository should be the publishing target for a static host. The example is written for GitHub, but the pattern works with GitLab, Gitea, Bitbucket, or another git host by changing the remote URL format in the sidecar command.

No persistence is needed. The publisher re-extracts on startup, and the sidecar re-syncs the full generated site from the default `emptyDir`.

## GitHub Pages

This works well with GitHub Pages. Set the target repository's Pages source to the `gh-pages` branch, and the pushed schema site is published as static content.

The sidecar handles fresh repositories by initializing the target branch and existing repositories by cloning the configured branch before applying incremental updates.

If your GitHub Pages site serves from a project subpath such as `https://user.github.io/repo/`, set `config.basePath` to the repository path so generated HTML links include the prefix.

## Install

```bash
helm install crd-schema-publisher oci://ghcr.io/sholdee/charts/crd-schema-publisher \
  --namespace crd-schema-publisher --create-namespace \
  -f examples/git-push/values.yaml
```

## Credentials

Create a fine-grained GitHub personal access token scoped to the target repository with `contents:write` permission. Store it in the Secret key `github-token`, and store the target repository as `owner/repo` in `github-repo`.

The example sets `GIT_BRANCH` to `gh-pages`. Change it only if your static host publishes from a different branch.

The sidecar waits for `/data/current/index.html` before changing the repository. If that path never appears, it stays fail-closed and does not rewrite the target repository.

## Resource Sizing

Memory usage scales with CRD count because the initial git commit stages all files at once. `256Mi` handles about `200` CRDs; increase the limit for larger clusters.

The default loop sleeps for 30 seconds between sync attempts. It can be lowered to 15 seconds safely; no-op cycles are near-zero cost.
