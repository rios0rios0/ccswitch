# ccswitch — AI assistant instructions

## Purpose

`ccswitch` monitors Claude Code usage and rotates between enrolled backup Claude accounts when the
active account is exhausted. It swaps `~/.claude/.credentials.json` (the `claudeAiOauth` block) so
Claude Code authenticates as the next account on its next launch.

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
  `FileCredentialsRepository` (swaps `.credentials.json`, patches `oauthAccount` in `~/.claude.json`),
  `AnthropicUsageRepository` (`GET /api/oauth/usage`), `AnthropicTokensRepository` (OAuth refresh),
  `ProcSessionsRepository` (scans `/proc` for a running `claude`).
- `internal/infrastructure/controllers` — cobra wiring (`NewRootCommand`).
- `internal/infrastructure/services` — `DaemonService` (pidfile + detached self-exec).

## Invariants

- **Match accounts by identity, never by refresh token.** The server rotates the refresh token on
  every refresh, so `OAuthCredentials.SameAccountAs` is only a positive signal — a false result does
  not mean "different account". Use `Store.MatchAccount`, which resolves by `accountUuid`/`email`
  and falls back to the refresh token only when no identity is available. Getting this wrong pins
  the store to a rotated-away token (401 on every refresh) and makes `ensure` clobber good
  credentials with stale ones.
- **Long-lived tokens (`claude setup-token`) cannot be polled.** They are minted without the
  `user:profile` scope that `/api/oauth/usage` requires, so they return 403. Such accounts are
  flagged `LongLived` (and detected by an absent refresh token, which also covers stores written
  before the flag existed); `Account.SupportsUsagePolling` gates polling and automatic selection.

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
make test    # go test with -tags test,unit and integration
make sast    # security scanners
```

## Conventions

- Logging uses Logrus aliased as `logger`; user-facing output goes to `os.Stdout`/`os.Stderr` with a
  `[ccswitch]` prefix.
- Unit tests carry **no** build tag and live in external `_test` packages with `// given/when/then`
  blocks; integration tests use `//go:build integration`. No mocking library — use `test/doubles`.
- All persistence is atomic (temp file + rename) and owner-only (0600).
