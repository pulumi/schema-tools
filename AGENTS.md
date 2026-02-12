# Repository Guidelines

## Project Structure & Module Organization
- `main.go` wires the CLI; commands live under `internal/cmd/` (compare, stats, squeeze, version).
- Core logic sits in `internal/pkg/` (schema loading, stats, git helpers). Test fixtures like `internal/pkg/schema.json` live here.
- Shared utilities are in `internal/util/` (e.g., `set`, `diagtree`).
- Version metadata is defined in `version/`.
- Tests are colocated with code as `*_test.go` (e.g., `internal/cmd/compare_test.go`).

## Build, Test, and Development Commands
- `make build` — build the binary with version metadata from `git describe`.
- `make test` — run the full Go test suite (`go test ./...`).
- `make lint` — run `golangci-lint` across the repo.
- `make install` — install the binary with version metadata.
- `go build` / `go install` — local builds without Makefile flags.

## Coding Style & Naming Conventions
- Go formatting: use `gofmt` defaults (tabs for indentation).
- File naming follows Go norms (`snake_case_test.go` for tests).
- Command names map to CLI verbs (e.g., `compare`, `stats`, `squeeze`).

## Testing Guidelines
- Framework: standard Go `testing` package.
- Tests are colocated; name with `*_test.go` and table-driven patterns where appropriate.
- Run all tests via `make test`; target packages via `go test ./internal/pkg`.

## Commit & Pull Request Guidelines
- Commit messages are short, imperative, and topic-first (e.g., "Bump golang.org/x/net ...", "README and dependencies maintenance").
- Include PR numbers in merge commits when applicable (e.g., `(#72)`).
- PRs should include: a clear summary, rationale for schema impact, and sample CLI output when behavior changes.

## Security & Configuration Tips
- Version metadata is injected at build time via `-ldflags`; avoid hardcoding `version.Version`.
- When comparing schemas, prefer tagged versions or commits for reproducibility (e.g., `-o v5.41.0 -n v5.42.0`).
