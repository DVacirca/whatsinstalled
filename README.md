# whatsinstalled

One consolidated view of everything installed on your machine — every package
manager, language ecosystem, and loose binary — so the tools you've accumulated
stop being invisible.

```
whatsinstalled            # launch the TUI dashboard
whatsinstalled scan       # rescan and print a per-source summary
```

## Features

| | |
|---|---|
| **Unified inventory** | 22 sources in one view — OS managers, language ecosystems, containers, loose binaries. Empty sources auto-hide. |
| **Rich per-package facts** | Source, location, owner, on-disk size, added / last-used, and a dependency marker (`↳`). |
| **Interactive TUI** | Tree grouped by location, per-source tabs, live filter, details pane, 7 themes, command palette. |
| **Natural-language search** | Ask _"which python tools do I have?"_ — local MiniLM embeddings with substring fallback, zero network at query time. _(experimental)_ |
| **Read-only & offline** | Never installs or removes. State lives in local SQLite; enrichment and embedding are pre-computed at init so search can't hang. |

## What it detects

A source appears only when its tool is installed **and** actually has packages,
so you never see empty tabs.

### System / OS
| Source | Detects |
|---|---|
| **apt** | dpkg packages (dependencies hidden by default — press `a` to show) |
| **snap** | Snap packages |
| **flatpak** | Flatpak packages |
| **brew** | Homebrew packages |
| **nix** | nix-env packages |
| **pacman** | Arch packages |
| **yay** | AUR packages |

### Language ecosystems
| Source | Detects | Where it looks |
|---|---|---|
| **npm** | top-level packages | global + `package.json` projects under `~/*` |
| **pnpm** | global packages | `pnpm ls -g` |
| **yarn** | global (v1) packages | `yarn global list` |
| **pip** | top-level packages | system Python + venvs under `~/*` |
| **pipx** | isolated CLI apps | `~/.local/share/pipx/venvs` |
| **uv** | `uv tool` CLIs | `~/.local/share/uv/tools` |
| **conda** | per-environment packages | all conda environments |
| **pixi** | global environments | pixi global area |
| **gem** | RubyGems | `gem list --local` |
| **cargo** | Rust binaries | `~/.cargo/bin` |
| **go** | Go modules | `~/go/pkg/mod` |

### Containers & loose files
| Source | Detects |
|---|---|
| **docker** | local images |
| **podman** | local images |
| **appimage** | `*.AppImage` in `~/Applications`, `~/Downloads`, `~/.local/bin`, `/opt` |
| **bin** | unmanaged binaries across all common *nix bin dirs — `~/.local/bin`, `~/.cargo/bin`, version-manager shims, `$PATH`, `/usr/local/bin`, `/usr/bin`, `/opt/homebrew/bin`, … Binaries owned by a package manager are excluded. |

> Tuned for Debian/Ubuntu (incl. WSL). Tools that don't apply to your machine are skipped.

## Key bindings (TUI)

| Key | Action | Key | Action |
|---|---|---|---|
| `↑↓` / `jk` | Navigate | `:` | Command palette |
| `←→` / `hl` | Expand / collapse | `t` | Switch theme |
| `Tab` | Switch source | `d` | Details |
| `/` | Filter by name | `r` | Rescan |
| `a` | Toggle dependencies | `q` | Quit |
| `?` | Natural-language search | | |

## Build

```
go build -o whatsinstalled ./cmd/whatsinstalled
```

Requires Go 1.25+. State is stored in a local SQLite database. See
[ARCHITECTURE.md](ARCHITECTURE.md) for internals.
