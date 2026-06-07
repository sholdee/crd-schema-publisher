---
title: "Output Structure"
description: "Generated site layout, stable read paths, metadata, and convert cleanup behavior."
weight: 180
---

Cluster-backed site generation (`run`, `extract`, `watch`) and preview temp generations use this layout:

```text
<output-dir>/
  .generations/
    <generation>/
      <apigroup>/
        <kind>_<version>.json          # JSON schema
        <kind>_<version>.html          # Interactive documentation page
      _meta/
        kinds.json                     # Internal Kind casing manifest
        schema-metadata.json           # Internal index source manifest
      master-standalone/
        <apigroup>-<kind>-stable-<version>.json  # kubeval-compatible format
      index.html                       # Browsable schema index
      schema-search.js                 # Shared schema-page search/autocomplete module
      favicon.svg                      # Constellation icon
  current -> .generations/<generation> # Stable read path for sidecars and local servers
```

Direct-volume consumers should read or serve `OUTPUT_DIR/current`, not the flat root of `OUTPUT_DIR`. `_meta/` is internal tool state; first-party Cloudflare, git, S3, and Caddy examples exclude it from published or served output.

`convert` writes schema files directly into `--output-dir` instead of creating `.generations/current`. It records generated files in `_meta/convert-manifest.json` so reruns can remove stale generated artifacts while preserving files that existed before `convert` ran.
