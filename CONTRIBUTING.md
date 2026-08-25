# Contributing

Contributions are welcome. By participating, you agree to maintain a respectful and constructive environment.

For coding standards, testing patterns, architecture guidelines, commit conventions, and all
development practices, refer to the **[Development Guide](https://github.com/rios0rios0/guide/wiki)**.

## Prerequisites

- Go 1.26+
- [Make](https://www.gnu.org/software/make/)
- [chlog](https://github.com/luizjhonata/chlog) (`go install github.com/luizjhonata/chlog@latest`)

## Development Workflow

1. Fork and clone the repository
2. Create a branch: `git checkout -b feat/my-change`
3. Install dependencies:
   ```bash
   go mod download
   ```
4. Make your changes
5. Validate:
   ```bash
   make lint
   make test
   make sast
   make cross-compile   # required when touching platform-specific code
   ```
6. Add a changelog fragment — never edit `CHANGELOG.md`, which is generated from them:
   ```bash
   chlog new --kind Added --body "added the thing that was not there before"
   ```
7. Commit following the [commit conventions](https://github.com/rios0rios0/guide/wiki/Git-Flow)
8. Open a pull request against `main`

## Architecture Notes

- The domain layer (`internal/domain`) defines entities and repository ports and must not import
  infrastructure.
- Infrastructure adapters (`internal/infrastructure`) implement the ports over the filesystem and HTTP.
- Unit tests use hand-rolled in-memory doubles (`test/doubles`); no mocking library is used. HTTP
  adapters are tested against a real `httptest.NewServer`.
- OS-specific code belongs in a `_unix.go` / `_windows.go` pair behind a shared, portable function;
  nothing else may branch on the operating system. Binaries are released for Linux, macOS, and
  Windows on amd64 and arm64, and a Windows-only compile error breaks the entire release.
