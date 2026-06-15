# Memory

## whatsinstalled

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
- **pipx**: `pipx list --json` → `venvs.<app>.metadata.main_package.{package,package_version}`. Location `~/.local/share/pipx/venvs/<app>`.
- **uv**: parse `uv tool list` — `name vX.Y.Z` lines; indented `- app` lines (binaries) are skipped. Location `~/.local/share/uv/tools/<name>`.
- **gem**: `gem list --local` → `name (1.2.3, 1.0.0)` or `name (default: 0.1.1)`; take first version, strip `default: `. Location from `gem environment gemdir`.
- **pnpm**: `pnpm ls -g --depth=0 --json` → array of `{dependencies:{name:{version}}}`. Empty when no globals (tab won't show).
- **yarn** (v1): parse `yarn global list` — `info "name@version" has binaries:` lines; split on the LAST `@` to support `@scope/pkg`.
- **podman**: copy of the docker scanner (`podman images --format {{json .}}`).
- **appimage**: always `IsAvailable`; scans `~/Applications`, `~/Downloads`, `~/.local/bin`, `/opt` (depth 1) for `*.AppImage`. `splitAppImageName` strips the suffix and best-effort extracts a trailing `-1.2.3` version (regex `[-_]v?\d[\w.]*$`).
- New scanners need no TUI changes: register in `scanner.AllScanners` and `buildTabs` renders a tab only when `counts[name] > 0`. Text-parsing scanners have `parseX` helpers unit-tested in `parse_test.go`.
  - **Tab order** is alphabetical (after the "All" tab) — NOT the registry order. `buildTabs` collects sources with `counts > 0` and `sort.Strings` them. Tests in `internal/tui/tabs_test.go`.
- **auto-installed filter**: `store.List/Search/CountBySource` take a `hideAuto bool` (shared `whereClause` helper adds `auto_installed = 0`). TUI model `hideAuto` defaults true; key `a` and palette "Deps" toggle it, then reload via `loadData`. Keeps tab counts consistent with rows.

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
- **Always rebuild the binary** (`go build -o whatsinstalled ./cmd/whatsinstalled`) after making scanner changes. `go build ./...` only compiles packages, not the final binary. A stale `./whatsinstalled` binary will silently ignore new scanners.

### Semantic Search (LLM Integration)
- **Cybertron + Spago**: `github.com/nlpodyssey/cybertron` is the Go package for running HuggingFace transformers. It depends on `spago` (pure-Go ML). After `go get`, run `go mod tidy` to resolve all transitive deps.
- **Model**: `sentence-transformers/all-MiniLM-L6-v2` — ~22MB download, 384-dimensional embeddings. First run downloads and caches to `~/.whatsinstalled/models/`.
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
- **All sources now route** via `Enricher.descMapForSource` (enrich.go), not just the original 6. Mappings: `pipx`/`uv` → PyPI (pip path); `pnpm`/`yarn` → npm path; `cargo` → crates.io; `gem` → rubygems.org; `brew` → `brew info --json=v2`; `pacman`/`yay` → `pacman -Qi`. Sources with no meaningful registry description (docker, podman, appimage, nix, go) fall through to an empty map.
- **crates.io requires a `User-Agent` header** or it 403s — `RemoteEnricher.fetchJSON` always sets one. Registry base URLs (`cratesURL`, `rubygemsURL`) are struct fields so tests can point them at httptest.
- Brew/pacman parsing is split into pure `parseBrewJSON`/`parsePacmanInfo` so it's unit-testable without the binaries installed (`local_test.go`, `remote_test.go`).

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

### Embedding Computation Freezing Fix
- **Root cause**: `runSemanticSearch` computed embeddings for ALL packages synchronously in a loop (90 packages * ~50ms = ~4.5s). This blocked the goroutine with no UI updates, causing the modal to show "Searching..." with no progress.
- **Fix**: Extracted embedding computation into `computeEmbeddings()` that runs in a background goroutine with channel-based progress reporting (same pattern as enrichment). The `search()` function now only does scoring and ranking on packages with existing embeddings.
- **State flow**: `startSearch()` → `enrich()` (if needed) → `computeEmbeddings()` (if needed) → `search()` → `semanticSearchResult`. Each step returns `enrichmentProgressMsg` with a channel so the UI stays responsive.
- **Progress display**: Embedding computation shows "Enriching 45/90 packages..." in the modal (reuses the same `m.enriching` UI state) with live log lines showing each package.

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
8. `extract embedding computation into async step with progress reporting` — embedding computation no longer blocks UI

### Session: Enrichment Hang + Search Modal Stuck Fixes

#### Subprocess timeouts (primary freeze root cause)
- **Root cause**: All `exec.Command().Output()` calls in `internal/enrich/local.go` had NO timeout. If any subprocess hung (snap lock, npm network issues, conda slowness), the enrichment goroutine blocked forever.
- **Fix**: Added `runCmd(timeout, name, args...)` helper using `exec.CommandContext` with 30s timeout for single-package commands (snap info, npm info, conda search) and 60s for bulk commands (pip show, apt show, dpkg).
- **File**: `internal/enrich/local.go`

#### Per-package progress feedback
- **Root cause**: `enrichSource()` in `enrich.go` called bulk methods then reported all progress after the entire source batch completed. For snap/npm/conda (per-package loops), the user saw no progress until all packages for that source finished.
- **Fix**: Changed `enrichSource()` to iterate per-package for snap/npm/conda using new single-package helpers (`enrichSingleSnap`, `enrichSingleNpm`, `enrichSingleConda`). Progress callbacks fire per-package so the user sees `[snap] firefox` etc. as enrichment happens.
- **File**: `internal/enrich/enrich.go`, `internal/enrich/local.go`

#### scanErrorMsg not clearing search state (critical bug)
- **Root cause**: When any error occurred in the search pipeline (enrichment, embedding, or search), `scanErrorMsg` was returned but the handler only set `m.scanErr` and cleared `m.scanning`. It NEVER cleared `m.searching` or `m.mode`, so the "Searching..." modal stayed visible forever with no feedback.
- **Fix**: `scanErrorMsg` handler now clears `m.searching`, `m.enriching`, `m.enrichCh`, `m.mode`, cancels enrichment goroutine, and reloads data to return to tree view.

#### Double isDone messages causing race condition
- **Root cause**: Enrichment goroutine sent TWO `isDone: true` messages — one BEFORE DB update (triggering premature `startSearch()`) and one AFTER. The premature `startSearch()` could re-enter enrichment if DB wasn't updated yet. The second isDone could clobber `m.enrichCh` set by `computeEmbeddings` goroutine, breaking the channel chain.
- **Fix**: Consolidated to a single `isDone: true` after DB update completes.

#### enrichmentCompleteMsg with no error hangs UI
- **Root cause**: When channel closed gracefully without a preceding isDone message, `enrichmentCompleteMsg{}` (no error) did nothing — no search triggered, but `m.searching` stayed true.
- **Fix**: When no error, `enrichmentCompleteMsg` handler now triggers `m.startSearch()`.

#### computeEmbeddings infinite loop on all failures
- **Root cause**: If `embedder.Encode()` failed for every package, no embeddings were stored, so `ListWithoutEmbeddings()` always returned the same packages, creating an infinite loop of computeEmbeddings → isDone → startSearch → computeEmbeddings.
- **Fix**: Added `anySuccess` tracking. If no embeddings succeeded, sends `isDone` with `err` field set, which the handler treats as a fatal error (clears search state, shows error).
- **Also**: Added 30s `context.WithTimeout` to `embedder.Encode()` call in `computeEmbeddings` (was using `context.Background()` with no timeout).

#### ESC now cancels enrichment goroutine
- **Root cause**: Pressing ESC during enrichment just cleared UI state but left the goroutine running (leaked goroutine, channel blocked after 100 messages).
- **Fix**: Added `m.cancelEnrich context.CancelFunc`. `enrich()` creates a cancellable context and stores the cancel func. ESC calls it. Goroutine checks `ctx.Done()` at key points and returns early. Also: `defer cancel()` in goroutine cleanup.

#### Poll timeout increased 5s → 15s
- Reduced false-positive timeouts for legitimate slow operations (remote HTTP with 100ms delays between packages).

#### Removed inline embedding from search()
- The old `runSemanticSearch()` computed embeddings for missing packages inline during search. Now `computeEmbeddings()` handles this as a separate async step with progress feedback, and `search()` only does scoring/ranking.

### Session: Results Tab + Apt Scanner Fix

#### Infinite enrichment loop (root cause of search hang)
- **Root cause**: After enrichment ran, `UpdateManyDescriptions` skipped packages with empty descriptions. Next `ListWithoutDescriptions` still returned them. `startSearch` dispatched again → enrichment ran again → same 253 packages → looped forever.
- **Fix**: `startSearch(skipEnrichment bool)` — first call allows enrichment, all subsequent calls skip it and proceed straight to embeddings/search.
- **File**: `internal/tui/dashboard.go`

#### Results tab for semantic search
- **Added**: `"results"` source at tab index 1, between "All" and "Apt". `loadData()` returns `m.semanticResults` when on Results tab. Search completion switches `m.tabIndex = 1` and dispatches `loadData`.
- **State**: New searches clear `m.semanticResults = nil` on Enter. `dataLoadedMsg` preserves results on Results tab (prevents stale tab-switch messages from overwriting tree).
- **File**: `internal/tui/dashboard.go`

#### Apt scanner includes auto-installed dependencies
- **Changed**: `AptScanner.Scan()` now calls `scanAll()` directly, including all installed dpkg packages (manual + auto). Previously filtered to `apt-mark showmanual` only.
- **File**: `internal/scanner/apt.go`

#### Panic recovery in all goroutines
- **Added**: `defer recover()` in `startSearch` closure, enrichment goroutine, and embedding goroutine. Prevents silent goroutine death from leaving UI hung.
- **File**: `internal/tui/dashboard.go`

#### 60-second search timeout safety net
- **Added**: `searchTimeoutMsg` dispatched via `tea.Tick(60s)` when search starts. If search hasn't completed, force-clears all state and returns to tree view with error.
- **File**: `internal/tui/dashboard.go`

### Session: Scanner Description Fixes

#### Pip scanner: descriptions from `pip show` during scan
- **Root cause**: `pip list --format=json` returns only name+version, no description. Enrichment later called **system** `pip show` which couldn't see virtualenv packages.
- **Fix**: `scanWithPip()` now runs `<venv>/bin/pip show <names...>` in bulk immediately after `pip list`, extracting the `Summary` field for every package. One bulk call per venv, ~1-2s.
- **File**: `internal/scanner/pip.go`

#### Bin scanner: whatis + directory hints + `--help` fallback
- **Root cause**: Bin scanner got filenames only. `whatis` in enrichment works for standard tools but misses custom builds, language shims, and anything without a man page.
- **Fix**: Post-scan enrichment in `BinScanner.Scan()`:
  1. **whatis batch**: runs `whatis <name1> <name2> ...` on all discovered binaries at once
  2. **Directory hints**: `~/.cargo/bin` → "Rust binary tool", `~/go/bin` → "Go binary tool", `.pyenv/shims` → "Python version manager shim", etc.
  3. **`--help` fallback**: runs `<binary> --help` for up to 20 remaining packages, takes the first line if it looks like a description (10-200 chars).
- **File**: `internal/scanner/bin.go`

#### Conda scanner: METADATA file reading
- **Root cause**: `conda list --json` has no description field. Scanner stored only `"channel: conda-forge"`. `conda search` enrichment is slow and usually returns nothing.
- **Fix**: `scanEnv()` now walks `envPath/lib/python3.x/site-packages/` looking for `<name>-*.dist-info/METADATA` files and extracts the `Summary:` line. This is the same metadata PyPI uses, so it's accurate.
- **File**: `internal/scanner/conda.go`

#### Npm scanner: package.json fallback
- **Root cause**: `npm list --depth=0 --json` sometimes has `_description`, often empty.
- **Fix**: After checking npm list JSON, falls back to reading `node_modules/<name>/package.json` and extracting the `description` field.
- **File**: `internal/scanner/npm.go`

### Session: Background Scan Progress Feedback

#### Scan felt unresponsive (no UI feedback)
- **Root cause**: `backgroundScan()` ran all 6 scanners sequentially in a single blocking call. For systems with thousands of apt packages, this could take 30+ seconds with zero feedback. Status bar just showed "⟳ scanning..." with no indication of which scanner was running or how many packages found.
- **Fix**: Converted `backgroundScan()` to channel-based async flow (same pattern as enrichment):
  - `backgroundScanWithProgress()` spawns a goroutine, sends `scanProgressMsg{source: "apt"}` before each scanner, then `scanProgressMsg{source: "apt", count: 1847}` after completion
  - `pollScanProgressCmd()` reads from the channel and delivers messages to the Update loop
  - Status bar now shows: `⟳ scanning apt... 1847`
  - 30-second timeout per scanner to prevent hangs
- **File**: `internal/tui/dashboard.go`

### Session: Pre-Compute Everything on Init

#### Search was slow because enrichment + embeddings happened during query
- **Root cause**: `startSearch()` would check for missing descriptions, run enrichment if needed, then compute embeddings if needed, THEN search. This could take 30-60 seconds on first query. The async chain (enrichment → channel → poll → embedding → channel → poll → search) was fragile and caused hangs.
- **Fix**: Moved the entire pipeline (scan → enrich → embed) to init. `fullInitWithProgress()` runs all three phases in a single goroutine during startup. Search becomes instant — just query embedding + scoring.
- **Init pipeline**:
  1. Scan all 6 sources (with per-scanner progress)
  2. Enrich ALL missing descriptions in one batch
  3. Compute ALL missing embeddings in one batch
  4. App is ready
- **Search is now**: `startSearch()` → `search()` → `semanticSearchResult` (no enrichment, no embedding computation)
- **UI shows init progress**: `⟳ Scanning apt... 1847` → `⟳ Enriching descriptions... 253` → `⟳ Computing embeddings... 1800`
- **Search blocked during init**: If user presses Enter while initializing, modal shows "⟳ Still initializing, please wait..."
- **Files**: `internal/tui/dashboard.go`

### Session: Splash Screen on Launch

#### User wanted a splash screen during init with feedback
- **Root cause**: Init progress was only shown in the tree panel area as plain text, not a proper centered splash. User expected a clear "Updating packages" overlay on launch.
- **Fix**: Added a centered splash screen overlay (`modalBorderStyle`) that covers the entire screen during init:
  - Title: "whatsinstalled"
  - Subtitle: "⟳ Updating packages"
  - Dynamic progress: "Scanning apt... 1847 found (1847 total)", "Enriching descriptions... 253/253 packages", "Computing embeddings... 1800 packages"
  - The splash disappears when init completes
- **Only new/absent data**: `ListWithoutDescriptions` and `ListWithoutEmbeddings` are used in init, so only packages missing descriptions/embeddings are processed. If data is already present, those phases are instant.
- **Fields added**: `initProgress string`, `initCh chan scanProgressMsg`, `totalFound int` to model
- **Files**: `internal/tui/dashboard.go`

### Session: Splash Screen Polling Fix

#### Splash stayed at "0 packages" and never updated
- **Root cause**: The `pollScanProgressCmd` had a 30-second timeout. `AptScanner.Scan()` on systems with thousands of packages could take >30s, causing the timeout to fire. Worse, the handler broke the poll loop after the first message because channel messages had `ch == nil`, so it never called `pollScanProgressCmd` again.
- **Fix**: 
  1. Store the channel in `m.initCh` when the envelope arrives
  2. For channel messages, if `!isDone && m.initCh != nil`, return another `pollScanProgressCmd(m.initCh)` to keep the loop alive
  3. Increased timeout from 30s → 120s per poll
  4. Added panic recovery to the init goroutine
  5. Removed `helpFallback` from bin scanner (ran up to 20 `--help` subprocess calls = up to 40s)
- **Splash now shows running total**: "Scanning apt... 1847 found (1847 total)" → "Scanning pip... 93 found (1940 total)"
- **Files**: `internal/tui/dashboard.go`, `internal/scanner/bin.go`

### Session: Init Speed + Second Launch Fix

#### Second launch rescanned everything (took 2 min again)
- **Root cause**: Init always ran the full 6-scanner scan regardless of whether data existed in DB. Second launch on an already-populated DB rescanned all 3000+ packages.
- **Fix**: `fullInitWithProgress()` now calls `db.Count()` first. If >0 packages exist, it skips the scan phase entirely and goes straight to enrich + embed. Second launch is instant (just a DB count query).
- **Message shown**: "Using cached data... 3000 packages" instead of re-scanning.

#### 2-minute init felt stuck with no visible progress
- **Root cause**: 
  1. Splash showed only ONE line (current progress), so embedding 1800 packages at ~50ms each felt frozen
  2. Embedding progress was only sent every 10 packages, so updates were ~5 seconds apart
- **Fix**: 
  1. Splash now shows a log history (`initLogs []string`) — last 6 messages scroll in real-time:
     ```
     Computing embeddings... 1/1800
     Computing embeddings... 2/1800
     Computing embeddings... 3/1800
     ...
     ```
  2. Per-package embedding progress: sends a message for EVERY package, not every 10
- **Files**: `internal/tui/dashboard.go`

### Session: Enrichment Progress Clarity

#### "Enriching descriptions" appeared 4 times with no differentiation
- **Root cause**: The enrichment callback sent `scanProgressMsg{source: "enrich", count: done}` for every package. All messages looked identical on the splash screen.
- **Fix**: 
  1. Enrichment source is now prefixed: `"enrich:apt"`, `"enrich:pip"`, etc.
  2. Handler parses the prefix and shows: `"Enriching pip... 45 done"`, `"Enriching apt... 12 done"`
  3. Throttled to every 5 packages (not every single one) to avoid flooding the log
- **Files**: `internal/tui/dashboard.go`

### Session: Results Tab Conditional + First Position

#### Results tab was always visible, not first
- **Root cause**: `tabSources` / `tabLabels` hardcoded `"results"` at index 1. It was always rendered regardless of search state.
- **Fix**:
  1. Removed `"results"` from static `tabSources` / `tabLabels` arrays
  2. Added `visibleTabSources()` / `visibleTabLabels()` methods — prepend `"results"` only when `m.semanticResults != nil`
  3. Added `currentSource()` helper so all tab-index lookups go through the visible list
  4. On search completion: `m.tabIndex = 0` (Results is now first tab)
  5. Tab cycling uses `len(m.visibleTabSources())` instead of fixed `len(tabSources)`
- **Behavior**:
  - Before any search: tabs are `All | Apt | Snap | Npm | Pip | Conda | Bin`
  - After search completes: tabs are `Results | All | Apt | Snap | Npm | Pip | Conda | Bin`
- **Files**: `internal/tui/dashboard.go`

### Session: UI Polish — Consistent Professional Look

#### Inconsistent two-tone effect, heavy borders, patchy backgrounds
- **Root cause**:
  1. `tableBorderStyle` wrapped the tree in a rounded border, but inner elements had mismatched backgrounds (`bgLight` header, no-bg cells)
  2. Bottom area had 3 separate `panelStyle` bordered panels → border noise where panels touched
  3. Tree loading states padded with raw spaces (no background) creating visible gaps
  4. Title bar had no full-width background → terminal default showed through on the right
  5. `tableCellStyle` / `tableSelectedStyle` / `tableHeaderStyle` had `Padding(0, 1)` → extra width causing misalignment
- **Fix**:
  1. **Removed outer border** on tree area — replaced with a single thin `separatorStyle` horizontal rule
  2. **Unified bottom area** — single `bottomPanelStyle` with `│` column dividers instead of 3 separate bordered panels
  3. **Full-width backgrounds** — every tree line is exactly `sepWidth` chars with `Background(bg)`; loading states use `tableCellStyle.Render(blankLine)` for uniform fill
  4. **Title bar padded to full width** with `Background(bg)` so no gaps
  5. **Removed `Padding(0, 1)` from table styles** — content itself is padded to exact width, styles only set colors
  6. **Added `tableGroupStyle`** for group nodes instead of inline ad-hoc style
  7. **Polished modal/splash titles** — removed stray spaces around titles
- **Behavior**:
  - Single consistent `#1a1b26` background across the entire app
  - Only `#24283b` used for elevation: header row, tab bar, status bar
  - Selected row pops with `#7aa2f7` accent bg — the only strong color in the content area
  - Bottom area is one clean panel with subtle internal dividers
- **Files**: `internal/tui/styles.go`, `internal/tui/tree.go`, `internal/tui/dashboard.go`

### Session: Strict Two-Tone Color Scheme

#### Still inconsistent — too many background colors
- **Root cause**: Multiple background colors (`#1a1b26`, `#24283b`) used in many places without clear rules. Bottom panel had its own bg, header had its own bg, etc.
- **Fix**: Rigid two-tone system:
  - **Primary** `#1a1b26` — everything that is NOT the main body: title bar, header, separator, tab bar, status bar, bottom panel, modals, dividers.
  - **Secondary** `#24283b` — ONLY the main table body (content area where packages are listed).
  - All styles renamed with `shell*` / `body*` prefixes so it's impossible to mix them up.
  - Removed `bgLight`, `bgHover`, `accent2`, `countAccentStyle` — no more color proliferation.
- **Behavior**:
  - The app now has exactly two background colors.
  - Primary (dark) frames the entire UI.
  - Secondary (slightly lighter) fills only the central package list.
  - Selected row uses accent bg + primary fg — the only strong contrast in the body.
- **Files**: `internal/tui/styles.go`, `internal/tui/tree.go`, `internal/tui/dashboard.go`

### Session: New Scanners — Pixi, Go, Docker

#### Missing package managers on the system
- **What was missing**: pixi (conda-like Python env manager), Go module cache, Docker images.
- **Fix**: Added three new scanners following the existing `Scanner` interface pattern:
  1. **PixiScanner** (`scanner/pixi.go`): Scans `~/.pixi/envs` global environments and local `pixi.toml` projects (depth ≤2 from home). Uses `pixi list --json`. Install/uninstall via `pixi add/remove --manifest-path`.
  2. **GoScanner** (`scanner/go.go`): Scans `~/go/pkg/mod` module cache. Walks cache dirs with `@` in name to extract `modulePath@version`. Install via `go get`, uninstall via `rm -rf` module dir (Go has no per-module uninstall).
  3. **DockerScanner** (`scanner/docker.go`): Scans `docker images --format json`. Filters out `<none>` repos/tags. Install/uninstall via `docker pull/rmi`.
- **Dashboard updates** (`dashboard.go`):
  - Added `pixi`, `go`, `docker` to `tabSources`/`tabLabels`
  - Added to title bar count loop
  - Added to init pipeline scanner list
  - Added to `doUninstall` and `doInstall` source switches
- **Files**: `internal/scanner/pixi.go`, `internal/scanner/go.go`, `internal/scanner/docker.go`, `internal/tui/dashboard.go`

### Session: Semantic Search Stale Results Fix

#### Old search results overwritten second search
- **Root cause**: `startSearch()` runs in a background goroutine via `tea.Cmd`. If a user presses Enter twice quickly, two goroutines run concurrently. The first (old) search could finish after the second (new) search, sending a stale `semanticSearchResult` that overwrites the new results.
- **Fix**: Added `searchVersion int` to model. Incremented every time a search starts (Enter) or is cancelled (Esc / timeout). `semanticSearchResult` now carries a `version` field. The handler ignores any result whose version doesn't match the current `m.searchVersion`.
- **Also changed**: `search()` method now returns `semanticSearchResult` for errors (instead of `scanErrorMsg`) so errors are also versioned and discarded if stale.
- **Files**: `internal/tui/dashboard.go`

### Session: Splash Init Did Not Detect New Packages

#### Newly installed packages (e.g. `snap install glow`) never appeared on launch
- **Root cause**: `fullInitWithProgress()` (`dashboard.go`) gated Phase 1 (scan) behind `if existingCount == 0` — it only scanned on the very first run (empty DB). Every later launch loaded straight from the cached SQLite DB, so packages installed after the first scan never showed. The `r` "Rescan" key and the palette Rescan call the same function, so they were **silently no-ops** too (they reloaded the DB without rescanning).
- **Why the guard existed**: a full scan is slow. Per-manager timings on this system (warm cache): **pip 19s, conda 14s, snap 4s, npm 2.4s** dominate; apt 27ms and bin 63ms are negligible despite holding ~3700 of the ~5150 packages. The cost is subprocess/interpreter startup (pip per-venv, conda Python, `snap info` per package), not package count or DB writes.
- **Fix**:
  1. Removed the `existingCount == 0` guard so Phase 1 scans on every launch.
  2. Parallelised Phase 1: scanners run in goroutines (`sync.WaitGroup`); results collected then upserted sequentially (single-writer SQLite). Wall-clock ~28s vs ~42-53s sequential (pip+conda are partly CPU-bound so they contend, not a clean ~max).
  3. Background refresh: new `m.bgUpdating` flag. If the DB already has rows at `Init()`, show the dashboard immediately (cached) with a `⟳ updating…` indicator in the top-right of the title bar instead of the blocking full-screen splash. Splash now only shows on first run (empty DB). On scan completion the existing `scanCompleteMsg`/`isDone` → `loadData` path rebuilds the tree, so new packages appear automatically. `r`/palette Rescan also background now.
- **Store already supports this**: `store.Open` sets `journal_mode=WAL` + `_busy_timeout=5000` ("essential for background scan + UI queries on the same DB").
- **Splash gate is `if m.scanning && !m.bgUpdating`** in `View()` (two places: the splash block and the tree "Loading..." block).
- **Files**: `internal/tui/dashboard.go`

### Session: Layout Overflow — Rows Rendered ~2× Terminal Width

#### TUI wrapped/garbled: every line rendered at `2*width − 65` columns
- **Symptom**: at terminal width 120 every line was 175 cols wide (95 at width 80), overflowing the terminal so each line wrapped → garbled layout. `JoinVertical` pads all siblings to the widest, so one bad component widened the whole frame.
- **Root cause** (`tree.go` `calcColumnWidths`): when `contentWidth >= targetContent`, the leftover slack was distributed **twice** — once by `if total < contentWidth { c.loc += contentWidth - total }` and again by the `if contentWidth >= targetContent { c.name += …; c.loc += … }` block. Columns summed to `contentWidth + slack`. Fix: guard the first block with `&& contentWidth < targetContent` so it only handles below-target rounding remainder; the name/location split owns the at/above-target slack. This drives `renderTreeHeader` and `renderLeafNode` (both call `calcColumnWidths`), so expanded leaf rows had the same bug.
- **Two more bottom-panel bugs found while fixing**:
  1. `bottomPanelStyle.Width(m.width)` → box = `m.width + 2` (the RoundedBorder adds 2; lipgloss `.Width` here = content+padding). Fixed to `.Width(m.width - 2)`.
  2. `renderHelpPanel` two-column `renderPair` didn't truncate values → "Switch source" etc. overflowed tiny columns at narrow widths. Added `truncate(v, valW)` (values only — keys are ≤7 and never overflow the fixed 10-wide key column; truncating keys cut the `↑↓ / jk` arrows because `truncate` miscounts arrow glyph width).
  3. Title bar (package counts, ~93 cols fixed) didn't clamp → wrapped under ~93. Added `shellStyle.MaxWidth(sepWidth)`.
- **After fixes**: frame fits exactly at widths 60/80/100/120/200 (with and without the `⟳ updating…` indicator). 40-col still overflows by ~6 (unusable density anyway; left as-is).
- **Debugging method that worked**: a throwaway `_test.go` that builds the model, sets `m.width/height`, calls `m.Update(m.loadData())`, then measures `lipgloss.Width()` of `m.View()` lines and each sub-panel — pinpointed `renderTreeHeader=175` while everything else was correct. Strip ANSI with `re.sub(r'\x1b\[[0-9;]*m','')` to read frames.
- **Files**: `internal/tui/tree.go`, `internal/tui/dashboard.go`

### Session: Restore Semantic "Ask" on `?` (Enter) — Fast + Hang-Proof

#### `?` modal had been downgraded to a substring filter
- **What happened**: an uncommitted edit had retitled the `?` modal "Search packages" and rewired it so typing ran `liveSearch()` (SQL `name/description LIKE %q%` via `store.SearchText`) and **Enter just showed those substring results** — `startSearch()`/`search()` (BERT embeddings) were left orphaned, only reachable through the dead enrichment-at-query-time chain.
- **Why it had been done**: the old semantic path enriched + embedded *at query time* through Bubble Tea channels, which was the documented source of every UI freeze (hangs, double-`isDone` races, infinite loops).
- **Fix (restore + improve)**: Enter now runs `startSearch()` → `search()` again, but relies on init (`fullInitWithProgress`) having already pre-computed descriptions + embeddings — so search is **one query-encode + in-memory cosine/keyword scoring**, no enrichment/embedding in the hot path. It runs in a `tea.Cmd` goroutine (panic-recovered, `searchVersion`-guarded, 60s `searchTimeoutMsg` safety net), so it can't hang.
- **Graceful fallbacks**: `embedder == nil` → keep the live substring matches; `ListWithEmbeddings()` empty (fresh DB, init still running) → `search()` falls back to `SearchText`. So the modal never errors out or shows an empty void.
- **UX**: title back to "Ask whatsinstalled"; live substring matches shown as an instant *preview* labelled "Enter to search by meaning"; Enter commits the semantic search into the Results tab.
- **Kept the dead enrichment chain** (`enrich()`, `computeEmbeddings()`, `enrichmentProgressMsg/CompleteMsg`, `pollProgressCmd`) — unreachable at runtime but `search_enrichment_test.go` (~15 tests) still asserts on those types; removing them is a separate cleanup, not worth it here.
- **Verified on real data**: `~/.whatsinstalled.db` has 5182/5182 packages embedded; queries return in ~0.5–0.7s with strong results (e.g. "compress files" → gunzip/gzip/lzma/7za; "monitor system resources" → btop/sar/iostat/sysstat). Model cached at `~/.whatsinstalled/models/sentence-transformers` (177M). DB path is `~/.whatsinstalled.db` (NOT `~/.whatsinstalled/*.db`); no `sqlite3` CLI on this box — query via a throwaway Go program using the `store` package.
- **Files**: `internal/tui/dashboard.go`

### Session: Data-Driven Eval Harness for "Ask whatsinstalled" (IR metrics)

#### Goal: measure ranking quality so tuning isn't guesswork
- **Refactor**: extracted the ranking out of `(*model).search` into a pure, UI/DB/network-free `search.Rank(queryVec, query, pkgs, opts) []Result` (`internal/search/rank.go`); the TUI and the harness now score with identical code. `search.Options{KeywordWeight, Threshold, TopK}`; `DefaultOptions` = {1.0, 0.05, 50}. Query expansion (`nlp.ExpandQuery`) stays at the caller (it changes the embedded text). `(*model).search` rewired to call it; the no-embeddings `SearchText` fallback stayed. Removed now-unused `sort` import from `dashboard.go`.
- **Harness**: `whatsinstalled eval` subcommand (`internal/cmd/eval.go`) + model-free `internal/search/eval` package (metrics, synthetic gen, report/diff — all unit-tested without the model). Golden set = `internal/search/eval/queries.json` (user-supplied curated, currently 3 examples) + auto **synthetic known-item** queries (a package's description → that package). Metrics: **MRR + Hit@1/3/10**. Flags: `--synthetic N`, `--variant default|no-expand|semantic-only|keyword-2x|thr-0|all`, `--out`, `--baseline <report.json>` (per-query regression diff). Writes `eval-report.json` (gitignored).
- **Phase 2 (LLM-judge) deferred, designed-in**: judge cost scales with top-K shown, NOT corpus size → ~$0.12/50-query run on Haiku, cached `(query,pkg)` re-runs ≈ pennies. `Result` keeps per-component scores so nDCG drops in later. Not built; no anthropic-sdk-go dependency yet.
- **KEY FINDING (data-driven, ran on real DB, 5321 embedded pkgs)**: the keyword boost **hurts** ranking. `semantic-only` (KeywordWeight=0) MRR **0.640** vs `default` (KeywordWeight=1) **0.527**; `keyword-2x` collapses to 0.274. Also `no-expand` == `default` — `ExpandQuery` never fires unless the query literally contains a domain keyword (network/python/web/…), which real queries rarely do. `thr-0` == `default` (threshold only filters the tail). **Applied: set `DefaultOptions.KeywordWeight = 0` (2026-06 thermonuclear review).** "tools for editing video" is the worst curated query (RR 0.200, ffmpeg at rank 5 — text editors crowd it out).
- **Files**: `internal/search/rank.go`, `internal/search/eval/{eval.go,queries.json}`, `internal/cmd/eval.go`, `internal/tui/dashboard.go`

### Session: Enrichment for all sources + alphabetical tabs (2026-06-15)

#### Tab ordering
- `buildTabs` now sorts source tabs alphabetically (after the "All" tab) instead of using `scanner.AllScanners` registry order. Tests updated in `internal/tui/tabs_test.go`.

#### Description enrichment gap (the big one)
- **Root cause of "many packages have no descriptions"**: `enrichSource` only handled 6 of 22 sources (bin, pip, npm, apt, snap, conda). The other 16 had NO enrichment path → guaranteed blank.
- **Fix**: replaced the switch with data-driven `Enricher.descMapForSource` routing every source. New registries: crates.io (cargo), rubygems.org (gem); new local commands: `brew info --json=v2`, `pacman -Qi`. pipx/uv reuse the PyPI path, pnpm/yarn reuse npm.
- Also fixed a latent double-count in progress reporting (npm/snap/conda fired the callback twice per package).
- **crates.io 403s without a `User-Agent`** — `fetchJSON` always sets one.

#### pixi finding
- The system has the pixi binary but ZERO environments: `~/.pixi/envs` empty, `~/.pixi/manifests/pixi-global.toml` is 0 bytes, `pixi global list` → none, no `pixi.toml` anywhere under `~`. The PixiScanner is correct; it returns nothing because there is nothing installed. Not a bug.

#### Embedding staleness gap (known, not yet fixed)
- Embeddings are computed once when `embedding IS NULL` (`store.go` / `dashboard.go` Phase 3) and NEVER recomputed. If a description/metadata changes after embedding, the vector goes stale. Must null+recompute on change. Captured in `plans/metadata-enrichment-pipeline.md`.

#### Plans for the metadata model
- Two plan docs added under `plans/`: `specialized-metadata-model.md` (tiny distilled generative model, teacher→student) and `metadata-enrichment-pipeline.md` (Go integration via a `MetadataGenerator` interface, init-time generation, embed-feed). Ask query path stays pure embedding retrieval; the generative model runs only at init.

### Session: Size + Added columns, reliable per-source (2026-06-15)

- Added a `Used` sibling column **`Added`** (store `Package.AddedAt`, col `added_at INTEGER`) = mtime of the package's own files. mtime is reliable (not suppressed by noatime/relatime, unlike the atime-based `Used`).
- **Size/Added are only populated where the path resolves to the package's OWN files** — otherwise the number would be wrong/misleading, so we leave "-":
  - file stat: bin, cargo, appimage (size = file size, added = mtime).
  - dedicated per-package dir (du via `pkg.PathSize`): pipx, uv venvs.
  - native: apt (dpkg Installed-Size, pre-existing), docker/podman (`parseDockerSize`/`parseDockerCreated` on the `{{json .}}` Size/CreatedAt).
  - NOT populated (Location is a shared container or placeholder like "system"/env name → per-package size not reliable): pip, conda, npm, pnpm, yarn, gem, pixi, snap, nix, pacman, yay, brew, go, flatpak.
- Helpers: `pkg.PathSize(path)` (file size or recursive du), `pkg.GetModTime(path)`. TUI: `formatRelative` (renamed from `formatLastUsed`) shared by Added+Used; tree now has 8 columns (`spaces=7`).
- Done at scan time (cheap: file stat + small venv du), not lazily — broad du of shared dirs was rejected as unreliable per-package.

### Session: Thermonuclear Code Quality Review (2026-06-16)

- **KeywordWeight → 0**: `DefaultOptions.KeywordWeight` changed from 1.0 to 0.0 after eval data confirmed it hurts MRR (0.527 → 0.640). The mechanism stays wired for the eval harness.
- **Dead code removed**: `Enricher.EnrichBin/Pip/Npm` convenience methods (duplicated `descMapForSource` logic, no callers); `LocalEnricher.enrichSingleSnap/Npm/Conda` (old per-package enrichment path, no callers).
- **Duplicate row scanning unified**: `scanRows` now takes `withEmbedding bool`; `ListWithEmbeddings` no longer duplicates the null-handling block.
- **Mode-entry helpers**: `enterSearchMode`, `enterFilterMode`, `enterDetailMode`, `enterThemePicker`, `enterAbout`, `enterCommandPalette`, `triggerRescan` on `*model` — both `update.go` keyhandler and `palette.go` call them. No more split-brain.
- **PackageText contexts**: all 22 sources now get descriptive context (was 6 of 22).
- **Files**: `internal/search/rank.go`, `internal/enrich/enrich.go`, `internal/enrich/local.go`, `internal/store/store.go`, `internal/tui/model.go`, `internal/tui/update.go`, `internal/tui/palette.go`, `internal/nlp/embedder.go`, `ARCHITECTURE.md`, `MEMORY.md`
