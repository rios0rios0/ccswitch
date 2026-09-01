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

- `internal/domain/entities` — pure types (`Account`, `Store`, `Settings`, `Usage`, `Limit`,
  `RotationState`, `Config`, `OAuthCredentials`, `AccountIdentity`).
- `internal/domain/commands` — one command per CLI verb (`enroll`, `list`, `status`, `use`,
  `rotate`, `ensure`, `threshold`, `monitor`), each constructed from repository ports.
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

- **A refresh must preserve the credential document, never rebuild it.** The token endpoint answers
  a refresh with the new pair, `expires_in`, `scope`, and (only when it rotated one)
  `refresh_token_expires_in` — it never restates `subscriptionType` or `rateLimitTier`. Claude Code
  will not persist its own later refresh of a credential set whose `scopes` do not name
  `user:inference`: it classifies it as not-claude.ai and drops it, so the pair on disk goes stale,
  the refresh after that answers `invalid_grant`, and Claude Code blanks the credentials. That is
  the logout. `TokensRepository.Refresh` therefore takes the whole `OAuthCredentials` — it has to
  name the scopes it wants in the request — and merges through
  `OAuthCredentials.WithRefreshed`, the same way Claude Code's own merge does.
- **Every enrolled account is polled, not only the active one.** Refresh tokens are rotated on every
  use and expire in weeks, so a backup nobody touches between rotations goes stale in the store and
  installing it hands Claude Code a token the server has forgotten. Backups poll on
  `backupPollInterval` rather than every tick — the usage endpoint rate-limits — but a backup whose
  token is expired or whose scopes are missing is attended to immediately, because that is exactly
  the account the next rotation installs.
- **Never capture blank credentials.** On `invalid_grant` Claude Code empties `claudeAiOauth` in
  place (`accessToken: ""`, `refreshToken: ""`, `expiresAt: 0`) instead of removing it. Capturing
  that overwrites the account's last good tokens with the marker saying they are gone and flips it
  to `LongLived`, so it is never polled or selected again. Guard every capture with
  `OAuthCredentials.Blank()`; a long-lived token still has an access token and is not blank.
- **Writing a credential store is a read-modify-write on both platforms.** `.credentials.json` holds
  `mcpOAuth` and `designOauth` beside `claudeAiOauth`, exactly as the macOS keychain item does.
  Marshalling only `claudeAiOauth` over it signs the user out of every authenticated MCP server on
  every rotation.
- **Exhaustion is decided by the utilization percentage alone.** The usage endpoint's `severity` is
  a display band, not a ceiling: it reads `critical` from around 95% while `locked_reason` is still
  null and the account is perfectly usable. Treating it as exhaustion capped every threshold at the
  point the warning fires, which made a threshold of 99 behave exactly like 90.
- **The threshold in force comes from the store unless `--threshold` was named.** `ccswitch
  threshold` persists it so the monitor — which reloads the store every tick — retunes without a
  restart, which is why `daemonArgs` must not bake `--threshold` into the detached daemon unless the
  caller passed it. Resolve it through `Config.ResolveThreshold(store.Settings)`, never by reading
  `Config.Threshold` directly.
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
  rotation. Merge into the stored document, refuse to write when it cannot be read or parsed, and read
  the item back to confirm the write: `security -i` silently truncates command lines over
  `securityStdinLimit` (4032), which is why payloads above it go through argv instead. Only
  `ErrKeychainItemNotFound` (exit status 44, `errSecItemNotFound`) means the item is absent and a write
  may start from an empty document — a locked keychain, a denied prompt or a timeout must abort the
  write, since inferring absence from "the read failed" erases exactly the `mcpOAuth` tokens this
  guards.
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

<!-- chlog:start -->
## Changelog (chlog) — MANDATORY

If the repository you are working in uses chlog (a `.chlog.yaml` or `.chlog.yml`
config file, or a `.changes/` directory, exists at the project root), the
following is binding and ALWAYS applies: whenever you make ANY change, you MUST
create a changelog fragment as part of the same change — automatically, without
being asked, before committing.

- Do NOT edit CHANGELOG.md directly; it is generated from fragments.
- Create the fragment with:
  `chlog new --kind <Kind> --body "<imperative description>"`
- Valid kinds: Added, Changed, Deprecated, Removed, Fixed, Security
- Choose the kind that best matches the change (e.g., new feature → Added,
  bug fix → Fixed, behavior change → Changed, removal → Removed, security fix → Security).
- If the change is backward-INCOMPATIBLE with the public API (a breaking
  change), you MUST add the `--breaking` flag:
  `chlog new --kind <Kind> --breaking --body "<description>"`.
  This is the ONLY thing that triggers a major version bump — the kind alone
  never does (per SemVer, major = incompatible change). When unsure whether a
  change breaks compatibility, ask the user instead of guessing.
- Fragments are YAML files in `.changes/unreleased/`; stage them with your commit.
- `chlog check` fails the build when a fragment is missing — never skip it.
<!-- chlog:end -->
