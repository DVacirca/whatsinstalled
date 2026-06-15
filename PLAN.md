# installr — TUI Dashboard

## Goal
Build `installr`, a Go/Cobra/Bubble Tea TUI that lists all system packages across 6 sources (apt, snap, npm, pip, conda, bin). The TUI has a clear tabbed layout with distinct panes, supports filtering, semantic search, shows package metadata including last-used time, and allows installing and uninstalling packages.

## Tech Stack
- Go 1.25+
- Cobra (command routing)
- Bubble Tea, Bubbles, Lip Gloss (TUI)
- SQLite via modernc.org/sqlite (cache, local state)
- Cybertron (pure-Go HuggingFace transformers) for semantic search embeddings
- Table-driven tests with Go stdlib

## Data Model
SQLite table `packages` (cached view of what is installed):

| Column | Type | Notes |
|--------|------|-------|
| id | INTEGER PK | |
| name | TEXT NOT NULL | |
| version | TEXT | |
| source | TEXT NOT NULL | apt / snap / npm / pip / conda / bin |
| location | TEXT NOT NULL | `system` or absolute path to the project/venv |
| size_bytes | INTEGER | disk usage; null if unavailable |
| description | TEXT | one-line summary; null if unavailable |
| installed_at | TEXT (ISO8601) | null if unavailable |
| auto_installed | INTEGER (0/1) | apt dependency flag |
| updated_at | INTEGER (Unix ms) | when the row was last refreshed |
| last_used | INTEGER | Unix ms access time (atime) of package files; unreliable on noatime mounts |
| added_at | INTEGER | Unix ms mtime of package files; reliable install/update time |
| embedding | TEXT | JSON float64 array for semantic search |
| user | TEXT | who installed the package |

Unique key: `(name, source, location)`

## Scanning Strategy
1. **Startup:** Load from SQLite cache immediately so the UI appears fast.
2. **Background scan:** After initial render, kick off a background scan of all managers in parallel:
   - `apt` → parse `dpkg-query -W` for all installed packages (manual + auto dependencies)
   - `snap` → parse `snap list`
   - `npm global` → parse `npm list -g --depth=0 --json`
   - `pip global` → parse `pip list --format=json` (system Python)
   - **Local envs:**
      - npm: Walk `~/*` depth 1. For each dir with `package.json`, parse `dependencies` + `devDependencies`.
      - pip: Walk `~/*` depth 1. For each dir containing `.venv/`, `venv/`, or `env/`, run `<venv>/bin/pip list --format=json`.
   - **bin**: Scan `~/.local/bin`, `~/bin`, `~/go/bin`, `~/.cargo/bin`, `~/.yarn/bin`, `~/.npm-global/bin`, `~/.nvm/versions/node/*/bin`, `~/.rbenv/shims`, `~/.pyenv/shims`, and any PATH directory under `$HOME` for executable files.
   - If running as root, also scan system-level locations.
3. **Merge:** Upsert into SQLite. If a package is missing from the new scan but exists in DB, mark stale and remove on next full refresh.
4. **Top-level only:** Do NOT recurse into sub-dependencies.
5. **Size:**
   - apt: `Installed-Size` field (KB)
   - snap: `snap list` Size column
   - npm/pip: `npm ls --depth=0` / `pip show`; fallback to `du -sh` on the package dir
   - If all fail, store null.
6. **Description:**
   - apt: `dpkg-query` Description field
   - snap: `snap info` summary
   - npm: `package.json` description
   - pip: `pip show` Summary field
   - conda: `channel` field
   - bin: none (binary name is self-describing)
7. **Last used:**
   - Determined from `atime` (access time) of the package directory or installation file.
   - Fallback to `mtime` if `atime` is unavailable.
   - Stored as Unix ms in `last_used` column.

## UI Layout (Bubble Tea)
Three distinct bordered regions:

```
┌─ installr ── total:86 ───────────────────────────────────────┐
│ [ All ] [ Apt ] [ Snap ] [ Npm ] [ Pip ]  /filter...         │
├───────────────────────────────────────────────────────────────┤
│ Name           Version    Source   Location       Size        │
│▶ nginx          1.24.0    apt      system         4.2M      │
│  lodash         4.17.21   npm      ~/webapp       2M        │
├───────────────────────────────────────────────────────────────┤
│ ↑↓ nav | tab switch | / filter | ? ask (exp) | d detail |    │
│ r refresh | q quit                                           │
└───────────────────────────────────────────────────────────────┘
```

- **Header row:** App title + total count. Styled with subtle accent.
- **Tab bar:** `All | Results | Apt | Snap | Npm | Pip | Conda | Bin`. Current tab has filled background + bold. `Tab` / `Shift+Tab` cycles. Active filter input lives to the right.
- **Table:** Main content. Columns `Name | Version | Source | Location | Size`. Sortable by pressing `s` to cycle columns (name → source → size → installed_at → name). `↑↓` (or `jk`) to navigate. Selected row is highlighted.
- **Footer:** Context-sensitive key hints. If a row is selected, also show: `Selected: nginx (apt, system)`.

## Modals (Lipgloss overlay)
- **Detail modal (key `d` / `Enter`):** Centered box with:
  ```
  ┌─ nginx ──────────────────────────────┐
  │ high-performance web server           │
  │ Version: 1.24.0                       │
  │ Source: apt                           │
  │ Location: system                      │
  │ Size: 4.2M                            │
  │ Installed: 2024-03-15                 │
  │ Auto-installed: no                    │
  │                                       │
  │ Press Esc or Enter to close           │
  └───────────────────────────────────────┘
  ```

## Keybindings
| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Next / previous tab |
| `↑` / `↓` (or `k` / `j`) | Navigate table |
| `/` | Focus filter input (type to filter current tab) |
| `?` | Open semantic search modal (natural language query) |
| `Esc` | Clear filter, close modal, cancel search, or unfocus filter |
| `Enter` / `d` | Open detail modal |
| `r` | Force rescan all managers (background) |
| `q` / `ctrl+c` | Quit |

## Semantic Search ("Ask installr")
- `?` opens the "Ask installr" modal: typing shows an instant substring preview,
  `Enter` runs the semantic search and shows ranked hits in the **Results** tab.
- Uses `sentence-transformers/all-MiniLM-L6-v2` (~22MB) via Cybertron (pure-Go);
  package text is embedded as 384-dim vectors with source context ("python
  package", "node javascript package", etc.).
- Descriptions + embeddings are pre-computed at init and cached in SQLite, so a
  query is just one encode + in-memory scoring — no enrichment/embedding in the
  hot path (fast, cannot hang). Falls back to a substring match if the model or
  embeddings are unavailable.
- Ranking is hybrid (cosine similarity + keyword boost), implemented in the pure
  `internal/search` package (`search.Rank`) shared by the TUI and the eval harness.
- Quality is measured with `installr eval` (`internal/search/eval`): MRR / Hit@k
  over a curated + synthetic golden set, with variant sweeps and baseline diffs.

## CLI Commands (Cobra)
- `installr` → Launch TUI (default)
- `installr scan` → Force rescan, print summary to stdout, exit
- `installr eval [--synthetic N] [--variant ...] [--baseline f.json] [--out f.json]` → score search ranking (MRR/Hit@k)
- `installr --version` → print the version

(installr is read-only — install/uninstall actions were removed.)

## Architecture / File Layout
```
installr/
├── cmd/installr/main.go
├── internal/
│   ├── cmd/
│   │   ├── root.go           # Cobra root, global flags (--db)
│   │   ├── subcommands.go    # scan
│   │   └── eval.go           # `eval` — search-ranking metrics harness
│   ├── store/
│   │   ├── store.go          # SQLite init, CRUD, embeddings, enrichment_cache
│   │   └── store_test.go
│   ├── scanner/
│   │   ├── scanner.go        # Scanner interface
│   │   ├── discovery.go      # AllScanners registry + DiscoverScanners
│   │   ├── apt/snap/npm/pip/conda/bin.go        # core managers
│   │   └── pixi/pipx/uv/go/docker/podman/brew/cargo/gem/pnpm/yarn/pacman/yay/flatpak/nix/appimage.go  # extra managers
│   ├── enrich/              # description enrichment (local tools → PyPI/npm APIs, cached)
│   ├── nlp/
│   │   ├── embedder.go       # sentence-transformers embedder, cosine
│   │   └── search.go         # query expansion + keyword score
│   ├── search/
│   │   ├── rank.go           # pure search.Rank (shared by TUI + eval)
│   │   └── eval/             # MRR/Hit@k metrics, queries.json golden set, report/diff
│   ├── tui/
│   │   ├── dashboard.go      # Bubble Tea Model, tabs, search wiring
│   │   ├── tree.go           # tree view; styles.go; themes
│   │   └── *_test.go
│   └── pkg/
│       └── env.go            # CWD, home dir, path helpers, last-used
├── go.mod
└── go.sum
```

## Implementation Order
1. Bootstrap module + Cobra root command
2. SQLite store schema + basic CRUD
3. Scanner interface + apt / snap / npm / pip / conda / bin implementations
4. Background scan orchestrator (goroutine + tea.Cmd)
5. TUI layout: header, tabs, tree, footer
6. Filter input + sort logic
7. Detail modal + confirm modal
8. Install / uninstall runner (tea.ExecProcess for TTY)
9. CLI subcommands (scan, install, uninstall)
10. Semantic search (Cybertron embeddings + cosine similarity)
11. Tests (table-driven for store + scanners + NLP)

## Testing Strategy
- **Store:** Table-driven tests for CRUD, upsert, stale cleanup. Use temp dirs + `t.TempDir()`.
- **Scanners:** Mock stdout from package manager commands; test parsing logic in isolation.
- **TUI:** Test `Update` state machine with typed messages (e.g., `dataLoadedMsg`, `scanCompleteMsg`).
- **Integration:** Build binary, run `installr scan`, assert stdout contains expected summary.

## Notes / Constraints
- Top-level packages only; no transitive dependency scanning.
- Local envs are scanned at `~/*` depth 1. If a subdir contains both `package.json` and `.venv/`, both npm and pip scanners will report packages for that path. This is correct.
- For npm/pip, `location` is the directory containing `package.json` / the venv, not the package dir itself.
- `auto_installed` is apt-only; always 0 for other sources.
- Use `t.TempDir()` and `filepath.Join` in tests; no OS-dependent paths.
