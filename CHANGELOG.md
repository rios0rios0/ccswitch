# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

When a new release is proposed:

1. Create a new branch `bump/x.x.x` (this isn't a long-lived branch!!!);
2. The Unreleased section on `CHANGELOG.md` gets a version number and date;
3. Open a Pull Request with the bump version changes targeting the `main` branch;
4. When the Pull Request is merged, a new Git tag must be created.

Releases to productive environments should run from a tagged version.
Exceptions are acceptable depending on the circumstances (critical bug fixes that can be cherry-picked, etc.).

## [Unreleased]

### Added

- added macOS support. Claude Code on macOS uses a `keychain-with-plaintext-fallback` credential store, in which the generic-password item `Claude Code-credentials` wins whenever it is readable and `~/.claude/.credentials.json` is consulted only when the keychain returns nothing. Rewriting that file was therefore a no-op on macOS: `ccswitch enroll` failed outright on machines where it does not exist, taking every other command with it. Credentials are now read and written through the keychain, and session detection reads the process table instead of `/proc`

### Fixed

- fixed `list` and `status` destroying the accounts they polled. Both discarded the credentials `pollUsage` returned, and because the server rotates the refresh token on every refresh and invalidates the previous one, a single `ccswitch list` over an account whose access token had expired left the store pinned to a refresh token that no longer existed. That account was then unreadable for good and had to be enrolled again. Both commands now write the refreshed pair back to the store and, when the credentials store still holds exactly the pair the refresh consumed, publish it there too — the same guard the monitor already applied, now shared between all three
- fixed an account staying unreadable after its token was invalidated ahead of the recorded expiry. A refresh was attempted only once `expiresAt` had passed, but the server invalidates tokens on its own schedule and a fresh login supersedes the pair ccswitch captured earlier, so a token rejected while its timestamp still looked valid was never renewed — the account reported `usage unavailable` on every poll until it was enrolled again. A rejected token is now reported as `ErrUnauthorized` and refreshed once before the poll gives up; other failures, including rate limiting, still do not spend the refresh token
- fixed rotation being able to erase every MCP server login on macOS. The keychain item is a single JSON document holding both `claudeAiOauth` and `mcpOAuth`, so installing an account by marshalling only `claudeAiOauth` over it would have signed the user out of every authenticated MCP server. The swap is now a read-modify-write that preserves unknown keys verbatim, refuses to write when the stored document cannot be read or parsed — only a genuine "no such item" is treated as absence, so a locked keychain or a denied access prompt no longer looks like a fresh install — and reads the item back before reporting success — `security -i` truncates command lines over 4032 characters without a reliable error, and the Claude Code document exceeds that as soon as any MCP server is authenticated
- fixed the Claude desktop app suppressing rotation on macOS. It runs as `Claude.app/Contents/MacOS/Claude`, whose base name matches the CLI's under the case-insensitive process comparison, so keeping the desktop app open would have held `ClaudeRunning()` permanently true and silently disabled rotation. Executables inside `.app` bundles are no longer counted as sessions

## [0.3.0] - 2026-08-05

### Added

- added a cross-compile job to the workflow, type-checking all six released OS/arch pairs on every pull request. The shared `go-binary.yaml` does not expose the `cross_compile` input, so platform-specific breakage was reaching delivery unchecked
- added tests for `DaemonService`, which had none: pidfile lifecycle, liveness reporting, and the guard that keeps `Ensure` from starting a second daemon
- added Windows support. The daemon now detaches with `DETACHED_PROCESS` + `CREATE_NEW_PROCESS_GROUP` instead of `setsid`, liveness is probed by waiting on the process handle instead of sending signal 0, and running sessions are found through a ToolHelp32 snapshot instead of `/proc`. Only a natively installed `claude.exe` is detected — an npm installation runs inside `node.exe`, which the process list cannot distinguish from any other Node process

### Fixed

- fixed the release publishing no binaries at all. `syscall.SysProcAttr.Setsid` does not exist on Windows, so GoReleaser's Windows build failed to compile and aborted the whole release — every version from `0.1.0` to `0.2.2` shipped zero assets, leaving `install.sh` with nothing to download

## [0.2.2] - 2026-08-04

### Changed

- refreshed `.github/copilot-instructions.md` to drop the stale `//go:build integration` test-tag claim, since the codebase carries no build tags, and to note the `httptest`-backed HTTP adapter tests

## [0.2.1] - 2026-07-28

### Fixed

- fixed a successful refresh being discarded when the usage call that follows it failed (for example on a transient network error). The refresh had already rotated the token server-side, so dropping its result left both the store and the credentials file on an invalidated token, failing every later refresh with `invalid_grant`
- fixed the monitor logging idle Claude Code sessions out: refreshing an expired access token rotates the refresh token and invalidates the previous one, but the new pair was kept only in the ccswitch store, leaving `~/.claude/.credentials.json` holding a token the server had already killed. Claude Code's next refresh then failed with `invalid_grant` and the session was logged out. Because a refresh only happens once the access token has expired, this hit sessions left idle. The refreshed pair is now written back to the credentials file whenever that file still holds the pair the refresh consumed

## [0.2.0] - 2026-07-27

### Added

- added `--token`/`--email` flags to `enroll`, letting a long-lived token minted by `claude setup-token` be enrolled directly without an interactive `/login`
- added `CLAUDE.md` with Claude Code guidance mirroring the existing `.github/copilot-instructions.md`

### Changed

- changed long-lived token accounts to be treated as a manual fallback: their usage cannot be read (`claude setup-token` mints a token without the `user:profile` scope, so the usage endpoint returns `403`), so they are never polled and never selected automatically, and `list`/`status` now say so instead of reporting a failed poll

### Fixed

- fixed `ensure` overwriting freshly refreshed credentials with the stale stored ones as a consequence of that mismatch, which could install a dead token on every `claude` launch
- fixed `pollUsage` silently swallowing a failed token refresh and then polling the usage endpoint with the known-stale access token, which surfaced as a misleading `401` instead of the real refresh failure
- fixed enrolled accounts being matched to the credentials on disk by their refresh token, which the server rotates on every refresh: once Claude Code refreshed, the account stopped being recognized and the store stayed pinned to a refresh token that had been rotated away, failing every later refresh with `401`

## [0.1.0] - 2026-07-22

### Added

- added `--prefer-primary` (default `true`), which keeps the monitor on the highest-priority account that has capacity and switches back to the primary as soon as its limits reset; `--prefer-primary=false` restores round-robin
- added `enroll` command that captures the logged-in Claude account (credentials + identity) into a local store
- added `ensure` command as a no-network pre-launch guard that installs the current account's credentials
- added `list`, `status`, `use`, and `rotate` commands for inspecting and switching accounts
- added `monitor` daemon that polls the Claude usage endpoint and rotates on exhaustion, with `--ensure-daemon` self-start
- added a warning when `ANTHROPIC_API_KEY` / `ANTHROPIC_AUTH_TOKEN` would shadow the rotated OAuth credentials
- added Anthropic usage client (`GET /api/oauth/usage`) and OAuth refresh client for polling backup accounts
- added atomic, owner-only (0600) persistence for the account store and credentials swaps
- added initial `ccswitch` CLI that monitors Claude Code usage and rotates between backup accounts

### Changed

- changed the Go version to `1.26.5` and updated all module dependencies

### Fixed

- fixed exhausted accounts being released after the soonest limit reset rather than the longest, which let an account be reselected while a longer window (such as the weekly limit) was still saturated

