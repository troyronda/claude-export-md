# Agents working in this repo

This is a public Go module. Everything pushed here is public from the first push, and a tagged version is cached by the Go module proxy permanently.

## Data

- Conversation exports hold private text. Process them only with local tools or Claude; never send export text to any other hosted service.
- Never put conversation text in logs, test output, error messages or anything that leaves the machine. `--dry-run` and reports print counts only, never titles or text.
- Test fixtures are made-up conversations only, never copies of a real export.
- Nothing in this repo names a person's machine paths, Keychain items or private setup.
- The tool never moves or changes the export it reads.

## Git

- Work on branches and open pull requests; the maintainer merges.
- No attribution anywhere: no `Co-Authored-By` trailers, no "Generated with" lines, no session links.
- Enable the hooks once: `git config core.hooksPath .githooks`. The pre-push hook runs the private-terms check and gitleaks and fails closed.
- Tags stay at `v0.x`, and a tag is made only after every check passes.

## Go

- One module at the repo root. Commands in `cmd/<name>/`, everything else in `internal/` until another module needs it; no `pkg/`.
- Package names are short, lowercase, singular nouns named for what they provide (no `util`, `common`, `tools` or plurals). Don't create a package before it has code.
- CI runs on every pull request: `gofmt`, `go vet`, `staticcheck`, `govulncheck`, `go test -race ./...`, a `go mod tidy` diff check, the private-terms check and gitleaks.
- `go.work` is never committed; CI builds from `go.mod`.
