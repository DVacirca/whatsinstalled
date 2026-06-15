# Plan — Metadata enrichment pipeline in installr (Body of work 2)

## Context

This is the installr-side integration for the specialized metadata model described
in `specialized-metadata-model.md`. The model produces rich plain-language
metadata per package; this work feeds that metadata into installr's init flow and
into the embedding/search path so the "Ask" feature gets richer categories and
associations (e.g. "web tools" → Django, node, axios).

Key constraints from the design:

- **No generative call at query time.** "Ask" stays pure embedding retrieval
  (`internal/search/rank.go` — cosine + keyword). The generative model runs only
  at init.
- **Cost paid at init.** First install enriches *every* installed package
  (hundreds; slow, one-time). Subsequent launches enrich only **new** entries
  (typically 1–2).
- Plain-language metadata is **embedded** via the existing MiniLM pipeline.

This is pure Go work. It depends on Body of work 1 only through an interface, so it
can be built and tested now against a stub backend.

## Work

### 1. `MetadataGenerator` interface

New package `internal/metadata`:

```go
type Metadata struct {
    Categories   []string
    Associations []string
    UseCases     string
    RelatedTools []string
    Aliases      []string
}

type Generator interface {
    Generate(name, source, description string) (Metadata, error)
}
```

Backends (selected by installr; rest of the pipeline is backend-agnostic):
- `stub` — fixed/empty output, for early dev and tests.
- `teacher` — direct large-LLM call, for bootstrapping and data collection.
- `local` — the distilled model from Body of work 1 (the production backend).

### 2. Storage

Extend the `packages` table in `internal/store/store.go`:
- Add a `metadata TEXT` column (JSON of `Metadata`) using the existing
  `ALTER TABLE … ADD COLUMN` migration pattern (`store.go:81-83`).
- Add `ListWithoutMetadata()` mirroring `ListWithoutEmbeddings()` (`store.go:239`).
- Add `UpdateMetadata(id int64, metadataJSON string)` mirroring `UpdateEmbedding`
  (`store.go:233`).

### 3. Init pipeline

Add a metadata phase to `runScan` in `internal/tui/dashboard.go` (currently
Phase 1 scan → Phase 2 enrich descriptions → Phase 3 embed, around
`dashboard.go:1359-1428`). New order:

```
scan → enrich descriptions → generate metadata (NEW) → embed
```

Reuse the existing channel / `scanProgressMsg` progress infra so the slow
first-run pass reports progress like the enrich and embed phases already do.

### 4. Incremental updates

On every launch, `ListWithoutMetadata()` returns only packages without metadata;
generate for just those. This is the same pattern installr already uses for
missing descriptions (`ListWithoutDescriptions`) and embeddings
(`ListWithoutEmbeddings`).

### 5. Feed metadata into embeddings

Extend `nlp.PackageText` (`internal/nlp/embedder.go:91`) so the embedded text
includes the metadata's `UseCases` + `Associations`, not just
`name + sourceContext + description`. This is the mechanism that places
"Django/node/axios" near "web tools" in vector space.

### 6. Embedding staleness fix (required)

Embeddings are currently computed once when `embedding IS NULL` and never
refreshed (`store.go:240`, `dashboard.go:1417`). Since metadata now changes the
embedding **input**, the metadata phase must **null out / recompute** the
embedding for any package whose metadata it just (re)generated, so the embed phase
picks it up. Without this, metadata never reaches the vectors.

### 7. Search path

Unchanged at query time: `Rank` (`internal/search/rank.go`) stays cosine +
keyword. Over time the static `domainSynonyms` map (`internal/nlp/search.go`) can
be reduced/retired as per-package `Associations` carry that signal — but that's a
later cleanup, **not** part of the first cut.

## Verification

- `go build ./...` and `go test ./...` pass.
- New unit tests: `store` metadata round-trip (`UpdateMetadata` /
  `ListWithoutMetadata`); `nlp.PackageText` includes metadata fields when present.
- Run the app once: confirm the metadata phase populates `packages.metadata`, that
  re-generated packages get re-embedded, and that init progress is reported.
- Confirm a labelled-query improvement via `internal/cmd/eval.go` vs the current
  `domainSynonyms` baseline.

## Definition of done

- `internal/metadata` exists with the interface and a `stub` backend wired in.
- The init pipeline generates + stores metadata (full pass first run, incremental
  after) and re-embeds affected packages.
- `nlp.PackageText` incorporates metadata; eval shows ranking is no worse than
  baseline with the stub and improves once a real backend is plugged in.
