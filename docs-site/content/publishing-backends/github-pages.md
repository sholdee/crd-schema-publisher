---
title: "GitHub Pages Subpaths"
description: "Set a base path when serving generated schema sites from GitHub Pages project paths."
weight: 130
---

When serving schemas from a GitHub Pages project path such as `https://user.github.io/iac/`, set the base path so generated HTML links include the subpath.

```bash
BASE_PATH=/iac
```

Or in the Helm chart:

```yaml
config:
  basePath: "/iac"
```

Without this setting, generated links resolve against the host root and break on project Pages sites.

Cloudflare Pages and root hosts do not need this setting because they serve from `/`. Cloudflare Pages users do not need to change anything for subpath handling.

If you already use the first-party `git-push` or `rclone-s3` examples, update them to read `/data/current`. Older example configs fail closed after upgrading to a new image: syncing stops, but existing remote content is not deleted or overwritten.
