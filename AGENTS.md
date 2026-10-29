# Agents working in this repo

A command that converts a claude.ai data export into one Markdown file per conversation. Public.

## Build and test

```sh
go build ./...
go test -race ./...
```

CI (`.github/workflows/ci.yml`) runs the shared `go` workflow from `troyronda/.github` (at `@main`) on every pull request: gofmt, a `go mod tidy` diff, `go vet`, staticcheck, govulncheck and the race tests.

## Layout

- One module at the repo root. The command is in `cmd/claude-export-md/`; everything else is in `internal/` (`export` reads exports, `render` writes Markdown, `state` records written files, `importer` runs an import). No `pkg/`.
- Package names are short, lowercase, singular nouns. Don't create a package before it has code.
- `go.work` is never committed; CI builds from `go.mod`.

## Rules

- Test fixtures are made-up conversations only, never copies of a real export.
- No secrets in the repo, in tests or in logs. Output and errors never include conversation titles or text.
- Dependencies: the standard library, official SDKs, or widely used and maintained libraries, pinned in `go.mod`. No dependency on small single-maintainer repositories.
- Tags stay at `v0.x`, are made only after every check passes, and never move.
