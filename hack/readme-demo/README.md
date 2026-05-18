# README Demo GIF

These scripts generate `docs/screenshots/overview.gif`.

The capture script expects a rendered site to be running locally. For a real
cluster-backed capture:

```bash
demo_root=/private/tmp/crd-schema-publisher-readme-demo
mkdir -p "${demo_root}/output"
go run ./cmd/ extract -o "${demo_root}/output"
PREVIEW_ADDR=127.0.0.1:8989 go run ./cmd/ preview -o "${demo_root}/output"
```

In another shell:

```bash
PREVIEW_URL=http://127.0.0.1:8989 node hack/readme-demo/capture.mjs
swift hack/readme-demo/make-gif.swift /private/tmp/crd-demo-frames/frames.tsv docs/screenshots/overview.gif
```

Useful environment variables:

- `PREVIEW_URL`: local preview URL. Defaults to `http://127.0.0.1:8989`.
- `FRAME_DIR`: PNG frame output directory. Defaults to `/private/tmp/crd-demo-frames`.
- `CHROME_PROFILE_DIR`: temporary Chrome profile. Defaults to `/private/tmp/crd-demo-chrome-profile`.
- `CHROME_PATH`: Chrome executable path. Defaults to macOS Google Chrome.
- `SCHEMA_COUNT`: count shown in the terminal extract stage. Defaults to `191`.
