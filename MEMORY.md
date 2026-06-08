# Memory

## installr

### Go Module / Toolchain
- `go mod tidy` with no packages won't download dependencies. Must have at least one `.go` file before running it.
- `modernc.org/sqlite` v1.52.0 requires Go >= 1.25.0, which triggered an automatic toolchain upgrade to `go1.25.11`.
- `go.mod` started with `go 1.22.0` but was bumped to `go 1.25.0` by `go mod tidy`.

### SQLite (modernc.org/sqlite)
- Import as `_ "modernc.org/sqlite"` — driver name is `"sqlite"` (not `sqlite3`).
- `sql.NullInt64` works well for nullable integer columns like `size_bytes`.
- `ON CONFLICT(name, source, location) DO UPDATE SET ...` is the correct upsert syntax for SQLite.
- **database locked**: When background scan goroutine and UI queries run concurrently, SQLite returns `SQLITE_BUSY`.
  - `?_busy_timeout=5000` helps but isn't enough if the scan holds the DB longer than 5s (conda across 16 envs takes ~30s).
  - **Real fix**: enable WAL mode with `PRAGMA journal_mode=WAL;`. WAL allows readers to proceed while a writer holds the lock, which is the correct concurrency model for a background-scan + UI-read pattern.

### Package Manager Scanning
- **apt**: `dpkg-query -W -f='${Package}\t${Version}\t${Installed-Size}\t${Status}\t${Auto-Installed}\t${Description}\n'` gives tab-separated output. Must filter by `Status` containing `install ok installed`.
  - `dpkg-query`'s `Auto-Installed` field is unreliable (mostly empty). Use `apt-mark showmanual` to get actually user-installed packages, then cross-reference with dpkg-query.
- **snap**: `snap list` has a header line to skip. `snap info <name>` can provide a `summary:` field for descriptions.
- **npm**: `npm list -g --depth=0 --json` for global. For local, look for `package.json` in `~/*` depth 1 and run `npm list --depth=0 --json` in that directory. Parse both `dependencies` and `devDependencies`.
- **pip**: `pip list --format=json` is fast. `pip show <name>` per package is extremely slow (~7s for 5 packages = ~125s for 93). Avoid calling `pip show` in a loop during scanning.
  - For local venvs, look for `.venv/`, `venv/`, or `env/` in `~/*` depth 1 and use `<venv>/bin/pip`.
- **conda**: `conda env list --json` returns env paths. For each env, `conda list --json -p <env_path>` returns packages. Each package has `name`, `version`, `channel`, `build_string`. System-wide packages may number in the thousands.
  - Use `pkg.FileOwner(envPath)` to determine who owns the conda environment.
- **bin**: Scans executable files in `~/.local/bin`, `~/bin`, `~/go/bin`, `~/.cargo/bin`, `~/.yarn/bin`, `~/.npm-global/bin`, `~/.nvm/versions/node/*/bin`, `~/.rbenv/shims`, `~/.pyenv/shims`, `/usr/local/bin`, and `/usr/bin`. Also scans any PATH directory under `$HOME`. Filters by executable bit (`mode & 0111`).

### User Tracking
- Added `user` column to `packages` table.
- **apt/snap**: hardcoded `"system"` since these are system-wide.
- **npm/pip/conda**: use `pkg.FileOwner(path)` to get the owner of the project/venv directory. Falls back to `pkg.CurrentUser()`.
- Use `syscall.Stat_t` + `user.LookupId()` on Unix to resolve UID to username.

### Bubble Tea TUI
- `tea.WithAltScreen()` is needed for a proper fullscreen TUI.
- When filtering, intercept keys before passing to the table widget. Return early from `Update` to avoid table consuming filter keystrokes.
- Modal overlay: render the background first, then splice modal lines into the background at calculated center position. `lipgloss.Width()` is essential for accurate text width.
- `table.Model` from bubbles needs explicit `SetWidth`/`SetHeight` on `WindowSizeMsg`.

### Tree View
- Replaced flat `bubbles/table` with a custom tree grouped by `location`.
- Group nodes are expandable/collapsible with `→/l/space` (expand) and `←/h` (collapse).
- **Gotcha**: In Go `for _, p := range pkgs { &p }` captures the loop variable address, so all pointers end up pointing to the last element. Use `for i := range pkgs { p := pkgs[i]; &p }` or index directly.
- Tree renders manually with `lipgloss` — selected row gets full-width background highlight by padding with spaces to terminal width.
- Scroll offset tracks cursor position to keep selection visible when tree exceeds viewport height.
- **Top cut off**: When total rendered content exceeds terminal height, the terminal scrolls and the title bar gets pushed into scrollback (off-screen).
  - **Fix 1**: Put the title bar INSIDE the main bordered panel, not as a separate element above it. This way it scrolls with the panel content and is always visible.
  - **Fix 2**: Wrap the entire `View()` output in `lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, content)`. This clips content to the viewport and anchors it to the top-left, so excess content is clipped from the bottom instead of scrolling the top off-screen.
- **Height budget**: `Height(h)` on a bordered style sets the TOTAL height (content + borders). So `Height(5)` with borders = 5 total lines (3 content + 2 border).
  When calculating layout, `panelStyle.Width(w).Height(h).Render(...)` produces `h` lines total, NOT `h + 2`.
  **Current budget**: treeContentH = m.height - 12. This accounts for treePanelBorders(2) + titleBar(1) + separator(1) + headerRow(1) + tabBar(1) + bottomPanels(5) + status(1) = 12 fixed lines.

### General
- Keep imports clean — `go build ./...` quickly surfaces unused imports.
- For `exec.ExitError`, `Stderr` is a `[]byte`, not a `string`.
- **Always rebuild the binary** (`go build -o installr ./cmd/installr`) after making scanner changes. `go build ./...` only compiles packages, not the final binary. A stale `./installr` binary will silently ignore new scanners.

### Semantic Search (LLM Integration)
- **Cybertron + Spago**: `github.com/nlpodyssey/cybertron` is the Go package for running HuggingFace transformers. It depends on `spago` (pure-Go ML). After `go get`, run `go mod tidy` to resolve all transitive deps.
- **Model**: `sentence-transformers/all-MiniLM-L6-v2` — ~22MB download, 384-dimensional embeddings. First run downloads and caches to `~/.installr/models/`.
- **Embedding caching**: Compute once per package, store as JSON in SQLite `embedding` column. Subsequent queries are ~50-200ms.
- **Cosine similarity**: Score > 0.3 filters noise; top 20 results shown.
- **Key handling**: Space key (`msg.String() == " "`) must be explicitly caught in search mode, otherwise it falls through to the outer `case " "` (tree expand). Enter key works but must not be caught by `case "d", "enter"` in the outer switch — the search mode check must come first.
- **Modal overlay**: Use `lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)` to render a centered modal on top of the main content. The modal should be rendered last so it overlays everything.
- **Search feedback**: When a search takes time, the UI must show a "Searching..." state. Use `m.searching` boolean + `lipgloss.NewStyle().Foreground(accent).Render("⟳ Searching...")` in the modal and tree panel. The status bar should also show "⟳ searching...".
- **Background search**: `semanticSearch()` must return a `tea.Cmd` (not a `tea.Msg`) so it runs in a background goroutine. Use `return func() tea.Msg { ... }` pattern. The `Update` method receives the result message when the goroutine finishes.
- **Data enrichment**: `PackageText()` must include source context ("python package", "node javascript package", "debian system package") so the model understands what kind of package it is, not just the name. Without this, npm and pip packages get similar scores because the names don't carry enough semantic meaning.

### Install / Uninstall (tea.ExecProcess)
- **TUI uninstall was broken**: `cmd.Run()` in a goroutine corrupts the alternate screen buffer and breaks sudo password prompts.
- **Fix**: Use `tea.ExecProcess(cmd, callback)` which suspends the TUI, runs the command in the foreground, and resumes after completion. This is the correct pattern for any command that needs TTY access (sudo, apt, etc.).
- **InstallCmd / UninstallCmd**: The Scanner interface returns `*exec.Cmd` instead of running it directly. The TUI passes this to `tea.ExecProcess`, while the CLI uses `.Run()` directly.

## Session: Search Enrichment Implementation

### Search Strategy
- **Hybrid semantic search**: BERT embeddings (sentence-transformers/all-MiniLM-L6-v2) for natural language queries + lazy enrichment for missing descriptions.
- **Two modes**: `/` for fast substring filter (current tab), `?` for semantic search (all packages).
- **Lazy enrichment**: Only enrich descriptions when a search is triggered, not during scan. This keeps the initial scan fast (~2-3s) while making search work well.

### Enrichment Sources (local-first priority)
- **bin packages**: `whatis` (bulk, 0.1s for 7,720 entries) → `dpkg -S` (maps `/usr/bin/{name}` to apt package → apt description).
- **pip packages**: `pip show` (batch, ~7s for 93 packages) → PyPI API fallback.
- **npm packages**: `npm info --json` (local, ~1s for 14 packages) → npm registry fallback.
- **Remote APIs**: PyPI (`pypi.org/pypi/{name}/json`) and npm registry (`registry.npmjs.org/{name}`). 100ms delay between requests to be polite. 30-day cache TTL.

### Enrichment Cache
- **SQLite table**: `enrichment_cache(name, source, description, fetched_at)` with 30-day TTL.
- **Purpose**: Avoids repeated API calls. Before any HTTP request, check cache. If cache hit and < 30 days old, use cached description.
- **Store methods**: `ListWithoutDescriptions()`, `UpdateManyDescriptions()` (batch update in single transaction).

### UI Progress Reporting
- **Channel-based**: `startSearch()` creates a goroutine for enrichment and sends `enrichmentProgressMsg` through a channel. `pollProgressCmd()` reads from the channel and returns messages to the `Update` loop.
- **Modal display**: Search modal shows real-time progress: "Enriching 45/253 packages..." with live log lines showing each package being processed.
- **State flags**: `m.enriching` (shows enrichment UI), `m.searching` (shows searching UI), `m.logs` (stores last 20 log lines).

### UI Freezing Fix
- **Root cause**: `pollProgressCmd` blocked forever on the channel if the goroutine hung or crashed. The UI froze because Bubble Tea's `Update` loop never received the completion message.
- **Fix**: Added `select` with 5-second timeout in `pollProgressCmd`. If channel blocks longer than 5s, returns `enrichmentCompleteMsg{err: timeout}`.
- **State cleanup**: Escape key in search mode now clears `m.enriching = false`, `m.enrichCh = nil`, `m.logs = nil` to prevent stuck state.

### Logging to stderr breaks TUI
- **Issue**: `fmt.Fprintf(os.Stderr, ...)` and `log.Printf` in enrich code wrote raw text to stderr, which corrupted the alternate screen buffer (TUI display).
- **Fix**: Removed all stderr logging. Progress is reported through the Bubble Tea message channel instead. The only logs are the ones displayed in the search modal UI.

### Current Enrichment State
- **Total packages**: 3,859
- **With descriptions**: 3,606 (93.4%)
- **Missing**: 253 (6.6%) — mostly system binaries without man pages or custom tools
- **Coverage**: apt/snap/conda = 100%, pip = 97.8%, npm = 78.6%, bin = 89.3%

### Commits
1. `baseline before search enrichment` — original state
2. `add lazy enrichment with caching for semantic search` — core enrichment package
3. `fix progress reporting in search flow` — Bubble Tea state handling
4. `fix embedding retrieval and add enrichment tests` — bug fix + 700 lines of tests
5. `add integration tests for enrichment with real data` — verified with production DB
6. `add logging to enrichment and search flow` — removed stderr, added UI logs
7. `fix ui freezing during search by adding timeout and proper state transitions` — frozen UI fix
