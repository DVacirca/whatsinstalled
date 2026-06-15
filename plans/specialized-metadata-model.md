# Plan — Specialized metadata model (Body of work 1)

## Context

installr's "Ask" feature is embedding retrieval (MiniLM via `nlpodyssey/cybertron`)
plus a hardcoded `domainSynonyms` map and a `KeywordScore` boost
(`internal/nlp/search.go`, `internal/search/rank.go`). Its quality ceiling is set
by how rich the per-package associations are — and today those associations live
in a static, hand-maintained map, while many packages carry no description at all.

The fix proposed here is a **separate, heavier generative model**, specialized on
Linux packages / libraries / environments, that produces **rich plain-language
metadata** per package. It is distilled from a large *teacher* LLM. It is **never
called at query time** — only at installr init to generate metadata, which is then
embedded (see the companion plan, `metadata-enrichment-pipeline.md`).

This document covers the model itself: dataset, training, packaging, eval. It is
an ML track (Python / Hugging Face), separate from the Go module. installr depends
on it only through a `MetadataGenerator` interface, so the two tracks proceed
independently.

## Goal

A tiny, fast, local generative model:

```
input:  { name, source, description }
output: structured metadata (JSON, schema below)
```

### Example (from the user)

> **Input:** "I'm tool from apt 'newtool', description 'a tool to help visualise
> spreadsheets in the terminal'. Enrich my metadata + categories for the ask
> feature."
>
> **Output:**
> - categories: `[cli, data, productivity]`
> - associations: `[spreadsheet, csv, tui, table, visualization, terminal]`
> - use_cases: "View and explore tabular/spreadsheet data directly in the terminal."
> - related_tools: `[visidata, sc-im, csvkit]`
> - aliases: `[newtool]`

## Output schema

```json
{
  "categories":    ["string"],   // coarse buckets: cli, web, database, security, …
  "associations":  ["string"],   // synonyms / related terms for recall
  "use_cases":     "string",     // 1–2 plain-language sentences — the PRIMARY embedded text
  "related_tools": ["string"],   // functionally similar / alternative tools
  "aliases":       ["string"]    // alternate names users search by (e.g. k8s)
}
```

`use_cases` is the field that most improves semantic recall once embedded;
`associations` / `related_tools` / `aliases` widen keyword and near-neighbour
matches.

## Work

1. **Teacher dataset generation**
   - Corpus: known packages across the sources installr scans — apt, pip/PyPI,
     npm, cargo/crates, gem, brew, conda, snap, etc. Seed from public package
     indexes and from real installr scans.
   - One fixed teacher prompt (large LLM, e.g. Claude) → the schema above.
   - Store as JSONL records `{ "input": {...}, "output": {...} }`.
   - Keep the teacher prompt versioned alongside the dataset.

2. **Distillation / training**
   - Small seq2seq or small instruct LM, fine-tuned on the JSONL.
   - Quantize for speed (the model runs over hundreds of packages at first init).
   - Lives in a `model/` subdir or a separate repo — **out of scope for the Go
     build**.

3. **Dataset growth over time**
   - Packages seen in the wild that the model handles weakly are queued.
   - Teacher re-labels them offline; model is retrained periodically.
   - Coverage improves **without any runtime external calls** — the runtime path
     stays local-only.

4. **Inference packaging for installr** (decision to lock down)
   - **Preferred:** export to ONNX / a format a Go runtime (`cybertron` or
     similar) can load in-process, mirroring how MiniLM already loads from
     `~/.installr/models`.
   - **Fallback:** ship a sidecar binary that installr shells out to, if the
     generator is too heavy to run in-process.

5. **Eval**
   - Held-out package set; metadata quality graded by human / teacher.
   - Downstream check: does richer metadata improve the existing Ask eval harness
     (`internal/search/eval`, `internal/cmd/eval.go`) on a labelled query set?

## Definition of done

- A reproducible teacher → JSONL → student pipeline exists.
- The distilled model produces valid-schema metadata for unseen packages.
- An installr-loadable artifact (ONNX/in-process or sidecar) is produced and its
  interface matches what `internal/metadata` (Body of work 2) expects.
- Eval shows the model's metadata measurably improves Ask ranking vs the current
  `domainSynonyms` baseline.
