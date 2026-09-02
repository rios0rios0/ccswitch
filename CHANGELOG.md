# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This file is not edited by hand. Every change writes its own fragment under
`.changes/unreleased/` with [chlog](https://github.com/luizjhonata/chlog), and a release compiles
the pending fragments into a version section here — so two branches each adding an entry no
longer touch the same lines, and a rebase that used to conflict on this file now conflicts on
nothing.

When a new release is proposed:

1. Create a new branch `bump/x.x.x` (this isn't a long-lived branch!!!);
2. The fragments pending under `.changes/unreleased/` are compiled into a version section by `chlog batch auto && chlog merge` (AutoBump does this for you — it reads the fragments directly);
3. Open a Pull Request with the bump version changes targeting the `main` branch;
4. When the Pull Request is merged, a new Git tag must be created.

Releases to productive environments should run from a tagged version.
Exceptions are acceptable depending on the circumstances (critical bug fixes that can be cherry-picked, etc.).

## [Unreleased]

## [0.7.0] - 2026-09-02

### Added

- added `ccswitch threshold [percent]`, which shows the rotation threshold or sets it. The value is persisted in the store, so a running monitor daemon picks it up on its next tick without being restarted, and it is applied at once: every account is repolled, the exhaustion markers the old threshold produced are rewritten, and the highest-priority account below the new threshold becomes active. `--reset` goes back to the built-in default

### Changed

- changed the Go version to `1.27.1` and updated all module dependencies
- made an exhaustion marker follow what the latest poll saw rather than persisting until its recorded reset time. An account whose limits reset early is released as soon as a poll shows it under the threshold, which is also what lets a threshold change take effect against fresh numbers
- made the utilization percentage the sole test for exhaustion. The server's `critical` severity no longer triggers rotation on its own: it is a display band rather than a ceiling -- reported from around 95% with `locked_reason` still null, i.e. while the account is perfectly usable -- so honouring it capped every threshold at the point the warning fires and made a threshold of 99 behave exactly like 90
- raised the default rotation threshold from 90% to 99%
- stopped baking `--threshold` into the detached daemon's command line unless it was named explicitly, so the daemon follows the stored threshold instead of treating its own startup value as an override

### Fixed

- advanced an account's poll timestamp on a failed attempt, not only a successful one. It is the backup poll cadence's sole input, so an account whose polls were failing was retried on every tick instead of every cadence -- and an endpoint answering `429` is exactly the condition the cadence exists to survive
- made `OAuthCredentials.Degraded` test the scope invariant it guards -- that the scopes name `user:inference` -- rather than the weaker "carries no scopes at all". A set narrowed to some other scope is just as fatal, is reached the same way, and was previously sticky: the repair path could not see it, and each refresh asked for the narrowed set again. A refresh now widens a degraded set back to Claude Code's full scope list, while a narrowing the endpoint explicitly reports is still recorded honestly and surfaced in the daemon log
- made the monitor poll every enrolled account rather than only the active one, so a backup's refresh token stays alive between rotations. Refresh tokens are rotated on every use and expire in weeks, so an account nobody touched went stale in the store and rotating to it installed a token the server had already forgotten
- made the monitor retry a switch that an earlier tick could only record. A rotation deferred because a `claude` session was running was never written again, so it landed only if the shell wrapper called `ensure`, and never at all without it
- made writing the credentials file a read-modify-write, so `mcpOAuth` and `designOauth` survive a rotation. Marshalling only `claudeAiOauth` over the file signed the user out of every authenticated MCP server on every switch -- the macOS keychain adapter already merged, the file adapter did not
- rejected a NaN rotation threshold. `strconv.ParseFloat` accepts the literal `NaN` and every comparison against it is false, so it passed the range check and, once stored, made `Percent >= threshold` false for every limit -- silently disabling rotation altogether
- stopped capturing the blank credentials Claude Code writes when a refresh answers `invalid_grant`. It empties `claudeAiOauth` in place rather than removing it, and capturing that replaced the account's last good tokens with the marker saying they were gone, then flagged the account long-lived so it was never polled or selected again
- stopped rebuilding credentials from the token-refresh response alone. The endpoint answers a refresh with the new token pair and little else, so the previous code wrote back a credential document with no `scopes`, no `subscriptionType` and no refresh-token expiry -- and Claude Code refuses to persist its own later refresh of a set whose scopes do not name `user:inference`, classifying it as not-claude.ai. The pair on disk then went stale, the refresh after that answered `invalid_grant`, and Claude Code blanked the credentials: the logout. A refresh now names the scopes it wants, reads `scope` and `refresh_token_expires_in` back, and merges onto the credentials it replaces. An account an earlier version already stripped is repaired on its next poll

## [0.6.0] - 2026-08-28

### Added

- added the Claude automated code review and `@claude` mention responder workflows, `claude-review.yaml` and `claude-mention.yaml`, matching the `reusable-claude-review.yaml` / `reusable-claude-mention.yaml` definitions they call in `rios0rios0/pipelines`, authenticating with the `CLAUDE_CODE_OAUTH_TOKEN` secret

### Fixed

- restored the `.changes/unreleased/` directory with a `.gitkeep`, so the release tooling keeps recognising this project as [chlog](https://github.com/luizjhonata/chlog)-based after a release consumes the last fragment. Git tracks files rather than directories, so the bump commit that removed the final fragment removed the directory too, and the next run read the empty `[Unreleased]` section as "nothing to release"
- restored the `id-token: write` permission on both Claude workflow callers. Without it the caller grants less than the reusable workflow declares, which GitHub rejects before the job starts -- runs ended in `startup_failure`. The action needs the scope because `setupGitHubToken()` exchanges a GitHub OIDC token for the GitHub App token it posts with, unless a `github_token` is passed explicitly.

### Removed

- removed the unused `id-token: write` permission from the Claude workflow callers, and changed `claude-review.yaml`'s display name to `Claude Review` so it matches its file name and its `Claude Mention` sibling. `anthropics/claude-code-action` needs `id-token: write` only for workload identity federation or the Bedrock / Vertex / Foundry OIDC paths; these authenticate with `claude_code_oauth_token`, so the scope allowed minting OIDC tokens for any audience without ever being used.

## [0.5.0] - 2026-08-26

### Added

- added a tailored `code-review` skill under `.github/skills/` so GitHub Copilot reviews changes against the [rios0rios0/guide](https://github.com/rios0rios0/guide/wiki) standards and this repository's own load-bearing invariants

### Changed

- changed the changelog to [chlog](https://github.com/luizjhonata/chlog) fragments: a change now writes its own YAML file under `.changes/unreleased/` through `chlog new --kind <Kind> --body "..."`, and `CHANGELOG.md` is GENERATED from them at release time by `chlog batch auto && chlog merge`. That is the one thing a single shared file cannot do — two branches each adding an entry no longer touch the same lines, so a rebase that used to conflict on `CHANGELOG.md` now conflicts on nothing. The `[Unreleased]` section was empty, so nothing had to be carried across. AutoBump already reads the fragments directly, so the release flow is unchanged.
- changed the Go module dependencies to their latest versions

### Fixed

- fixed the `main` pipeline, which every repository's `sast:gitleaks` job had been failing since the code-review skill landed: the skill's own security bullet listed credential prefixes verbatim to warn against writing them, and the scanner's second pass matches those prefixes on their own, so the warning tripped the rule it was describing. The bullet now names the vendors instead, and the commit that carried the original wording is allowlisted by fingerprint in `.gitleaksignore`, because the scan walks the whole history reachable from `HEAD` and no edit at the tip can clear a past commit. No credential was ever committed.

## [0.4.3] - 2026-08-24

### Changed

- changed the Go module dependencies to their latest versions
- changed the Go version to `1.27.0` and updated all module dependencies

## [0.4.2] - 2026-08-17

### Changed

- changed the Go module dependencies to their latest versions

## [0.4.1] - 2026-08-15

### Changed

- changed the Go version to `1.26.6` and updated all module dependencies

## [0.4.0] - 2026-08-12

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

