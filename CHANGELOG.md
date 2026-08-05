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

- added Windows support. The daemon now detaches with `DETACHED_PROCESS` + `CREATE_NEW_PROCESS_GROUP` instead of `setsid`, liveness is probed by waiting on the process handle instead of sending signal 0, and running sessions are found through a ToolHelp32 snapshot instead of `/proc`. Only a natively installed `claude.exe` is detected — an npm installation runs inside `node.exe`, which the process list cannot distinguish from any other Node process
- added a cross-compile job to the workflow, type-checking all six released OS/arch pairs on every pull request. The shared `go-binary.yaml` does not expose the `cross_compile` input, so platform-specific breakage was reaching delivery unchecked
- added tests for `DaemonService`, which had none: pidfile lifecycle, liveness reporting, and the guard that keeps `Ensure` from starting a second daemon

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

