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
- `internal/domain/commands` — one command per CLI verb (`enroll`, `list`, `show`, `update`,
  `remove`, `reorder`, `status`, `use`, `rotate`, `ensure`, `threshold`, `monitor`, `self-update`),
  each constructed from repository ports. `version` is printed by the controller alone.
- `internal/domain/repositories` — ports (`Accounts`, `Credentials`, `Usage`, `Tokens`, `Sessions`,
  `SelfUpdate`).
- `internal/infrastructure/repositories` — adapters: JSON store, the credentials swappers
  (`FileCredentialsRepository` for `.credentials.json`, `KeychainCredentialsRepository` for the macOS
  login keychain; both patch `oauthAccount` in `~/.claude.json` through the shared
  `claudeStateFile`), HTTP usage/refresh clients, the session probes (`ProcSessionsRepository`
  scanning `/proc`, `PSSessionsRepository` reading the macOS process table,
  `ToolhelpSessionsRepository` walking a Windows ToolHelp32 snapshot), and
  `CliforgeSelfUpdateRepository`, which checks for and installs GitHub releases through the shared
  [cliforge](https://github.com/rios0rios0/cliforge) library.
- `internal/infrastructure/controllers` — cobra wiring (`NewRootCommand`), including the root's
  `--verbose`/`--version` flags and the passive update check its `PersistentPreRun` runs.
- `internal/infrastructure/services` — `DaemonService` (pidfile + detached self-exec).

Platform differences are isolated in `_unix.go` / `_darwin.go` / `_windows.go` pairs —
`detachAttrs`/`processAlive` in `services`, the session and credentials adapters in `repositories`,
and `newSessionsRepository`/`newCredentialsRepository` (which pick one) and `consoleStyle` (which
switches a Windows console into virtual terminal processing so it renders colors) in `controllers`.
Nothing else branches on the OS; keep it that way.

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
- **A refresh must be published back to the credential store.** ccswitch and Claude Code share one
  refresh token, and the server rotates it on every refresh — whoever refreshes second with the old
  token gets `invalid_grant` and is logged out. Keeping a refreshed pair only in the ccswitch store
  therefore logs Claude Code out, and because a refresh only fires once the access token is spent it
  bites idle sessions first. `publishRefreshed` writes the new pair back only while the store still
  holds exactly the pair the refresh consumed; that guard is what stops it clobbering a different
  account's credentials, and it is why publishing the same account's newer tokens is safe even while
  a session runs (unlike an account switch, which the running-session guard in `switchTo` blocks).
  Every command that polls usage — `monitor`, `list`, `status` — can spend the refresh token, so
  every one of them publishes.
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
  left every release up to 0.2.2 with zero published binaries. The workflow also runs the whole test
  suite on Windows (`tests > test:windows`), so a test asserting something Windows does not have,
  such as Unix mode bits, belongs in a `_unix_test.go` file behind `//go:build !windows`.
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
- **Plain output is what pipes and scripts read, so it never changes with the styling.** `list`
  dresses itself up only as `Config.Output` says, which `PersistentPreRun` resolves through
  `outputStyle`: `--color` first, then `FORCE_COLOR` (forces, or drops colors at `0`/`false`), then
  `NO_COLOR` (drops), then `CLICOLOR_FORCE` (forces), then `CLICOLOR=0` and `TERM=dumb` (drop), then
  whether stdout is a character device. That order is each variable's own specification:
  force-color.org and Node.js rank `FORCE_COLOR` over `NO_COLOR`, and bixense ranks `NO_COLOR` over
  `CLICOLOR_FORCE`. Rendering goes through `palette`, whose zero value is the plain style: `paint`
  returns text untouched and `readingLine` prints the pre-styling line, so an undecorated listing stays
  byte-for-byte what it was. Pad text before painting it, since `fmt` widths count the escape codes, and
  color a figure through `Reading.Spent`, the same test that exhausts an active limit, never a second
  copy of it. The decorated figure and meter round down, as Claude Code's `/usage` does, so a figure
  never reads as the threshold while it is still yellow.
- **Long-lived tokens cannot be polled.** Tokens from `claude setup-token` lack the `user:profile`
  scope that `GET /api/oauth/usage` requires (403), so such accounts are flagged `LongLived` and
  gated by `Account.SupportsUsagePolling`. Never poll them or select them automatically — they are a
  manual fallback (`ccswitch use <email>`). That includes handing over from a removed or missing
  current account: go through `Store.Successor`, never "the first account in order", which may be one.
- **Priority is `Account.Order`, and every change keeps it contiguous from 0.** `Store.Move`,
  `Store.Reorder` and `Store.Remove` renumber the order in place — pointers into `Store.Accounts` keep
  naming the same account — while the CLI counts positions from 1 for the primary (`Store.Position`,
  `entities.ParsePriority`). Reordering touches no credentials: the monitor reloads the store every
  tick and `selectTarget` compares `Order` values, which is also why they must never tie.
- **The passive update check stays off the commands nobody watches.** cliforge counts a day as
  checked only once a background lookup has answered, and starts at most five lookups a day, so a
  command that exits before the answer arrives spends one of the five. `ensure` runs before every
  `claude` launch and promises no network; `monitor` is the daemon, whose lookup would answer and
  mark the day with the notice written only to its log file; `completion` is sourced from a shell rc
  on every start and cobra's `__complete` runs on every TAB press, so either would use up the day's
  lookups before a command anyone reads gets one. `checksForUpdates` keeps them out, together with
  `version`, `self-update` and `help`; it judges a command by its ancestor directly under the root,
  because `completion bash` is named `bash`. Exempt any new command that the shell integration runs.
- **`self-update` cannot update a running daemon.** The daemon keeps running the binary it was
  started from, and `monitor --ensure-daemon` finds it alive and leaves it be, so after an install
  `self-update` names the daemon's pid for the user to stop. Whether a release went in is read off
  the binary — `os.SameFile` on the executable before and after — because cliforge reports success
  the same way for an install, a dry run, an up-to-date binary and a declined prompt. Take the
  "before" identity through an open handle (`File.Stat`), never `os.Stat`: on Windows `os.Stat` reads
  the identity only when `os.SameFile` compares it, by which time the new binary holds the path, so
  every install would read as none and the daemon would never be named.

## Conventions

- All persistence is atomic (temp file + rename) and owner-only (0600).
- Logrus is imported aliased as `logger`; user-facing text goes to stdout/stderr with a `[ccswitch]`
  prefix. Colors in log lines are left to logrus, which uses them on a terminal only: the daemon logs to
  a file, so `ForceColors` would fill it with escape codes. The colors of `list` come from `palette`
  and keep to the sixteen theme colors plus bold and faint: never "bright black", which Solarized Dark
  paints in the background color, and bold only on red, since Windows Terminal and xterm show bold as
  bright by default, which washes green, yellow and cyan out on a light background.
  `-v`/`--verbose` and `DEBUG=true` turn on debug logging, and `daemonArgs` passes `--verbose` on to a
  daemon started by an invocation that named it.
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
  `chlog new --kind <Kind> --body '<past-tense description>'`
- Write an apostrophe inside the single-quoted body as `'\''`.
- Valid kinds: Added, Changed, Deprecated, Removed, Fixed, Security
- Choose the kind that best matches the change (e.g., new feature → Added,
  bug fix → Fixed, behavior change → Changed, removal → Removed, security fix → Security).
- If the change is backward-INCOMPATIBLE with the public API (a breaking
  change), you MUST add the `--breaking` flag:
  `chlog new --kind <Kind> --breaking --body '<past-tense description>'`.
  This is the ONLY thing that triggers a major version bump — the kind alone
  never does (per SemVer, major = incompatible change). When unsure whether a
  change breaks compatibility, ask the user instead of guessing.
- Fragments are YAML files in `.changes/unreleased/`; stage them with your commit.
- `chlog check` fails the build when a fragment is missing — never skip it.
<!-- chlog:end -->
