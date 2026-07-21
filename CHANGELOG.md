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

### Changed

- changed the Go version to `1.26.5` and updated all module dependencies

### Added

- added initial `ccswitch` CLI that monitors Claude Code usage and rotates between backup accounts
- added `enroll` command that captures the logged-in Claude account (credentials + identity) into a local store
- added `list`, `status`, `use`, and `rotate` commands for inspecting and switching accounts
- added `ensure` command as a no-network pre-launch guard that installs the current account's credentials
- added `monitor` daemon that polls the Claude usage endpoint and rotates on exhaustion, with `--ensure-daemon` self-start
- added Anthropic usage client (`GET /api/oauth/usage`) and OAuth refresh client for polling backup accounts
- added atomic, owner-only (0600) persistence for the account store and credentials swaps
- added a warning when `ANTHROPIC_API_KEY` / `ANTHROPIC_AUTH_TOKEN` would shadow the rotated OAuth credentials
