# whatsinstalled

A CLI/TUI that gives you one consolidated view of everything installed on your
system — across every package manager, language ecosystem, and loose binaries —
so the graveyard of tools you've accumulated stops being invisible.

For each package it shows the **source** it came from, **where** it lives, **who**
installed it, its **on-disk size**, and when it was **added** / **last used**
(size, added and used are filled in where they can be measured reliably). It also answers natural-language
questions like _"which python tools do I have?"_ via a small local LLM
(experimental).

```
whatsinstalled            # launch the TUI dashboard
whatsinstalled scan       # rescan and print a per-source summary
```

## What it detects

`whatsinstalled` auto-detects the sources below. A source only shows up when its tool
is installed **and** it actually has packages — the dashboard tabs are rendered
dynamically per machine, so you never see empty tabs.

### System / OS package managers
| Source | Detects |
|---|---|
| **apt** | Installed dpkg packages. Dependency packages are hidden by default — press `a` to show them. |
| **snap** | Snap packages |
| **flatpak** | Flatpak packages |
| **brew** | Homebrew packages |
| **nix** | nix-env packages |
| **pacman** | Arch pacman packages |
| **yay** | AUR packages |

### Language / ecosystem package managers
| Source | Detects | Where it looks |
|---|---|---|
| **npm** | Top-level npm packages | Global + any `package.json` project under `~/*` and the current dir |
| **pnpm** | Global pnpm packages | `pnpm ls -g` |
| **yarn** | Global yarn (v1) packages | `yarn global list` |
| **pip** | Top-level pip packages | System Python + `.venv`/`venv`/`env` virtualenvs under `~/*` and the current dir |
| **pipx** | Isolated Python CLI apps | `~/.local/share/pipx/venvs` |
| **uv** | `uv tool install` CLI tools | `~/.local/share/uv/tools` |
| **conda** | Packages per environment | All conda environments |
| **pixi** | pixi global environments | pixi global install area |
| **gem** | RubyGems | `gem list --local` |
| **cargo** | Cargo-installed Rust binaries | `~/.cargo/bin` |
| **go** | Downloaded Go modules | `~/go/pkg/mod` |

### Containers & loose files
| Source | Detects |
|---|---|
| **docker** | Local Docker images |
| **podman** | Local Podman images |
| **appimage** | Portable `*.AppImage` apps in `~/Applications`, `~/Downloads`, `~/.local/bin`, `/opt` — tracked by no package manager |
| **bin** | Manually-installed binaries in `~/.local/bin`, `~/bin`, `~/go/bin`, `~/.cargo/bin`, `~/.yarn/bin`, `~/.npm-global/bin`, version-manager shims, `/usr/local/bin`, `/usr/bin` |

> Tuned for Debian/Ubuntu (incl. WSL). Tools that don't apply to your machine are
> simply skipped.

## Key bindings (TUI)

| Key | Action |
|---|---|
| `↑↓` / `jk` | Navigate |
| `←→` / `hl` | Expand / collapse |
| `Tab` | Switch source |
| `/` | Filter by name |
| `a` | Show / hide auto-installed dependencies |
| `?` | Natural-language search (experimental) |
| `:` | Command palette |
| `t` | Switch theme |
| `d` | Details |
| `r` | Rescan |
| `q` | Quit |

## Build

```
go build -o whatsinstalled ./cmd/whatsinstalled
```

Requires Go 1.25+. State is stored in a local SQLite database.
