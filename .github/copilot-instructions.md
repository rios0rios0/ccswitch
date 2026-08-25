# ccswitch — AI assistant instructions

## Purpose

`ccswitch` monitors Claude Code usage and rotates between enrolled backup Claude accounts when the
active account is exhausted. It swaps the `claudeAiOauth` block of Claude Code's credential store —
`~/.claude/.credentials.json` on Linux/Windows, the login keychain on macOS — so Claude Code
authenticates as the next account on its next launch.

## Architecture

Clean Architecture with a strict domain / infrastructure split:

- `internal/domain/entities` — pure model: `Account`, `Store`, `Usage`, `Limit`, `RotationState`,
  `Config`, `OAuthCredentials`, `AccountIdentity`. No infrastructure imports.
- `internal/domain/commands` — application logic: `EnrollAccountCommand`, `ListAccountsCommand`,
  `StatusCommand`, `UseAccountCommand`, `RotateAccountCommand`, `EnsureActiveCommand`,
  `MonitorCommand`. Each takes ports and exposes `Execute` (or `Run`/`Tick` for the daemon).
- `internal/domain/repositories` — ports: `AccountsRepository`, `CredentialsRepository`,
  `UsageRepository`, `TokensRepository`, `SessionsRepository`.
- `internal/infrastructure/repositories` — adapters: `JSONAccountsRepository` (atomic 0600 store),
  `FileCredentialsRepository` (swaps `.credentials.json`), `KeychainCredentialsRepository` (swaps the
  macOS `Claude Code-credentials` keychain item via `security`) — both patch `oauthAccount` in
  `~/.claude.json` through the shared `claudeStateFile` — `AnthropicUsageRepository`
  (`GET /api/oauth/usage`), `AnthropicTokensRepository` (OAuth refresh), `ProcSessionsRepository`
  (scans `/proc` for a running `claude`), `PSSessionsRepository` (reads the macOS process table),
  `ToolhelpSessionsRepository` (walks a Windows ToolHelp32 snapshot for `claude.exe`).
- `internal/infrastructure/controllers` — cobra wiring (`NewRootCommand`).
- `internal/infrastructure/services` — `DaemonService` (pidfile + detached self-exec).

## Invariants

- **Match accounts by identity, never by refresh token.** The server rotates the refresh token on
  every refresh, so `OAuthCredentials.SameAccountAs` is only a positive signal — a false result does
  not mean "different account". Use `Store.MatchAccount`, which resolves by `accountUuid`/`email`
  and falls back to the refresh token only when no identity is available. Getting this wrong pins
  the store to a rotated-away token (401 on every refresh) and makes `ensure` clobber good
  credentials with stale ones.
- **Six targets are released, so platform code must compile for all of them.** OS-specific code
  lives in `_unix.go` / `_darwin.go` / `_windows.go` pairs (`detachAttrs`/`processAlive` in
  `services`, the session and credentials adapters in `repositories`,
  `newSessionsRepository`/`newCredentialsRepository` in `controllers`); nothing else branches on the
  OS. `make cross-compile` type-checks linux/darwin/windows × amd64/arm64 and the workflow runs the
  same matrix per pull request — skipping it is how `Setsid` (absent on Windows) reached delivery
  and left every release up to 0.2.2 with zero published binaries. Note that `make lint` only covers
  the host OS, so run it with `GOOS=darwin` / `GOOS=windows` after touching platform-specific code.
- **On macOS the keychain is the only credential store that matters, and writing it is a
  read-modify-write.** The generic-password item `Claude Code-credentials` wins whenever it is
  readable and `.credentials.json` is consulted only when the keychain returns nothing, so writing
  that file is a no-op there. The item is one JSON document holding `claudeAiOauth` **and**
  `mcpOAuth` (every authenticated MCP server's tokens): merge into the stored document, refuse to
  write when it cannot be read or parsed, and read the item back to confirm — `security -i` silently
  truncates command lines over `securityStdinLimit` (4032), which is why larger payloads go through
  argv. Only `ErrKeychainItemNotFound` (exit status 44) means absence; treating any failed read as
  absence erases the `mcpOAuth` tokens this guards.
- **The Claude desktop app is not a Claude Code session.** It runs as
  `Claude.app/Contents/MacOS/Claude`, whose base name matches the CLI's under the case-insensitive
  `matchesClaudeProcess` compare, so `PSSessionsRepository` skips executables inside `.app` bundles.
  Counting it would hold `ClaudeRunning()` permanently true and silently disable rotation.
- **Long-lived tokens (`claude setup-token`) cannot be polled.** They are minted without the
  `user:profile` scope that `/api/oauth/usage` requires, so they return 403. Such accounts are
  flagged `LongLived` (and detected by an absent refresh token, which also covers stores written
  before the flag existed); `Account.SupportsUsagePolling` gates polling and automatic selection.
- **A refresh must be published back to the credential store.** ccswitch and Claude Code share one
  refresh token, and the server rotates it on every refresh — whoever refreshes second with the old
  token gets `invalid_grant`. Keeping a refreshed pair only in the ccswitch store therefore logs
  Claude Code out. The shared `publishRefreshed` helper writes it back whenever the credential store
  still holds the pair the refresh consumed; that guard is what keeps it from overwriting a different
  account's credentials. It is used by `monitor`, `list` and `status` alike — every command that
  polls usage can spend the refresh token.

## Key external contracts

- Usage: `GET https://api.anthropic.com/api/oauth/usage` with `Authorization: Bearer <accessToken>`
  and `anthropic-beta: oauth-2025-04-20`. Returns `five_hour`/`seven_day` utilization (0-100) and a
  `limits[]` array with `percent`/`severity`/`is_active`/`resets_at`.
- Refresh: `POST https://platform.claude.com/v1/oauth/token` with `grant_type=refresh_token`, the
  refresh token, and Claude Code's public `client_id`.

## Commands

```bash
make build   # build to bin/ccswitch
make lint    # golangci-lint (very strict; see pipelines .golangci.yml)
make test    # unit + integration tests (single test: go test ./internal/domain/entities/ -run TestStoreMatchAccount)
make sast    # security scanners
make cross-compile  # go vet for all six released OS/arch targets
```

## Conventions

- Logging uses Logrus aliased as `logger`; user-facing output goes to `os.Stdout`/`os.Stderr` with a
  `[ccswitch]` prefix.
- Tests live in external `_test` packages with `// given/when/then` blocks and carry no build tags.
  No mocking library — use `test/doubles`; HTTP adapters are tested against a real `httptest.NewServer`.
- All persistence is atomic (temp file + rename) and owner-only (0600).

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
