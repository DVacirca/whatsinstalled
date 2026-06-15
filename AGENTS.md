# AGENTS.md — working in the whatsinstalled repo

Guidance for AI agents (and humans) working in this codebase. Keep it current.

## What this is
`whatsinstalled` is a Go CLI/TUI that inventories packages installed across many
package managers, enriches them with descriptions, and supports natural-language
("Ask whatsinstalled") semantic search. POC/MVP — favour the simplest thing that works
over enterprise patterns.

## Build / test / run
```bash
go build ./...                       # compile all packages
go build -o whatsinstalled ./cmd/whatsinstalled  # build the binary (ALWAYS rebuild after changes;
                                     # a stale ./whatsinstalled silently ignores new code)
go test ./...                        # full suite
go vet ./...                         # vet

./whatsinstalled                           # launch the TUI
./whatsinstalled scan                      # rescan + print per-source counts
./whatsinstalled eval --synthetic 30       # evaluate semantic-search ranking (MRR/Hit@k)
./whatsinstalled --version                 # print version
```
whatsinstalled is read-only: it inventories and searches packages — there is no
install/uninstall action (removed).
Note: the shell on this machine prints `command not found: _encode/_decode` noise
from the user's zsh profile — ignore it; it is not from our code. There is no
`sqlite3` CLI here — query the DB via a throwaway Go program using the `store`
package.

## Layout
```
cmd/whatsinstalled        entrypoint
cmd/enrich                one-off enrichment helper
internal/cmd              cobra commands (root, scan, eval)
internal/scanner    one file per package manager; AllScanners registry + DiscoverScanners
internal/store      SQLite (modernc.org/sqlite), WAL mode; Package model, embeddings, enrichment_cache
internal/enrich     per-source description enrichment: local tools then registries (PyPI, npm, crates.io, rubygems, brew, pacman); cached, timeouts
internal/nlp        embedder (all-MiniLM-L6-v2 via cybertron), cosine, query expansion, keyword score
internal/search     pure ranking: search.Rank — shared by the TUI and the eval harness
internal/search/eval IR metrics (MRR/Hit@k), synthetic queries, queries.json golden set, report/diff
internal/tui        Bubble Tea dashboard, tree view, styles, themes
```

## Runtime facts (this machine)
- DB: `~/.whatsinstalled.db` (NOT `~/.whatsinstalled/*.db`). Path comes from `store.DBPath()`.
- Embedding model cached at `~/.whatsinstalled/models/sentence-transformers` (~177MB,
  384-dim). First run downloads it; `nlp.LoadEmbedder()` returns an error if absent
  and search degrades to a substring fallback.
- Init pipeline (`fullInitWithProgress` in `internal/tui/commands.go`) scans →
  enriches → embeds, so search is just one query-encode + in-memory scoring (fast,
  cannot hang). Do NOT reintroduce enrichment/embedding into the search hot path.

## Semantic search + evaluation
- `?` opens "Ask whatsinstalled"; Enter runs `search.Rank` over `store.ListWithEmbeddings()`.
- Ranking is hybrid: cosine similarity + `nlp.KeywordScore` boost (`KeywordWeight`).
- To change ranking, edit `search.DefaultOptions()` / `search.Rank`, then measure with
  `whatsinstalled eval` (variants: default, no-expand, semantic-only, keyword-2x, thr-0).
  Save `--out base.json`, then `--baseline base.json` to catch regressions.
- Known finding: the keyword boost was shown to hurt relevance (semantic-only beats
  default on MRR). `DefaultOptions.KeywordWeight` is now 0.
- Curated golden queries live in `internal/search/eval/queries.json` — expand them.

## Conventions
- Git: imperative, lowercase subject < 72 chars, no conventional-commits prefix.
  Make one commit per logical change. Never push without being asked.
- Tests: deterministic, behaviour-focused; real objects over mocks. Metric/ranking
  logic is unit-tested without the model; model-dependent tests skip if uncached.
- Beware Go map iteration order — never derive UI order (e.g. tabs) directly from a
  map. `buildTabs` sorts source tabs alphabetically (after "All") for a stable
  order. (Deriving order from map iteration caused a real tab-reorder bug.)
- Log non-obvious discoveries/gotchas to MEMORY.md immediately, not at session end.

## Key docs
- `docs/IDEA.md` pitch · `docs/PLAN.md` design · `docs/REQUIREMENTS.md` spec · `docs/TECH.md` stack ·
  `docs/MEMORY.md` running log of decisions, gotchas, and findings (read this first).
