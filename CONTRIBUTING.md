# Contributing

Thanks for your interest! Pull requests are welcome for bug fixes, improvements, and new features.

## Setup

```bash
# Requires Go (see go.mod for minimum version) and golangci-lint
go mod download
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Enable pre-commit hook
git config core.hooksPath .githooks
```

## Documentation Site

The public docs site lives under `docs-site/` and is built with Hugo.

```bash
${HUGO_BIN:-hugo} --source docs-site --destination /tmp/crd-schema-publisher-docs-site --cleanDestinationDir --minify
```

Run that command before opening a PR that changes `docs-site/`, `README.md`, or docs assets.

## Guidelines

- Follow [Conventional Commits](https://www.conventionalcommits.org/) for commit messages
- All Go tests must pass: `go test ./...`
- If you change theme JavaScript or tests, also run: `node --test theme/*.test.js`
- Linter must pass: `golangci-lint run`
- Run `go mod tidy` when changing dependencies
- All changes require a pull request to `main` -- direct pushes are blocked
- CI gate job must pass before merge
- GitHub Actions must be pinned by commit SHA with a version comment (e.g., `actions/checkout@<sha> # v4`)

## References

- See [SECURITY.md](SECURITY.md) for reporting vulnerabilities
- See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community standards
