# para

A TUI note browser in Go with Bubble Tea, modeled on `~/Git/lazytask`.
There is no plan file. `README.md`, the code and its tests record the current
behavior; read them before changing behavior. `docs/import-qa.md` is the manual
check for the import and `docs/image-qa.md` the one for images.

## Commands

- Go 1.25 (`go.mod` declares 1.25.0, which `modernc.org/sqlite` requires).
- Build: `make build` (writes the `para` binary, which is git-ignored).
- Install: `make install` (builds, then copies the binary to `~/bin`, which must
  be on `PATH`).
- Test: `make test` (`go test ./...`).
- Both `go build ./...` and `go test ./...` must pass before a commit, and
  after a commit that adds files, from a clean clone (a bad `.gitignore` once
  hid `cmd/para/` from every commit).

## Testing policy

Tests cover engine packages only (`internal/store`, `internal/state`,
`internal/undo`, `internal/config`, `internal/intake`, `internal/cli`,
`internal/digest`, `internal/loader`, `internal/clip`, `internal/templates`, `internal/imgfmt`, `internal/imgpreview`, `internal/attach`, the link index). Use table-driven tests
against temp directories; never touch a real notebook or the real state
database. UI packages (`cmd/para`, `internal/ui/*`) have no automated
tests; each UI chunk ships a manual QA note instead. This deliberately
deviates from the organization 80% coverage policy (project owner decision).

## Workflow

- Work on a feature branch, never on `main`.
- One chunk per commit, with a human checkpoint between chunks.
- Agent instructions live in this file; never create a `CLAUDE.md`.
- Verify a dependency exists and is maintained before adding it to `go.mod`.
