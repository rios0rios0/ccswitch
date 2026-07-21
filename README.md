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
- **Seamless shell integration**: an optional shell wrapper (a few lines shown below) keeps the daemon alive and ensures each `claude` launch uses the current account.

## How it works

Claude Code stores its subscription OAuth tokens in `~/.claude/.credentials.json` (under `claudeAiOauth`). `ccswitch` keeps a local store of enrolled accounts and, when the active one is exhausted, atomically rewrites that file with the next account's tokens. Claude Code refreshes the swapped-in access token itself on launch, so no manual login is needed.

Rotation happens at launch boundaries, not mid-conversation: a running session that hits its limit is not hot-switched; you exit and relaunch (optionally `claude --continue`) and the new account is already active.

### Rotation policy

By default (`--prefer-primary`) the monitor always runs on the **highest-priority account that has capacity**. Priority is enrollment order, so the first account you enroll is the primary — check the numbering with `ccswitch list`. It falls back to a backup only while the primary is exhausted, and switches back to the primary as soon as the primary's limits reset.

An exhausted account is held until **every** limit that put it over the threshold has reset — not merely the soonest one. That matters when a short window (the 5-hour session) resets while a long one (the weekly limit) is still saturated: releasing the account early would select it, immediately exhaust it again, and flap.

With `--prefer-primary=false` the monitor instead cycles forward, staying on each account until that account is exhausted and only then advancing.

Because the daemon enforces this policy continuously, `ccswitch use` and `ccswitch rotate` take effect immediately but are not sticky — the next poll returns to whatever the policy selects. To pin an account manually, run with `--prefer-primary=false` or stop the daemon.

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/rios0rios0/ccswitch/main/install.sh | sh
```

Or build from source:

```bash
make install    # builds and copies the binary to ~/.local/bin/ccswitch
```

Download pre-built binaries from the [releases page](https://github.com/rios0rios0/ccswitch/releases).

## Usage

```bash
ccswitch enroll                    # capture the currently logged-in Claude account
# log in as another account with `claude` then `/login`, then:
ccswitch enroll                    # capture the next account
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
| `--credentials`  | `~/.claude/.credentials.json`         | Path to Claude Code's credentials file.                |

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
        ├── repositories/         # JSON store, Claude credentials file, HTTP usage/token clients, /proc probe
        └── services/             # background daemon supervision
```

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
