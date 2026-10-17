<div align="center">

<img src="docs/logo.svg" alt="zlib" width="160"/>

# zlib

A Z-Library CLI to search, download, and send books to Kindle, for you and AI agents like Claude Code and Codex.

<a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/heartleo/zlib" alt="Go version" height="20"/></a> <a href="https://github.com/heartleo/zlib/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/heartleo/zlib/ci.yml" alt="CI" height="20"/></a> <a href="https://github.com/heartleo/zlib/releases/latest"><img src="https://img.shields.io/github/v/release/heartleo/zlib" alt="Release" height="20"/></a> <a href="https://github.com/heartleo/zlib/releases"><img src="https://img.shields.io/github/downloads/heartleo/zlib/total" alt="Downloads" height="20"/></a> <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green" alt="License" height="20"/></a>

<a href="https://trendshift.io/repositories/135168?utm_source=repository-badge&amp;utm_medium=badge&amp;utm_campaign=badge-repository-135168" target="_blank" rel="noopener noreferrer"><img src="https://trendshift.io/api/badge/repositories/135168" alt="heartleo%2Fzlib | Trendshift" width="250" height="55"/></a>

English | [简体中文](README.zh.md)

[Install](#install) · [Quick start](#quick-start) · [Terminal interface](#terminal-interface) · [Commands](#commands) · [Agent setup](#agent-setup) · [Docs](#documentation)

</div>

![Book search in the terminal](docs/demo-search.gif)

## Features

🔍 **Search** — filter by year, language, and format; sort results\
📚 **Browse** — favorites, recommendations, popular books, and download history\
📥 **Download** — view book details and choose a download directory\
📖 **Send to Kindle** — deliver files to your [Kindle address](docs/kindle.md)\
🕒 **Profile** — check your daily download quota\
🎨 **Themes** — switch color themes with `/theme`\
🌐 **Network** — find working mirrors with `zlib doctor`, or use a proxy\
🤖 **AI Agent Integration** — integrate with Claude Code and Codex through [Agent plugins](#agent-setup)

## Install

macOS / Linux with Homebrew:

```bash
brew install heartleo/tap/zlib
```

macOS / Linux with the install script (installs to `/usr/local/bin`, may ask for `sudo`):

```bash
curl -fsSL https://raw.githubusercontent.com/heartleo/zlib/main/install.sh | sh
```

Windows with winget:

```powershell
winget install --id heartleo.zlib --exact
```

Or download a binary from [Releases](https://github.com/heartleo/zlib/releases/latest)
and put it on your `PATH`. See [Installation](docs/installation.md) for Go
installation, source builds, and upgrades.

## Quick start

You need a Z-Library account. First, find a domain reachable from your network:

```sh
zlib doctor
```

Log in with a domain from the results. Replace the example URL below with one
from your results; enter your email and password in the prompt:

```sh
zlib login --domain https://z-lib.gd
zlib tui
```

Press `/`, choose `/search`, enter a title, author, or ISBN, and press `Ctrl+S`.
You can also type `/search dune` and press `Enter`. Select a book and press `Enter`
to confirm the download.

Downloads default to the current directory. You can choose another folder before
downloading. Login is saved locally; subsequent sessions only need `zlib tui`.

If no domain works or login fails, see [Troubleshooting](docs/troubleshooting.md).
Existing browser sessions can be [imported as cookies](docs/troubleshooting.md#login-returns-authorization-failed).

## Terminal interface

`zlib tui` opens one persistent shell. Every screen lists its keys at the bottom;
these cover a first session:

| Key | Action |
| --- | --- |
| `/` | Open the command list: search, favorites, history, profile, settings, themes |
| `↑/↓` or `j/k` | Select a book |
| `←/→` | Previous or next page of results |
| `Enter` | Download the selected book, after a confirmation |
| `Ctrl+F` | Change the query and filters of the current search |
| `Esc` | Go back |
| `/quit` | Exit |

Settings live under `/config` (network, downloads, Kindle). See
[Terminal interface](docs/tui.md) for every screen and key.

## Commands

Pass a query for table output, or add `--json` for scripts:

```sh
zlib search "dune" --ext epub
zlib search "dune" --ext epub --json --count 10
```

| Task | Command |
| --- | --- |
| Browse favorites | `zlib favorites` |
| Browse recommendations | `zlib recommended` |
| Browse popular books | `zlib most-popular` |
| Browse download history | `zlib history` |
| Check account and quota | `zlib profile` |
| Rotate accounts on quota exhaustion | `zlib accounts add`, `zlib accounts switch` |
| Send to [Kindle](docs/kindle.md) | `zlib kindle send ./book.epub` |

To download, replace `id:hash` with a book's complete `id` from
the results:

```sh
zlib download "id:hash"
```

Run `zlib <command> --help` for flags. [Usage](docs/usage.md) covers filters,
pagination, and download options.

## Agent setup

Install the plugin. If `zlib` is missing, the agent installs it on first use.
Log in yourself with `zlib login` so your password stays out of chat.

### Claude Code

Run inside Claude Code:

```text
/plugin marketplace add heartleo/zlib
/plugin install zlib@zlib-plugin-cc
```

### Codex

Run in your terminal:

```sh
codex plugin marketplace add heartleo/zlib
codex plugin add zlib@zlib
```


> Use zlib to find The Pragmatic Programmer in EPUB. Show the results before downloading.

## Multi-account rotation

Each Z-Library account gets its own daily download quota. Configure several
and `download` (and every other download path — search Enter, `history
--download`) rotates automatically: when the server rejects a download with
the daily-limit error, the CLI marks that account exhausted and retries with
the next account that may still have quota. The rotation order is the active
account first, then the rest. Every account is tried at most once per
download.

Accounts live in `~/.config/zlib/accounts.json`; the active one is mirrored
into `session.json`, so single-account setups behave exactly as before.

```bash
zlib accounts add main --user-id 123 --user-key abc --domain https://zlib.bz
zlib accounts add alt                    # interactive login flow, same flags as `zlib login`
zlib accounts import backup              # file the current session.json login under a name
zlib accounts list                       # table with each account's cached quota
zlib accounts switch backup              # manual switch
zlib accounts remove backup              # remove one account
zlib accounts clear                      # remove all (the active login stays)
```

`--user-id`/`--user-key` record the `remix_userid`/`remix_userkey` pair
directly — the same credentials the Z-Library desktop app keeps in its
`config.json` — without any network request.

Quota bookkeeping costs no extra API calls: each successful download steps
the cached remaining down, the daily-limit error zeroes it, and when every
account looks exhausted the CLI re-checks with the server once (this is also
how it notices the quota reset after midnight). `zlib profile` refreshes the
active account's cache. When all accounts are truly out of quota the download
fails with an error listing each account's remaining count.

Notes:

- An expired session on one account is not rotated away; remove and re-add
  that account to re-login it.
- A set `ZLIB_DOMAIN` still overrides every account's own domain.

## Documentation

| Guide | Covers |
| --- | --- |
| [Installation](docs/installation.md) | Install methods, source builds, `zlib update` |
| [Usage](docs/usage.md) | Search, downloads, favorites, history |
| [Terminal interface](docs/tui.md) | Commands and keyboard controls |
| [Configuration](docs/configuration.md) | Domains, proxies, folders, configuration paths |
| [Themes](docs/themes.md) | Available palettes and preview controls |
| [Kindle](docs/kindle.md) | SMTP setup and delivery |
| [Troubleshooting](docs/troubleshooting.md) | Login and network problems |

Report bugs in [Issues](https://github.com/heartleo/zlib/issues) with your
`zlib version`, OS, command, and error. Omit passwords, cookies, and session files.
Report security vulnerabilities privately as described in [SECURITY.md](SECURITY.md).

## Star History

<a href="https://www.star-history.com/?repos=heartleo%2Fzlib&type=date&logscale=&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=heartleo/zlib&type=date&theme=dark&logscale&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=heartleo/zlib&type=date&logscale&legend=top-left" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=heartleo/zlib&type=date&logscale&legend=top-left" />
 </picture>
</a>
