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

- **Usage monitoring**: a background daemon polls the Claude usage endpoint (`/api/oauth/usage`) for every enrolled account, so it knows when the active one is exhausted and keeps each backup's tokens alive.
- **Automatic rotation**: when the active account crosses a utilization threshold (default 99%), it swaps in the next account that still has capacity. Retune it at any time with `ccswitch threshold <percent>` — a running daemon picks the new value up without a restart.
- **Primary-first**: it always runs on the highest-priority account that has capacity, and returns to your primary as soon as its limits reset. Pass `--prefer-primary=false` for plain round-robin instead.
- **Account management**: list, inspect, reprioritize, reorder and remove enrolled accounts from the command line; `ccswitch list` shows when every limit of every account resets.
- **Usage at a glance**: on a terminal, `ccswitch list` and `ccswitch show` draw a meter beside every limit, green while the limit is well clear of the rotation threshold, yellow as it closes in and red once it reaches it, line the reset times up in columns, and show exhausted accounts in red.
- **Enroll once**: each account is captured a single time (its long-lived refresh token is persisted); after that, rotation is automatic — no repeated `/login`.
- **Session-safe**: never rewrites credentials while a `claude` process is running; the switch is applied on the next launch.
- **Cross-platform**: Linux, macOS, and Windows, on amd64 and arm64.
- **Seamless shell integration**: an optional shell wrapper (a few lines shown below) keeps the daemon alive and ensures each `claude` launch uses the current account.
- **Self-updating**: `ccswitch self-update` installs the latest release in place, and the commands you run by hand tell you, at most once a day, when a newer release is out.

## How it works

Claude Code stores its subscription OAuth tokens under `claudeAiOauth` — in `~/.claude/.credentials.json` on Linux and Windows, and in the login keychain on macOS (see [macOS notes](#macos-notes)). `ccswitch` keeps a local store of enrolled accounts and, when the active one is exhausted, atomically installs the next account's tokens in whichever of those the platform uses. Claude Code refreshes the swapped-in access token itself on launch, so no manual login is needed.

Rotation happens at launch boundaries, not mid-conversation: a running session that hits its limit is not hot-switched; you exit and relaunch (optionally `claude --continue`) and the new account is already active.

Every enrolled account is polled, not just the active one — the backups on a slower cadence. That is
what keeps a backup usable: the OAuth refresh token is rotated on every use and expires in weeks, so
an account nobody touches between rotations goes stale in the store, and installing a stale pair
hands Claude Code a token the server has already forgotten. Claude Code answers the resulting
`invalid_grant` by blanking its stored credentials, which is what a logout on rotation actually is.

For the same reason a refresh preserves the whole credential document rather than rebuilding it from
the response. The token endpoint returns the new pair and little else, and Claude Code will not
persist its own later refresh of a credential set whose `scopes` do not name `user:inference` — it
classifies it as not-claude.ai and drops it — so a refresh names the scopes it wants, reads `scope`
and `refresh_token_expires_in` back, and merges onto the credentials it replaces.

Enrolled accounts are matched to the credentials on disk by their **identity** (`emailAddress`/`accountUuid` from `~/.claude.json`), never by their refresh token. The server rotates the refresh token on every refresh, so matching on it would stop recognizing the account the first time Claude Code refreshed — leaving the store pinned to a token that has been rotated away, which then fails every refresh with `401`.

### The rotation threshold

An account counts as exhausted once any of its active limits reaches the threshold, and the
utilization percentage is the only test. The usage endpoint also reports a `severity` per limit, but
that is a display band rather than a ceiling: it reads `critical` from around 95% with
`locked_reason` still null — while the account is perfectly usable — so treating it as exhaustion
capped every threshold at the point the warning fires and made a threshold of 99 behave exactly
like 90.

The threshold set by `ccswitch threshold` lives in the store rather than on a command line, which is
what lets it change in flight: the monitor reloads the store on every tick and reads it from there.
An explicit `--threshold` still wins for the invocation that passes it.

```bash
ccswitch threshold          # show the threshold in force and where it comes from
ccswitch threshold 100      # run each account to the wire before rotating
ccswitch threshold --reset  # back to the built-in default
```

Setting one applies it immediately as well as persisting it: every account is repolled, the
exhaustion markers the old threshold produced are rewritten from what the polls saw, and the
highest-priority account below the new threshold becomes active.

### Rotation policy

By default (`--prefer-primary`) the monitor always runs on the **highest-priority account that has capacity**. Priority starts out as enrollment order, so the first account you enroll is the primary; `ccswitch list` numbers the accounts from 1 for the primary, and [Managing accounts](#managing-accounts) shows how to change the order. It falls back to a backup only while the primary is exhausted, and switches back to the primary as soon as the primary's limits reset.

An exhausted account is held until **every** limit that put it over the threshold has reset — not merely the soonest one. That matters when a short window (the 5-hour session) resets while a long one (the weekly limit) is still saturated: releasing the account early would select it, immediately exhaust it again, and flap. That recorded reset time is an upper bound, not a lease: the monitor keeps polling the account and releases it as soon as a poll shows it back under the threshold.

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

Download pre-built binaries from the [releases page](https://github.com/rios0rios0/ccswitch/releases). On Windows, run the installer from Git Bash, MSYS2 or Cygwin (it needs `unzip`), or download the `.zip` and put `ccswitch.exe` somewhere on your `PATH`.

The installer takes `--version <version>` to pin a release, `--install-dir <dir>` for somewhere other than `~/.local/bin`, `--force` to reinstall, and `--dry-run` to see what it would do.

### Updating

```bash
ccswitch self-update            # install the latest release, after asking
ccswitch self-update --force    # install it without asking
ccswitch self-update --dry-run  # only show what would be installed
ccswitch --version              # or `ccswitch version` for the bare number
```

The commands you run by hand — `list`, `status`, `use` and the like — also look for a newer release in
the background and print a one-line notice if the answer arrives before they finish. A day counts as
checked only once an answer has arrived, so a command that finishes first leaves the check to the next
one, and no more than five lookups start in a day. The commands the
[shell integration](#shell-integration) runs unattended never look: `ensure` stays off the network,
and the `monitor` daemon's output only reaches its log file.

A monitor daemon that is already running keeps running the binary it was started from, so after an
update `self-update` names its process: stop it, and the next `ccswitch monitor --ensure-daemon`
starts the new version.

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

`ccswitch` behaves the same on Windows, with three differences worth knowing:

- **Session detection recognizes `claude.exe` only.** The daemon never rewrites credentials while Claude Code is running, and it identifies a running session by the executable name. A natively installed Claude Code is detected; an npm installation runs the CLI inside `node.exe`, which is indistinguishable from any other Node process, so a session started that way is not seen and credentials may be swapped underneath it. Prefer the native install, or stop the daemon while a session is open.
- **The store is not owner-only.** On Linux and macOS the account store and credentials are written with `0600`. Windows ignores those bits, so the files inherit the permissions of their parent directory (normally your user profile, which is already restricted to you).
- **`self-update` leaves the previous binary behind until the next update.** Windows cannot delete the file of a program that is still running, and the replaced `ccswitch.exe` is still running the update itself (and perhaps a monitor daemon), so it is moved aside as `ccswitch.exe.backup-<number>` beside the new one. The next `self-update` removes it, or you can delete it yourself once no older `ccswitch` process is left.

## Usage

```bash
ccswitch enroll                    # capture the currently logged-in Claude account
# log in as another account with `claude` then `/login`, then:
ccswitch enroll                    # capture the next account
ccswitch enroll --token <token> --email <email>   # enroll a long-lived token (manual fallback only, see below)
ccswitch enroll --priority top     # capture it as the new primary (see Managing accounts)
ccswitch list                      # list all accounts with live usage and when each limit resets
ccswitch show <email>              # show one account in detail
ccswitch update <email> --priority 2   # move an account in the rotation order
ccswitch reorder <email>...        # set the rotation order, primary first
ccswitch remove <email>...         # remove accounts from the store
ccswitch status                    # show the active account and its usage
ccswitch use <email>               # manually switch accounts
ccswitch rotate                    # rotate to the next healthy account
ccswitch threshold 100             # set the rotation threshold, applied immediately
ccswitch monitor                   # run the daemon in the foreground
ccswitch monitor --ensure-daemon   # start the daemon in the background if not running
ccswitch self-update               # update ccswitch to the latest release (see Updating)
ccswitch version                   # print the version
```

### Managing accounts

Every enrolled account has a place in the rotation order, and `ccswitch list` numbers them from 1 for
the primary. Under each account it prints every limit the usage endpoint reports, its utilization, and
when it resets — as a countdown and as a local time, with the date spelled out because a weekly limit
can reset a full week out, on today's weekday. An exhausted account also says when it is available
again, which is when every limit over the threshold has reset. On a terminal it looks like this, in
[color](#colors):

```text
rotation threshold: 99%

* 1. primary@example.com [ok]
     5-hour       ███████████▏░░░░░░░░  56%  resets in 2h 13m   Tue Sep 29 17:47
     7-day        ████████▏░░░░░░░░░░░  41%  resets in 4d 2h    Sat Oct 3 17:50
     7-day scoped ██▍░░░░░░░░░░░░░░░░░  12%  resets in 4d 2h    Sat Oct 3 17:50

  2. backup@example.com [exhausted, available again in 31m (Tue Sep 29 16:05)]
     5-hour       ████████████████████ 100%  resets in 31m      Tue Sep 29 16:05
     7-day        ████████████░░░░░░░░  60%  resets in 5d 23h   Mon Oct 5 15:10

  3. manual@example.com [manual only] long-lived token; its usage cannot be polled
```

Piped or redirected, `list` prints the same listing as plain text, without meters or escape codes, which
is the form a script should read:

```text
rotation threshold: 99%
* 1. primary@example.com [ok]
     5-hour         56%  resets in 2h 13m (Tue Sep 29 17:47)
     7-day          41%  resets in 4d 2h (Sat Oct 3 17:50)
     7-day scoped   12%  resets in 4d 2h (Sat Oct 3 17:50)
  2. backup@example.com [exhausted, available again in 31m (Tue Sep 29 16:05)]
     5-hour        100%  resets in 31m (Tue Sep 29 16:05)
     7-day          60%  resets in 5d 23h (Mon Oct 5 15:10)
  3. manual@example.com [manual only] long-lived token; its usage cannot be polled
```

When the usage endpoint cannot be read for an account — it rate-limits — `list` falls back to the last
reading the monitor recorded for it, whose reset times still hold. `ccswitch show <email>` prints the
same for one account, with the same meters and colors on a terminal, together with its plan, when its
tokens expire, and when the monitor last polled it. Neither ever prints a token.

A priority is a position counted from 1 for the primary, or one of `top`, `bottom`, `up` and `down`:

```bash
ccswitch update backup@example.com --priority top    # make it the primary
ccswitch update backup@example.com --priority down   # one place lower
ccswitch update backup@example.com --priority 2      # second place
ccswitch reorder work@example.com home@example.com   # these two first; the rest keep their order
ccswitch enroll --priority top                       # enroll the logged-in account as the primary
```

Changing the order touches no credentials. The monitor reloads the store on every poll, so a running
daemon applies the new order on its next one: with `--prefer-primary` it switches to an account you
moved above the active one as long as that account has capacity.

`ccswitch remove <email>...` removes accounts and their stored tokens; nothing is removed unless every
named account is enrolled. Enrolling a removed account again takes a login as that account first.
Removing the active account hands over to the highest-priority remaining account with capacity, and
installs it at once — or on the next launch while a `claude` session is running. A long-lived account is
never handed over to automatically; when only those remain, pick one with `ccswitch use <email>`.

### Flags

| Flag             | Default                               | Description                                            |
|------------------|---------------------------------------|--------------------------------------------------------|
| `--threshold`    | `99`                                  | Utilization percent (0-100) that triggers rotation, for this invocation only. The value stored by `ccswitch threshold` applies when this is not passed. |
| `--interval`     | `5m`                                  | Monitor poll interval.                                 |
| `--prefer-primary` | `true`                              | Always run on the highest-priority account with capacity, returning to the primary as soon as its limits reset. |
| `--store`        | `~/.local/state/ccswitch/store.json`  | Path to the account store.                             |
| `--credentials`  | `~/.claude/.credentials.json`         | Path to Claude Code's credentials file. Ignored on macOS, where the login keychain is used instead. |
| `-v`, `--verbose` | `false`                              | Enable debug logging. A daemon started by `monitor --ensure-daemon -v` logs at debug level too. `DEBUG=true` in the environment does the same. |
| `--color`        | `auto`                                | When to color `ccswitch list` and `ccswitch show`: `auto` colors a terminal unless the environment says otherwise, `always` colors a pipe too, and `never` colors nothing. See [Colors](#colors). |

### Colors

On a terminal, `ccswitch list` and `ccswitch show` color every limit by how close it stands to the
rotation threshold: green below 80% of the threshold, yellow from there, and red once it reaches the
threshold, the point at which an active limit rotates its account away. The same color fills the limit's
meter, 20 cells of 5% each, filled in eighths of a cell. Figures and meters round down, as Claude Code's
own usage screen does, so a figure never reads as the threshold before it turns red, and only a limit
that is used up fills its meter. Exhausted accounts are red, countdowns cyan, the mark on the account
Claude Code runs on (`*` in `list`, `(active)` in `show`) green, and manual-only accounts magenta.
Warnings are yellow: usage that could not be read, and credentials Claude Code would discard. Dates,
positions and the empty part of each meter are faint.

The first of these that applies decides whether there are colors:

1. `--color always` or `--color never`. `always` colors a pipe as well, for a pager that renders them,
   such as `less -R`; `never` still draws the meters on a terminal.
2. `FORCE_COLOR`, which forces colors when set to anything but `0` or `false`, which turn them off.
3. [`NO_COLOR`](https://no-color.org/) set to any non-empty value, which turns them off and keeps the
   meters.
4. `CLICOLOR_FORCE` set to anything but `0`, which forces colors.
5. `CLICOLOR=0` or `TERM=dumb`, which turn them off and keep the meters.
6. Otherwise, colors when the output is a terminal.

Each variable ranks the way its own specification has it. `FORCE_COLOR` outranks `NO_COLOR`, as in the
reference code of [force-color.org](https://force-color.org/) and in Node.js, since it is usually set for
the one command it comes with while `NO_COLOR` is a standing preference. `CLICOLOR_FORCE` yields to
`NO_COLOR`, as [its specification](https://bixense.com/clicolors/) says.

Output that is not a terminal and not forced into color is the plain text shown under
[Managing accounts](#managing-accounts). On Windows, ccswitch switches the console into virtual terminal
mode to render the colors; a console that cannot be switched, older than Windows 10, shows the meters
without them.

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
    │   ├── commands/             # enroll, list, show, update, remove, reorder, status, use, rotate, threshold, ensure, monitor, self-update
    │   └── repositories/         # ports: accounts, credentials, usage, tokens, sessions, self-update
    └── infrastructure/
        ├── controllers/          # cobra CLI wiring
        ├── repositories/         # JSON store, credentials swappers (file / macOS keychain), HTTP usage/token clients, session probes, GitHub releases (via cliforge)
        └── services/             # background daemon supervision
```

Anything the operating system does differently lives in a `_unix.go` / `_darwin.go` / `_windows.go`
pair: process detachment and liveness in `services`, the credentials swapper and session detection
in `repositories` (credentials file versus login keychain; `/proc` scan, process table, or ToolHelp32
snapshot), and the choice between them in `controllers`, together with switching a Windows console into
virtual terminal mode so it renders colors. Everything else is portable.

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
