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

- added `--token`/`--email` flags to `enroll`, letting a long-lived token minted by `claude setup-token` be enrolled directly without an interactive `/login`

### Changed

- changed long-lived token accounts to be treated as a manual fallback: their usage cannot be read (`claude setup-token` mints a token without the `user:profile` scope, so the usage endpoint returns `403`), so they are never polled and never selected automatically, and `list`/`status` now say so instead of reporting a failed poll

### Fixed

- fixed enrolled accounts being matched to the credentials on disk by their refresh token, which the server rotates on every refresh: once Claude Code refreshed, the account stopped being recognized and the store stayed pinned to a refresh token that had been rotated away, failing every later refresh with `401`
- fixed `ensure` overwriting freshly refreshed credentials with the stale stored ones as a consequence of that mismatch, which could install a dead token on every `claude` launch
- fixed `pollUsage` silently swallowing a failed token refresh and then polling the usage endpoint with the known-stale access token, which surfaced as a misleading `401` instead of the real refresh failure

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

