# Requirements — whatsinstalled

## 1. Overview

`whatsinstalled` is a CLI/TUI tool that gives a consolidated view of packages installed on a Linux system across multiple package managers plus manually installed binaries. It tracks **all installed** packages (apt includes auto-installed dependencies), shows where each package lives, who installed it, when it was last used, and allows installation and uninstallation.

## 2. Supported Package Managers

| Manager | Scope | Location Representation |
|---------|-------|------------------------|
| apt | System-wide | `system` |
| snap | System-wide | `system` |
| npm | Global + local projects | `system` or project directory path |
| pip | Global + local venvs | `system` or venv parent directory |
| conda | All environments | Environment name (`base`, `chat-gpt`, etc.) |
| bin | Manual binaries in user directories | Absolute path to bin directory |

### 2.1 Bin Scanner
Scans executable files in user directories:
- `~/.local/bin`, `~/bin`, `~/go/bin`, `~/.cargo/bin`, `~/.yarn/bin`
- `~/.npm-global/bin`, `~/.nvm/versions/node/*/bin`
- `~/.rbenv/shims`, `~/.pyenv/shims`
- Any PATH directory under `$HOME` that is not a system package manager dir

Filters by executable bit (`mode & 0111`).

### 2.1 Scope Rules
- **apt**: All installed packages (`dpkg-query`). Includes both manually and auto-installed dependencies.
- **snap**: All installed snaps (no dependency concept).
- **npm**: Top-level packages only (`--depth=0`). No transitive dependencies. Include both global (`-g`) and local projects (any directory with `package.json` at `~/*` depth 1 + CWD).
- **pip**: Top-level packages only. Global system Python + local venvs (`.venv/`, `venv/`, `env/` at `~/*` depth 1 + CWD).
- **conda**: All packages in all environments (`conda env list --json` → `conda list --json -p <env>`).

## 3. Data Model

Each package record must include:

| Field | Type | Notes |
|-------|------|-------|
| `name` | string | Package name |
| `version` | string | Installed version |
| `source` | string | one of the 21 supported managers (`apt`, `snap`, `npm`, `pip`, `conda`, `bin`, `pixi`, `pipx`, `uv`, `go`, `docker`, `podman`, `brew`, `cargo`, `gem`, `pnpm`, `yarn`, `pacman`, `yay`, `flatpak`, `nix`, `appimage`) |
| `location` | string | `system` or path/env name |
| `size_bytes` | int64? | Disk usage if available |
| `description` | string | One-line summary if available |
| `installed_at` | string | ISO8601 or empty |
| `user` | string | Who installed it (see §4) |
| `auto_installed` | bool | `true` for apt dependency packages (`apt-mark showmanual`); hidden by default, toggle with `a` |
| `last_used` | time? | Access time of the package files (atime; unreliable on `noatime`/`relatime` mounts) |
| `added_at` | time? | Modification time of the package files (mtime; reliable install/update signal) |
| `embedding` | string | JSON float64 array for semantic search (cached) |

**Unique key**: `(name, source, location)`

## 4. User Tracking

Every package must record the user who installed it:
- **apt/snap**: Hardcoded `"system"` (system-wide, no per-user tracking).
- **npm/pip/conda**: Determine the owner of the project directory / venv path / conda env directory via `stat(2)` UID lookup. Fall back to the current OS user.

## 5. Scanning Behavior

### 5.1 Startup
1. Open SQLite database (create if absent).
2. Load cached data immediately for instant TUI display.
3. Kick off **background scan** of all package managers in parallel.
4. Merge results into DB via upsert.
5. Remove stale records (packages no longer found on system).
6. Refresh TUI with new data.

### 5.2 Performance
- Avoid per-package subprocess calls during scanning (e.g., do not run `pip show` in a loop — ~7s for 5 packages = unusable).
- Batch operations where possible (JSON output: `pip list --format=json`, `conda list --json`).
- Use `npm list --depth=0 --json` instead of parsing text.

### 5.3 Concurrency
- SQLite must use **WAL mode** (`PRAGMA journal_mode=WAL`) to allow UI readers while background writer holds the lock.
- Connection string must include `?_busy_timeout=5000` as a safety net.

## 6. TUI Design

### 6.1 Layout (Superfile-inspired)
```
┌─ whatsinstalled ── apt:90 │ snap:3 │ npm:14 │ pip:93 │ conda:1340 ─────┐
│  Name            Version   Src   Location        User    Size    │
│▶ ▾ system                   [193]                              │
│    nginx          1.24.0   apt   system        system   4.2M   │
│    core20         20260410 snap  system        system   -      │
│  ▸ base                      [108]                              │
│  ▸ chat-gpt                  [44]                               │
│  ▸ ~/projects/webapp         [2]                                │
│                                                                  │
│  [All] [Apt] [Snap] [Npm] [Pip] [Conda]  /filter                │
├─────────────────────────────────────────────────────────────────┤
│ Description          │ Metadata           │ Keys                │
│ nginx — web server   │ Name: nginx        │ ↑↓ navigate         │
│                      │ Version: 1.24.0    │ ←→ expand/collapse  │
│                      │ Source: apt         │ Tab switch source   │
│                      │ Location: system    │ / filter            │
│                      │ User: system        │ d details           │
│                      │ Size: 4.2M          │ ? ask (exp)         │
│                      │                     │ r rescan            │
│                      │                     │ q quit              │
├─────────────────────────────────────────────────────────────────┤
│ nginx (apt)  │  whatsinstalled — package tracker                     │
└─────────────────────────────────────────────────────────────────┘
```

### 6.2 Components
- **Title bar** (top strip): App name + per-source package counts. While a background scan/refresh runs, an animated spinner indicator shows in the right corner (the full-screen splash only appears on first run, when the cache is empty).
- **Tree panel** (main area): Hierarchical tree grouped by `location`. Group nodes show `▸`/`▾` expand/collapse indicator and child count `[N]`. Leaf nodes show package columns.
- **Column header**: `Name Version Src Location User Size Added Used`. `Size`/`Added`
  are populated only where the path resolves to the package's own files (file
  stat for bin/cargo/appimage, `du` of dedicated venvs for pipx/uv, native
  metadata for apt and docker/podman); shared-container sources show `-` rather
  than a misleading per-package number. `Added`/`Used` render as a compact age.
- **Tab bar** (inside tree panel): `All` first, then one tab per source **in alphabetical order** (only sources with packages appear; `Results` is prepended after a semantic search). Active tab highlighted.
- **Filter input** (inline next to tabs): `/query█` when active.
- **Bottom panels** (3 equal columns):
  - **Left**: Description (or location info when a group is selected).
  - **Center**: Metadata key-value pairs (Name, Version, Source, Location, User, Size, Added, Last Used).
  - **Right**: Keybindings reference.
- **Status bar** (bottom strip): Context info — selected package, scanning state, or errors.

### 6.3 Interaction
| Key | Action |
|-----|--------|
| `↑` / `↓` / `k` / `j` | Navigate tree |
| `→` / `l` / `space` | Expand group |
| `←` / `h` | Collapse group |
| `Tab` / `Shift+Tab` | Next / previous source tab |
| `/` | Start filter input |
| `?` | Ask whatsinstalled (experimental): semantic search → Results tab |
| `Esc` | Clear filter / close detail / cancel search |
| `Enter` / `d` | Show detail view (highlights Description panel) |
| `r` | Force rescan all managers (background) |
| `q` / `Ctrl+C` | Quit |

### 6.6 Search ("Ask whatsinstalled")
Press `?` to open the centered "Ask whatsinstalled" modal and type a query:
- As you type, an instant **substring preview** (case-insensitive, name or
  description) is shown for immediate feedback.
- `Enter` runs **semantic search** — embedding cosine similarity (keyword boost
  disabled by default; eval shows it hurts MRR) — over all packages and shows the
  ranked hits in the **Results** tab. `Esc` cancels and clears.
- Descriptions + embeddings are pre-computed at startup and cached, so search is
  one query-encode + in-memory scoring (fast, cannot hang). If the model or
  embeddings are unavailable, it falls back to the substring matches.
- Ranking lives in the pure `internal/search` package (`search.Rank`); its quality
  is measured by the `whatsinstalled eval` harness (`internal/search/eval`).

### 6.4 Detail View
- Pressing `d` highlights the Description panel title (`▸ Description`).
- Press `Esc` to exit detail view.
- No modal overlay — all state changes are inline.

## 7. CLI Commands

| Command | Description |
|---------|-------------|
| `whatsinstalled` | Launch TUI (default) |
| `whatsinstalled scan` | Force rescan all managers, print summary, exit |
| `whatsinstalled eval [--synthetic N] [--variant ...] [--baseline f] [--out f]` | Score semantic-search ranking (MRR/Hit@k) |
| `whatsinstalled --version` | Print the version |

Note: install/uninstall actions were removed — whatsinstalled is read-only (inventory
+ search). Core sources scanned: `apt`, `snap`, `npm`, `pip`, `conda`, `bin`;
plus, via the `scanner.AllScanners` registry: `pixi`, `pipx`, `uv`, `go`,
`docker`, `podman`, `brew`, `cargo`, `gem`, `pnpm`, `yarn`, `pacman`, `yay`,
`flatpak`, `nix`, `appimage`.

Each scanner appears as a TUI tab only when it is installed AND actually has
packages (`DiscoverScanners` → `IsAvailable() && Probe()`; `buildTabs` emits a
tab only when `counts[name] > 0`), so the tab strip is fully dynamic per host.

- **pipx**: isolated Python CLI apps (`pipx list --json`, `~/.local/share/pipx/venvs/*`).
- **uv**: tools from `uv tool install` (`uv tool list`, `~/.local/share/uv/tools/*`).
- **gem**: locally installed RubyGems (`gem list --local`).
- **pnpm / yarn**: globally installed JS packages (`pnpm ls -g --json`, `yarn global list`).
- **podman**: local container images (mirrors the docker scanner).
- **appimage**: portable `*.AppImage` apps in `~/Applications`, `~/Downloads`,
  `~/.local/bin`, `/opt` — tracked by no package manager.

The apt scanner sets `auto_installed` from `apt-mark showmanual` (the dpkg
`${Auto-Installed}` field is unreliable). The TUI hides auto-installed
dependency packages by default; press `a` to toggle. Filtering happens in the
store (`List`/`Search`/`CountBySource` take a `hideAuto` flag) so tab counts and
rows stay consistent.

All commands accept `--db <path>` to override the default database location (`~/.whatsinstalled.db`).

## 8. Database

- **Engine**: SQLite via `modernc.org/sqlite` (pure Go, no CGO).
- **Path**: `~/.whatsinstalled.db` or `WHATSINSTALLED_DB` env var.
- **Schema**: Must support migrations (e.g., adding `user` column to existing DBs).
- **Cache strategy**: Upsert on scan, purge stale records after scan completes.

## 9. Styling

- Dark theme (`#1a1b26` background).
- Rounded borders on panels.
- Accent color (`#7aa2f7`) for active elements.
- Selected row: bold white on accent background.
- Group nodes: bold accent color.
- Auto-installed packages (if shown): dim + italic.

## 10. Error Handling

- Scanner failures are logged to stderr and skipped (don't crash the app).
- SQLite busy errors must be prevented via WAL mode + busy timeout.
- `dpkg-query` exit errors: check `ExitError.Stderr` (bytes, not string).

## 11. Testing

- Table-driven tests for store CRUD operations.
- Use `t.TempDir()` for temporary test databases.
- Test parsing logic in isolation with mock command output.
- Every public function gets at least a doctest.

## 12. Build & Deployment

- Go 1.25+
- Single binary output: `go build -o whatsinstalled ./cmd/whatsinstalled`
- No external runtime dependencies.
