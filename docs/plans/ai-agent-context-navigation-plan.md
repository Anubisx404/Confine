# Confine — AI Agent Context Navigation Plan

## 1. Objective

Turn Confine's existing indexing engine into a **context service for AI coding agents** (Claude Code, Gemini CLI, Codex, Cursor, Copilot, Antigravity, custom harnesses). Agents should find the right files and folders in one or two calls instead of dozens of `ls` / `grep` / `read_file` round-trips, and should receive context that is **ranked, budgeted, and explainable**.

The same data that drives the treemap (file tree, SLOC, symbols, imports, calls, Markdown catalog, AI-file catalog, search index) is exposed through a machine-first interface. The human UI gains an **Agent Lens** so a developer can see exactly what an agent looked at.

### Why Confine Is a Good Fit
| Existing capability | Location | Agent value |
| :--- | :--- | :--- |
| Parallel, `.gitignore`-aware indexer with hard skips (`node_modules`, `.git`, `dist`, ...) | `internal/indexer` | Agents never waste tokens on vendored or generated files |
| Per-file language, SLOC, byte size, symbol and import counts | `internal/indexer`, `internal/analyzer` | Cheap size/complexity signals for ranking and budgeting |
| Extracted symbols, imports, and top calls per file | `internal/analyzer` | Symbol lookup and dependency graph without an LSP |
| Sub-10ms trigram + bloom line search with snippets | `internal/search` | Fast content lookup returning exact `path:line` hits |
| README / plan / AI-file catalog with titles and categories | `internal/indexer/docs_scan.go`, `internal/search` | Agents load project intent (README, PLAN, AGENTS.md, skills) first |
| Runtime excludes and atomic snapshot swaps | `internal/server` | Scope narrowing per task without re-reading disk on every call |

---

## 2. Agent-Facing Surfaces

All three surfaces share one in-process `Snapshot` and one query layer (`internal/agentctx`).

### 2.1 MCP Server (`confine mcp`)
A stdio Model Context Protocol server, implemented with the Go standard library (`encoding/json` over stdin/stdout, JSON-RPC 2.0). Agents register it once:

```json
{ "mcpServers": { "confine": { "command": "confine", "args": ["mcp", "-path", "."] } } }
```

### 2.2 CLI Subcommands (`confine find|outline|context|...`)
Same tools as MCP, printing compact text (default) or JSON (`-json`). Works in any harness that can run a shell command, with no MCP support required.

### 2.3 HTTP Endpoints (`/api/agent/*`)
Same tools over the existing local server, for browser tooling and for agents that already talk HTTP.

---

## 3. Tool Catalog

Every tool response includes `elapsedMs`, `snapshotId` (so agents can detect stale results after edits), and `truncated` when a budget clipped the output.

| Tool | Input | Output | Built on |
| :--- | :--- | :--- | :--- |
| `project_overview` | `maxTokens` | Project name, languages by SLOC, top-level folders with file counts and purpose hints, README/PLAN titles, AI instruction files present | `TreeResult`, Markdown catalog |
| `find_files` | `query`, `kind?` (file/dir), `lang?`, `under?`, `limit` | Ranked paths using fuzzy path match, filename tokens, symbol names, and doc titles | Tree + analyzer symbols |
| `find_symbol` | `name`, `kind?` (func/type/class/...), `exact?` | `path:line`, symbol kind, enclosing file SLOC | `analyzer.Symbol` index |
| `search_code` | `query`, `limit`, `under?` | `path:line:col` + snippet, grouped by file with hit counts | `search.Index.Search` |
| `outline` | `path` (file or dir), `depth` | Dir: child tree with SLOC/lang/symbol counts. File: symbols, imports, top calls | Tree + analyzer |
| `related_files` | `path`, `limit` | Files linked by imports (both directions), shared symbol references, same-folder siblings, co-located tests | Import/call graph (new) |
| `context_pack` | `task` (natural language), `maxTokens`, `focus?` paths | Ordered bundle: instruction files, relevant docs, ranked files with outlines, and selected full-file or snippet bodies, fitted to the budget | All of the above |
| `list_ai_files` | `kind?` (skill/rule/agent/hook/...) | AGENTS.md, CLAUDE.md, GEMINI.md, `.cursor/rules`, `.agents/skills/*/SKILL.md`, hooks, MCP configs with titles | AI catalog (shipped) |
| `list_docs` | `category?` (readme/plan/doc) | README and plan catalog with titles | Markdown catalog (shipped) |
| `read_ranges` | `[{path, startLine, endLine}]` | Exact file slices with line numbers, capped per request | Snapshot lines + `/api/file` safety checks |

### 3.1 Ranking Model for `find_files` and `context_pack`
A deterministic, explainable score per candidate file, with no embeddings and no network calls:

```
score = 4.0 * pathTokenMatch      (query tokens in path segments / filename)
      + 3.0 * symbolMatch         (query tokens equal or prefix of declared symbols)
      + 2.0 * contentHitDensity   (search hits / sqrt(SLOC), capped)
      + 1.5 * graphProximity      (1 / hops from any focus file in the import graph)
      + 1.0 * docMention          (path or symbol mentioned in README / PLAN / AGENTS.md)
      - 0.5 * sizePenalty         (log2(SLOC) above the 90th percentile)
      - 2.0 * testOrFixturePenalty (unless the task mentions tests)
```

Each result carries a `why` array (`["path:auth", "symbol:LoginHandler", "imported-by:server.go"]`) so the agent and the human can audit the choice.

### 3.2 Token Budgeting
- Token estimate: `ceil(bytes / 3.6)` for code and `ceil(bytes / 4.2)` for prose, calibrated in tests against a reference tokenizer sample. The estimate is reported as an estimate, never as an exact count.
- `context_pack` fill order: (1) AI instruction files relevant to the touched folders, (2) README / plan sections that mention the task terms, (3) outlines of the top N files, (4) full bodies of the top K files, (5) snippet windows (`±12` lines) around search hits for the remainder.
- Stops at `maxTokens`, then lists the omitted candidates with their scores so the agent can request them with `read_ranges`.

---

## 4. New Backend Work

### 4.1 `internal/agentctx` Package
- Pure functions over `*server.Snapshot` data (`TreeResult`, `search.Index`, analyzer results). No HTTP and no I/O beyond what the snapshot already holds.
- `SymbolIndex`: `map[lowerName][]SymbolRef` built once per snapshot from `node.Analysis.Symbols`.
- `ImportGraph`: resolves `analyzer.ImportRef` strings to workspace paths per language (Go module path prefix from `go.mod`; relative JS/TS specifiers with extension probing; Python dotted modules; Rust `mod`/`use crate::`; Dart `package:`/relative). Unresolved imports are kept as external package names.
- `Ranker` implementing §3.1, and `Packer` implementing §3.2.

### 4.2 Snapshot Freshness
- Optional `-watch` flag: poll directory mtimes (stdlib only, 1 s interval, debounced) and rebuild the snapshot when files change, bumping `snapshotId`.
- MCP `notifications/resources/updated` sent after a rebuild so agents can refresh cached answers.

### 4.3 Safety
- Read-only. No tool writes files, runs commands, or follows symlinks outside the root (reuses `resolveSafeFilePath`).
- Output caps: `read_ranges` max 400 lines per range, 2,000 lines per call; `context_pack` hard ceiling of 200k estimated tokens.
- Respects runtime excludes, so a developer can hide secrets folders (`.env`, `secrets/`) from every agent surface at once.

---

## 5. Human-Facing UI: Agent Lens

- **Agent Activity tab:** a live log of agent tool calls (tool, arguments, result count, elapsed time) streamed over Server-Sent Events from the MCP/CLI process when it shares the server.
- **Treemap highlight:** files returned by the latest agent call glow with the accent outline; files actually read with `read_ranges` get a filled marker. This shows at a glance whether the agent stayed in the right part of the codebase.
- **Context Pack preview:** the developer can type a task, run `context_pack` from the UI, see the ranked list with `why` reasons and token estimates, toggle files in or out, and copy the bundle as Markdown for any chat-based model.
- **AI Files subtab (shipped):** `Markdowns → AI` lists AGENTS.md, CLAUDE.md, GEMINI.md, skills, rules, agents, workflows, commands, prompts, hooks, MCP and tool config, grouped by kind.

---

## 6. Phased Delivery & Verification

| Phase | Scope | Verification gate |
| :--- | :--- | :--- |
| **A** | `internal/agentctx`: `SymbolIndex`, `find_files`, `find_symbol`, `outline`, `project_overview`; CLI subcommands with text and `-json` output | Table-driven unit tests on fixture repos (Go, TS, Python); golden-file output tests; `go vet`, zero-comment scan |
| **B** | `ImportGraph` + `related_files`; `Ranker` with `why` reasons | Fixture repos with known import edges; precision@5 on a hand-labelled set of 30 queries recorded as a baseline |
| **C** | `context_pack` + `read_ranges` with token budgeting | Budget never exceeded across 500 randomized budgets; estimate error measured against a reference tokenizer sample and reported |
| **D** | `confine mcp` stdio server (JSON-RPC 2.0, `initialize`, `tools/list`, `tools/call`) | Protocol conformance tests with a scripted client; manual registration test in at least one real MCP host |
| **E** | `/api/agent/*` endpoints, `-watch` freshness, Agent Lens UI | `httptest` coverage; live browser check of highlight and SSE log; rebuild latency measured on this repo and on a 5k-file fixture |

### Success Metrics (measured, not assumed)
- **Round-trips:** median agent tool calls to locate the files for a task, with and without Confine, over a fixed task set.
- **Precision@5** of `find_files` and `context_pack` on the labelled query set.
- **Latency:** p50/p95 per tool on this repo and on a 5k-file fixture, measured with Go benchmarks.
- **Token cost:** estimated tokens per completed task with and without `context_pack`.
