# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`ccswitch` is a Go CLI + daemon that watches Claude Code usage and rotates the `claudeAiOauth` block
of Claude Code's credential store — `~/.claude/.credentials.json` on Linux/Windows, the login
keychain on macOS — between enrolled backup accounts when the active one is exhausted. The next
`claude` launch then authenticates as the swapped-in account. See `README.md` for user-facing
behavior, flags, and shell integration.

## Commands

- `make build` — build to `bin/ccswitch`.
- `make test` — unit + integration tests.
- `make lint` — golangci-lint (strict; config comes from the external `rios0rios0/pipelines` makefiles).
- `make sast` — security scanners.
- `make cross-compile` — `go vet` for all six released OS/arch targets; run it after touching
  anything platform-specific.
- `make run` — `go run ./cmd/ccswitch`.

Run a single test with the standard toolchain, e.g.
`go test ./internal/domain/entities/ -run TestStoreMatchAccount`.

## Architecture

Clean Architecture; the domain layer must never import infrastructure.

- `internal/domain/entities` — pure types (`Account`, `Store`, `Usage`, `Limit`, `RotationState`,
  `Config`, `OAuthCredentials`, `AccountIdentity`).
- `internal/domain/commands` — one command per CLI verb (`enroll`, `list`, `status`, `use`,
  `rotate`, `ensure`, `monitor`), each constructed from repository ports.
- `internal/domain/repositories` — ports (`Accounts`, `Credentials`, `Usage`, `Tokens`, `Sessions`).
- `internal/infrastructure/repositories` — adapters: JSON store, the credentials swappers
  (`FileCredentialsRepository` for `.credentials.json`, `KeychainCredentialsRepository` for the macOS
  login keychain; both patch `oauthAccount` in `~/.claude.json` through the shared
  `claudeStateFile`), HTTP usage/refresh clients, and the session probes (`ProcSessionsRepository`
  scanning `/proc`, `PSSessionsRepository` reading the macOS process table,
  `ToolhelpSessionsRepository` walking a Windows ToolHelp32 snapshot).
- `internal/infrastructure/controllers` — cobra wiring (`NewRootCommand`).
- `internal/infrastructure/services` — `DaemonService` (pidfile + detached self-exec).

Platform differences are isolated in `_unix.go` / `_darwin.go` / `_windows.go` pairs —
`detachAttrs`/`processAlive` in `services`, the session and credentials adapters in `repositories`,
and `newSessionsRepository`/`newCredentialsRepository` (which pick one) in `controllers`. Nothing
else branches on the OS; keep it that way.

## Invariants (get these wrong and rotation breaks silently)

- **Match accounts by identity, never by refresh token.** The server rotates the refresh token on
  every refresh, so a token match is a positive signal only — a non-match does *not* mean "different
  account". Resolve through `Store.MatchAccount` (matches on `accountUuid`/`email`, falling back to
  the token only when no identity is known). `OAuthCredentials.SameAccountAs` is the token-only
  comparison; do not use it to *reject* a match. Getting this wrong pins the store to a rotated-away
  token (401 on every refresh) and makes `ensure` clobber good credentials with stale ones.
- **Six targets are released, so platform code must compile for all of them.** `make cross-compile`
  type-checks linux/darwin/windows × amd64/arm64, and the workflow runs the same matrix on every
  pull request. Skipping it is how `Setsid` — which does not exist on Windows — reached delivery and
  left every release up to 0.2.2 with zero published binaries.
- **On macOS the keychain is the only credential store that matters, and writing it is a
  read-modify-write.** Claude Code uses a `keychain-with-plaintext-fallback` store: the generic-password
  item `Claude Code-credentials` wins whenever it is readable, and `~/.claude/.credentials.json` is
  read only when the keychain returns nothing — so writing that file is a no-op there. The item is one
  JSON document holding `claudeAiOauth` **and** `mcpOAuth`, the OAuth tokens of every authenticated MCP
  server; marshalling just `claudeAiOauth` over it signs the user out of every MCP server on every
  rotation. Merge into the stored document, refuse to write when it cannot be parsed, and read the item
  back to confirm the write: `security -i` silently truncates command lines over `securityStdinLimit`
  (4032), which is why payloads above it go through argv instead.
- **The Claude desktop app is not a Claude Code session.** It runs as
  `Claude.app/Contents/MacOS/Claude`, whose base name matches the CLI's under `matchesClaudeProcess`'s
  case-insensitive compare. Counting it would make `ClaudeRunning()` permanently true and silently
  disable rotation for anyone who keeps the desktop app open, so `PSSessionsRepository` skips
  executables inside `.app` bundles.
- **Long-lived tokens cannot be polled.** Tokens from `claude setup-token` lack the `user:profile`
  scope that `GET /api/oauth/usage` requires (403), so such accounts are flagged `LongLived` and
  gated by `Account.SupportsUsagePolling`. Never poll them or select them automatically — they are a
  manual fallback (`ccswitch use <email>`).

## Conventions

- All persistence is atomic (temp file + rename) and owner-only (0600).
- Logrus is imported aliased as `logger`; user-facing text goes to stdout/stderr with a `[ccswitch]`
  prefix.
- Tests live in external `_test` packages, structure bodies with `// given` / `// when` / `// then`
  blocks, and rely on hand-rolled doubles in `test/doubles` — no mocking library. HTTP adapters are
  tested against a real `httptest.NewServer`.

See `.github/copilot-instructions.md` for the same guidance framed for GitHub Copilot.
