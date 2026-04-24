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

## Guidelines

- Follow [Conventional Commits](https://www.conventionalcommits.org/) for commit messages
- All Go tests must pass: `go test ./...`
- If you change `theme/schema_search.js` or `theme/schema_search.test.js`, also run: `node --test theme/schema_search.test.js`
- Linter must pass: `golangci-lint run`
- Run `go mod tidy` when changing dependencies
- All changes require a pull request to `main` -- direct pushes are blocked
- CI gate job must pass before merge
- GitHub Actions must be pinned by commit SHA with a version comment (e.g., `actions/checkout@<sha> # v4`)

## References

- See [SECURITY.md](SECURITY.md) for reporting vulnerabilities
- See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community standards
