<h1 align="center">ccswitch</h1>
<p align="center">
    <a href="https://github.com/rios0rios0/ccswitch/releases/latest">
        <img src="https://img.shields.io/github/release/rios0rios0/ccswitch.svg?style=for-the-badge&logo=github" alt="Latest Release"/></a>
    <a href="https://github.com/rios0rios0/ccswitch/blob/main/LICENSE">
        <img src="https://img.shields.io/github/license/rios0rios0/ccswitch.svg?style=for-the-badge&logo=github" alt="License"/></a>
    <a href="https://github.com/rios0rios0/ccswitch/actions/workflows/default.yaml">
        <img src="https://img.shields.io/github/actions/workflow/status/rios0rios0/ccswitch/default.yaml?branch=main&style=for-the-badge&logo=github" alt="Build Status"/></a>
</p>

`ccswitch` monitors Claude Code usage limits and transparently rotates between enrolled backup accounts when the active account runs out — so you keep working without re-authenticating.

## Features

- **Usage monitoring**: a background daemon polls the Claude usage endpoint (`/api/oauth/usage`) and knows when the active account is exhausted.
- **Automatic rotation**: when the active account crosses a utilization threshold (default 90%, or a `critical` limit), it swaps in the next account that still has capacity.
- **Primary-first**: it always runs on the highest-priority account that has capacity, and returns to your primary as soon as its limits reset. Pass `--prefer-primary=false` for plain round-robin instead.
- **Enroll once**: each account is captured a single time (its long-lived refresh token is persisted); after that, rotation is automatic — no repeated `/login`.
- **Session-safe**: never rewrites credentials while a `claude` process is running; the switch is applied on the next launch.
- **Cross-platform**: Linux, macOS, and Windows, on amd64 and arm64.
- **Seamless shell integration**: an optional shell wrapper (a few lines shown below) keeps the daemon alive and ensures each `claude` launch uses the current account.

## How it works

Claude Code stores its subscription OAuth tokens under `claudeAiOauth` — in `~/.claude/.credentials.json` on Linux and Windows, and in the login keychain on macOS (see [macOS notes](#macos-notes)). `ccswitch` keeps a local store of enrolled accounts and, when the active one is exhausted, atomically installs the next account's tokens in whichever of those the platform uses. Claude Code refreshes the swapped-in access token itself on launch, so no manual login is needed.

Rotation happens at launch boundaries, not mid-conversation: a running session that hits its limit is not hot-switched; you exit and relaunch (optionally `claude --continue`) and the new account is already active.

Enrolled accounts are matched to the credentials on disk by their **identity** (`emailAddress`/`accountUuid` from `~/.claude.json`), never by their refresh token. The server rotates the refresh token on every refresh, so matching on it would stop recognizing the account the first time Claude Code refreshed — leaving the store pinned to a token that has been rotated away, which then fails every refresh with `401`.

### Rotation policy

By default (`--prefer-primary`) the monitor always runs on the **highest-priority account that has capacity**. Priority is enrollment order, so the first account you enroll is the primary — check the numbering with `ccswitch list`. It falls back to a backup only while the primary is exhausted, and switches back to the primary as soon as the primary's limits reset.

An exhausted account is held until **every** limit that put it over the threshold has reset — not merely the soonest one. That matters when a short window (the 5-hour session) resets while a long one (the weekly limit) is still saturated: releasing the account early would select it, immediately exhaust it again, and flap.

With `--prefer-primary=false` the monitor instead cycles forward, staying on each account until that account is exhausted and only then advancing.

Because the daemon enforces this policy continuously, `ccswitch use` and `ccswitch rotate` take effect immediately but are not sticky — the next poll returns to whatever the policy selects. To pin an account manually, run with `--prefer-primary=false` or stop the daemon.

## Installation

Linux, macOS, and Windows are supported, on both amd64 and arm64.

```bash
curl -fsSL https://raw.githubusercontent.com/rios0rios0/ccswitch/main/install.sh | sh
```

Or build from source:

```bash
make install    # builds and copies the binary to ~/.local/bin/ccswitch
```

Download pre-built binaries from the [releases page](https://github.com/rios0rios0/ccswitch/releases). The installer script is for Linux and macOS; on Windows, download the `.zip` and put `ccswitch.exe` somewhere on your `PATH`.

### macOS notes

On macOS Claude Code does not keep its tokens in `~/.claude/.credentials.json`. It uses a
`keychain-with-plaintext-fallback` credential store: the login keychain is authoritative, and the
file is only consulted when the keychain read returns nothing. So while a keychain item exists,
writing that file has no effect at all.

`ccswitch` therefore reads and writes the generic-password item `Claude Code-credentials` (filed
under your login name) directly, via `security`. Two consequences worth knowing:

- **`--credentials` is ignored on macOS.** The keychain item is the target; there is no path to
  point at.
- **Rotation preserves your MCP logins.** That keychain item is a single JSON document holding both
  `claudeAiOauth` *and* `mcpOAuth` — the OAuth tokens of every MCP server you have authenticated.
  Rotation is a read-modify-write that replaces only `claudeAiOauth`, so the MCP tokens survive.
  Every write is read back and compared before being reported as successful, because `security -i`
  truncates payloads over ~4 KB without a reliable error, and a truncated write here would sign you
  out of Claude *and* of every MCP server at once.

Session detection uses the process table rather than `/proc`, which macOS does not provide.
Executables inside `.app` bundles are never counted as sessions: the Claude desktop app runs as
`Claude.app/Contents/MacOS/Claude`, whose name matches the CLI's case-insensitively, and treating it
as a live session would block every rotation for as long as the desktop app is open.

### Windows notes

`ccswitch` behaves the same on Windows, with two differences worth knowing:

- **Session detection recognizes `claude.exe` only.** The daemon never rewrites credentials while Claude Code is running, and it identifies a running session by the executable name. A natively installed Claude Code is detected; an npm installation runs the CLI inside `node.exe`, which is indistinguishable from any other Node process, so a session started that way is not seen and credentials may be swapped underneath it. Prefer the native install, or stop the daemon while a session is open.
- **The store is not owner-only.** On Linux and macOS the account store and credentials are written with `0600`. Windows ignores those bits, so the files inherit the permissions of their parent directory (normally your user profile, which is already restricted to you).

## Usage

```bash
ccswitch enroll                    # capture the currently logged-in Claude account
# log in as another account with `claude` then `/login`, then:
ccswitch enroll                    # capture the next account
ccswitch enroll --token <token> --email <email>   # enroll a long-lived token (manual fallback only, see below)
ccswitch list                      # list all accounts with live usage
ccswitch status                    # show the active account and its usage
ccswitch use <email>               # manually switch accounts
ccswitch rotate                    # rotate to the next healthy account
ccswitch monitor                   # run the daemon in the foreground
ccswitch monitor --ensure-daemon   # start the daemon in the background if not running
```

### Flags

| Flag             | Default                               | Description                                            |
|------------------|---------------------------------------|--------------------------------------------------------|
| `--threshold`    | `90`                                  | Utilization percent (0-100) that triggers rotation.    |
| `--interval`     | `5m`                                  | Monitor poll interval.                                 |
| `--prefer-primary` | `true`                              | Always run on the highest-priority account with capacity, returning to the primary as soon as its limits reset. |
| `--store`        | `~/.local/state/ccswitch/store.json`  | Path to the account store.                             |
| `--credentials`  | `~/.claude/.credentials.json`         | Path to Claude Code's credentials file. Ignored on macOS, where the login keychain is used instead. |

### Long-lived tokens

A token minted by `claude setup-token` can be enrolled directly, without an interactive `/login`:

```bash
ccswitch enroll --token <token> --email <email>
```

**Such an account cannot be monitored.** `setup-token` mints a token scoped for programmatic inference only — it does not carry the `user:profile` scope that `GET /api/oauth/usage` requires, so polling it returns `403 permission_error`. `ccswitch` therefore never polls a long-lived account and never rotates to it automatically, since it has no way to tell whether the account still has capacity. It is available as a **manual fallback**:

```bash
ccswitch use <email>     # switch to it deliberately
```

While a long-lived account is active the monitor keeps applying the rotation policy, so it returns to a normal account as soon as one has capacity again. To make an account monitorable, log in to it with `claude` and `/login` and enroll it normally — `ccswitch` picks the full-scoped credentials back up automatically.

### Important

If `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` is set in your environment, Claude Code authenticates with that key and ignores the rotated OAuth credentials. `ccswitch` warns when it detects this.

## Shell integration

`ccswitch` works on its own, but rotation is most seamless when the daemon stays alive and every `claude` launch uses the current account. Add this to your interactive shell config (e.g. `~/.zshrc`):

```bash
if command -v ccswitch >/dev/null 2>&1; then
    ccswitch monitor --ensure-daemon 2>/dev/null   # start the daemon if it is not already running
    claude() {
        ccswitch ensure --quiet 2>/dev/null          # no-network: install the current account's credentials
        command claude "$@"
    }
fi
```

## Architecture

Clean Architecture with a domain (ports) / infrastructure (adapters) split:

```
ccswitch/
├── cmd/ccswitch/                 # entrypoint
└── internal/
    ├── domain/
    │   ├── entities/             # Account, Usage, Limit, RotationState, Store, Config
    │   ├── commands/             # enroll, list, status, use, rotate, ensure, monitor
    │   └── repositories/         # ports: accounts, credentials, usage, tokens, sessions
    └── infrastructure/
        ├── controllers/          # cobra CLI wiring
        ├── repositories/         # JSON store, credentials swappers (file / macOS keychain), HTTP usage/token clients, session probes
        └── services/             # background daemon supervision
```

Anything the operating system does differently lives in a `_unix.go` / `_darwin.go` / `_windows.go`
pair: process detachment and liveness in `services`, the credentials swapper and session detection
in `repositories` (credentials file versus login keychain; `/proc` scan, process table, or ToolHelp32
snapshot), and the choice between them in `controllers`. Everything else is portable.

## Development

```bash
make lint    # golangci-lint
make test    # unit + integration tests
make sast    # security scanners
make build   # build the binary
```

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

See [LICENSE](LICENSE) file for details.
