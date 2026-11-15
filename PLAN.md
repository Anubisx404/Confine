# Confine (`confine`) — Implementation Plan

## 1. Overview & Goals

**Confine** is a high-frame-rate (60–120 FPS), Makepad Scope-inspired 2D/3D spatial codebase explorer and static analysis viewer built with **Go 1.26** and **WebGL2 + HiDPI Canvas 2D**. Pointed at any local repository or project directory, it indexes the filesystem in parallel, extracts lightweight static analysis metrics, computes a nested squarified treemap with 3D extrusion heights, and serves an interactive GPU-accelerated spatial map with continuous 3-tier semantic zoom (from city-scale modules down to clickable syntax-highlighted source code).

### Core Engineering Goals
- **Fluid 60–120 FPS Rendering:** Single/dual-pass instanced WebGL2 rendering for thousands of nested directory pedestals and file blocks, paired with a synchronized `devicePixelRatio`-scaled 2D overlay canvas for crisp typography and syntax-highlighted code.
- **Sub-10ms Interactive Search:** In-memory line/token index delivering instant case-insensitive and substring queries with bounding-box coordinates for live heat-map highlighting and camera fly-to.
- **Zero-Build-Step Local Binary:** Single Go binary embedding `web/` assets via `go:embed`, relying on the Go standard library (zero external runtime or npm bundler dependencies).
- **Dynamic Project Switching:** Point at an initial directory via CLI (`-path`) or switch active workspaces on the fly via `POST /api/open` without restarting the server.
- **Zero-Comment Code Style & Clean Architecture:** All `.go`, `.html`, `.js`, and `.css` source files strictly enforce **zero comments** (`//`, `/* */`, `<!-- -->` prohibited). Clarity is achieved through descriptive symbol names, small single-purpose functions, shallow nesting, and explicit package boundaries.

---

## 2. Repository Layout & Package Responsibilities

```text
Confine/
├── go.mod                          # module confine, go 1.26
├── PLAN.md                         # Master architecture & implementation plan
├── cmd/
│   └── confine/
│       └── main.go                 # CLI flags (-path, -port), snapshot bootstrap, graceful HTTP shutdown
├── internal/
│   ├── indexer/
│   │   ├── indexer.go              # Node, FileStats, TreeResult, concurrent directory walker
│   │   ├── gitignore.go            # Scoped .gitignore parser & glob/negation matcher
│   │   └── language.go             # Language detection, binary sniffing, SLOC counter
│   ├── analyzer/
│   │   └── analyzer.go             # Lightweight symbol/function extraction & import/call reference mapping
│   ├── treemap/
│   │   └── treemap.go              # Rect, LayoutNode, squarified algorithm, depth z, color palette
│   ├── search/
│   │   └── search.go               # Match, QueryResult, sub-10ms in-memory index, Markdown catalog
│   └── server/
│       └── server.go               # Server, atomic snapshot state, REST API handlers, go:embed static serving
└── web/
    ├── index.html                  # Makepad Scope dark shell layout (top bar, viewport, right sidebar)
    ├── styles.css                  # Technical workstation theme, split panes, tab & inspector styling
    ├── app.js                      # Application state, API client, History stack, Search & Markdowns UI
    ├── renderer.js                 # WebGL2 instanced block renderer, 2D<->3D camera, LOD 0/1/2 overlay
    └── tokenizer.js                # Lightweight multi-language syntax highlighter for LOD 2 source view
```

### Package Dependency Graph (Acyclic)
- `cmd/confine` -> `internal/indexer`, `internal/analyzer`, `internal/treemap`, `internal/search`, `internal/server`
- `internal/server` -> `internal/indexer`, `internal/analyzer`, `internal/treemap`, `internal/search`
- `internal/treemap` -> `internal/indexer`
- `internal/search` -> `internal/indexer`, `internal/treemap`
- `internal/analyzer` -> `internal/indexer` (or pure file-content input decoupled from filesystem walking)

---

## 3. Phase 1 — Parallel File System Indexer & Static Analyzer (`internal/indexer`, `internal/analyzer`)

### 3.1 Concurrent Directory Walker (`internal/indexer`)
1. **Bounded Worker Pool:**
   - Spawn `runtime.NumCPU()` worker goroutines reading directory/file tasks from a work-stealing or buffered work channel.
   - Maintain a deterministic tree by collecting directory entries, sorting names lexicographically once per directory, and assembling parent-child links without lock contention on hot paths.
2. **`.gitignore` & Hard-Skip Filtering:**
   - Parse `.gitignore` files hierarchically as traversal descends; maintain an immutable slice/stack of rule scopes per directory depth so child `.gitignore` rules override or negate (`!pattern`) parent rules using relative path matching (`**`, `*`, `?`, rooted `/`, and trailing `/` directory-only rules).
   - **Hard-Skip Directories (always skipped):** `.git`, `node_modules`, `build`, `.next`, `dist`, `target`, `.gradle`, `.idea`, `__pycache__`, `.venv`, `vendor`, `out`.
   - **Hard-Skip Generated Lockfiles:** `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `Cargo.lock`, `go.sum`, `poetry.lock`, `Gemfile.lock`, `composer.lock`.
   - **Binary Detection:** Skip known binary extensions (`.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.ico`, `.pdf`, `.zip`, `.tar`, `.gz`, `.ipa`, `.apk`, `.exe`, `.dll`, `.so`, `.dylib`, `.wasm`, `.ttf`, `.otf`, `.woff`, `.woff2`, `.mp3`, `.mp4`) and perform a NUL-byte (`0x00`) heuristic check on the first 8,192 bytes of unknown files.
3. **Per-File Metrics (`FileStats`) & `Node` Model:**
   - `Node`: `ID` (stable uint32 index), `Name`, `RelPath` (forward-slash normalized), `IsDir`, `Depth`, `Stats FileStats`, `ParentID`, `Children []*Node`.
   - `FileStats`:
     - `ByteSize int64`
     - `LineCount int` (total newline-delimited lines)
     - `SourceLineCount int` (non-whitespace lines excluding pure comment lines for the detected language)
     - `Language string`
     - `ChildrenCount int` (immediate children for directories; `0` for files)
     - `DescendantCount int` (recursive count of all descendant files + directories; aggregated bottom-up after parallel file reads complete)
     - `IsMarkdown bool`
     - `SymbolCount int`
     - `ImportCount int`
4. **Language Detection Matrix:**
   - Map file extensions and exact filenames (`Dockerfile`, `Makefile`, etc.) to canonical language identifiers:
     - **Systems / Compiled:** `Go` (`.go`), `Rust` (`.rs`), `C` (`.c`, `.h`), `C++` (`.cpp`, `.cc`, `.cxx`, `.hpp`, `.hh`), `C#` (`.cs`), `Java` (`.java`), `Dart` (`.dart`)
     - **Scripting / Web:** `TypeScript` (`.ts`, `.tsx`, `.mts`), `JavaScript` (`.js`, `.jsx`, `.mjs`, `.cjs`), `Python` (`.py`, `.pyw`), `HTML` (`.html`, `.htm`), `CSS` (`.css`, `.scss`, `.sass`)
     - **Data / Docs / Query:** `JSON` (`.json`, `.arb`), `YAML` (`.yaml`, `.yml`), `Markdown` (`.md`, `.mdx`), `SQL` (`.sql`)
     - **Fallback:** `Text` for valid UTF-8/ASCII non-binary text files without a specialized grammar.

### 3.2 Lightweight Static Analyzer (`internal/analyzer`)
1. **Design Principle:** Fast, single-pass, dependency-free lexical line scanners tuned per language family (no external compiler invocations) running inside the indexer worker pool while file bytes are already in CPU cache.
2. **Extracted Artifacts (`AnalysisResult`):**
   - `Symbols []Symbol`: `{Name, Kind ("function"|"method"|"class"|"struct"|"interface"|"enum"|"type"), Line, EndLine}`
     - Go: `func`, `type ... struct`, `type ... interface`
     - Dart / Java / C# / TypeScript / JS: `class`, `enum`, `mixin`, `extension`, `interface`, `function`, arrow/method signatures
     - Rust: `fn`, `struct`, `enum`, `trait`, `impl`
     - Python: `def`, `async def`, `class`
     - SQL: `CREATE TABLE`, `CREATE FUNCTION`, `CREATE VIEW`, `CREATE POLICY`
   - `Imports []ImportRef`: `{Target, Line}` parsed from `import`, `export ... from`, `use`, `require(...)`, `#include`, `from ... import`.
   - `CallMetrics`: top referenced/called identifiers (`Name`, `Count`) computed by tokenizing identifier occurrences followed by `(` (excluding language keywords and declaration lines), surfaced in the Inspector panel.

---

## 4. Phase 2 — Hierarchical Squarified Treemap Engine (`internal/treemap`)

### 4.1 Core Geometry & Layout Data Structures
- `Rect`: `{X float64, Y float64, W float64, H float64}` in normalized world coordinates (e.g., root canvas `[0, 1000] x [0, 650]` or normalized `[0, 1] x [0, 1]` scaled cleanly by viewport aspect).
- `LayoutNode`:
  - Identity & Hierarchy: `ID`, `ParentID`, `Name`, `RelPath`, `IsDir`, `Depth int`
  - Geometry: `Bounds Rect`, `ContentRect Rect` (inner area after directory header padding), `Z float64` (base elevation), `Height float64` (3D extrusion height)
  - Visuals: `Color string` / `ColorRGBA [4]uint8`, `ModuleName string`
  - Metrics: `Weight float64`, `Stats indexer.FileStats`, `Symbols []analyzer.Symbol`, `ImportCount int`

### 4.2 Nested Squarified Treemap Algorithm
1. **Weighting Presets:**
   - `sloc` (default): `max(SourceLineCount, 1)` so empty/tiny files still occupy a visible footprint.
   - `bytes`: `max(ByteSize, 64)`
   - `symbols`: `max(SymbolCount*10 + SourceLineCount, 1)`
   - Directory weight equals the exact sum of its children's weights; empty directories are pruned or assigned minimal epsilon weight if explicitly retained.
2. **Deterministic Sorting:**
   - Sort siblings in descending order of `Weight`.
   - Tie-breaker: directories before files, then ascending lexicographical `Name`, ensuring layout stability across reloads.
3. **Directory Header Padding & Inset:**
   - For every directory node with area above a minimum pixel-equivalent threshold, carve out a top header strip (`headerH = clamp(bounds.H * 0.08, minHeader, maxHeader)`) and uniform inner margin (`pad = clamp(min(bounds.W, bounds.H) * 0.02, minPad, maxPad)`).
   - Children are squarified strictly inside `ContentRect`, guaranteeing zero overlap with sibling headers or parent borders.
4. **Bruls–Huizing–van Wijk Squarify Step:**
   - Maintain current row slice along the shorter side of the remaining free rectangle (`shortSide = min(rem.W, rem.H)`).
   - Greedily append sorted items to the row while the worst aspect ratio `max(w/h, h/w)` improves (`worst(row + [next]) <= worst(row)`).
   - Once adding `next` degrades aspect ratio, commit the row along `shortSide`, advance `rem`, and repeat until all siblings are placed.
5. **3D Extrusion (`Z` and `Height`) & Hierarchical Palette:**
   - Base `Z` increases stepwise per directory depth (`baseZ = depth * layerStep`) so nested directories form stepped plateaus.
   - Leaf file extrusion `Height` scales sub-linearly with metric (`baseBlockH + k * log2(sloc + 1)`), creating an immediately legible code-city skyline where massive files stand out without dwarfing small files.
   - **Module Color Palette:** Assign each top-level directory (`Depth == 1`) a distinct perceptual hue (curated dark-mode-safe saturated palette avoiding muddiness), propagate that hue to all descendants with subtle lightness/chroma shifts by depth, and apply a crisp accent tint on file headers by language.

---

## 5. Phase 3 — Sub-10ms In-Memory Search & Markdown Catalog (`internal/search`)

### 5.1 Sub-10ms In-Memory Search Engine
1. **Index Representation:**
   - Store all indexed non-binary file contents in an immutable `Index` built concurrently right after treemap generation.
   - Per-file entry holds: `FileID`, `RelPath`, `Bounds treemap.Rect`, `Z float64`, `ModuleColor string`, `RawLines []string`, `LowerLines []string` (pre-lowercased lines, or a single contiguous lowercase byte arena with line start offsets to maximize CPU cache locality and minimize GC pointers).
   - Optional fast-path prefilter: per-file 64-bit character/trigram bloom signature (`uint64`) computed over lowercased contents so queries of length >= 3 skip non-matching files in nanoseconds before scanning lines.
2. **Query Execution (`Search(query string, limit int) QueryResult`):**
   - Measure exact execution duration with `time.Now()` / `time.Since(start)`.
   - Normalize `q := strings.ToLower(strings.TrimSpace(query))`; support multi-token AND matching or exact substring matching (default: fast `strings.Contains` on `LowerLines` after bloom check).
   - Collect matches up to `limit` (default `200`, max `1000`) while tracking `TotalMatches` and per-file `HitCount` map (used to render heat-map intensity across the 2D/3D treemap).
   - Build concise `Snippet` (trimmed line around match column, capped at ~160 chars) and attach the file's treemap `Rect` (`X, Y, W, H, Z`) so clicking any hit flies the camera straight to the target tile and line.
3. **Latency Target:** `< 10 ms` on medium-to-large repositories (1,000–20,000+ source files) by avoiding allocations in the inner loop and running per-shard parallel scanning when file count exceeds a threshold (e.g., > 500 files).

### 5.2 Markdown Catalog
- Built during indexing by filtering `Node.Stats.IsMarkdown == true`.
- Each `MarkdownEntry` captures: `Name`, `RelPath`, `Title` (extracted from first `# ` heading, falling back to filename), `ByteSize`, `LineCount`, `Bounds treemap.Rect`.
- Sorted with root-level canonical docs prioritized (`README.md`, `PLAN.md`, `SPEC.md`, `ARCHITECTURE.md`, `CHANGELOG.md`, `CONTRIBUTING.md`), followed alphabetically by `RelPath`.

---

## 6. Phase 4 — Local HTTP API & Asset Server (`internal/server`)

### 6.1 Embedded Assets & Snapshot Concurrency
- Embed `web/` directly into `internal/server` (or root `embed.FS` passed into `server.New(...)`) via `//go:embed`? **Important Zero-Comment Rule Check:** Go's `//go:embed` directive is a compiler directive, not a human comment. If strict zero-comment linting checks `//` lines, place `embed.FS` wiring in a dedicated minimal file or whitelist `^//go:embed ` in verification rules while keeping zero human comments anywhere—verify exact convention with the verifier/user rule (`go:embed` is required by Go compiler syntax for embedded assets).
- Store active workspace state (`Tree`, `Layout`, `SearchIndex`, `Markdowns`, `RootPath`, `AnalysisMap`) behind `atomic.Pointer[Snapshot]` so reads (`GET /api/*`) are lock-free and `POST /api/open` swaps snapshots atomically after completing indexing in the background.

### 6.2 REST API Contract
| Method & Path | Query / Body | Response (`application/json`) | Notes |
|---|---|---|---|
| `GET /` | — | `text/html` (`web/index.html`) | Serves SPA shell + static `.js`/`.css` assets from `embed.FS` |
| `GET /api/tree` | `?preset=sloc\|bytes\|symbols` | `{ rootPath, projectName, preset, stats, nodes: []LayoutNode }` | Flat node array sorted pre-order with `parentId` to keep JSON compact and fast to upload into WebGL2 buffers |
| `GET /api/file` | `?path=<relPath>` | `{ path, language, byteSize, lineCount, sourceLineCount, lines: []string, symbols: []Symbol, imports: []ImportRef, topCalls: []CallMetric }` | Strictly validates `filepath.Clean` + relative containment (`!strings.HasPrefix(rel, "..")`) and resolves symlinks inside root |
| `GET /api/search` | `?q=<query>&limit=200` | `{ query, totalMatches, elapsedMs, fileHits: map[string]int, matches: []Match }` | Returns sub-10ms timing, hit snippets, and treemap bounding boxes |
| `GET /api/markdowns` | — | `{ count, items: []MarkdownEntry }` | Feeds the `Markdowns` sidebar browser |
| `POST /api/open` | `{"path": "<dir>"}` | `{ ok, rootPath, projectName, stats }` | Validates directory exists, re-indexes, recomputes layout & search index, swaps snapshot |

---

## 7. Phase 5 — WebGL2 2D <-> 3D Viewport & Continuous Semantic Zoom (`web/`)

### 7.1 Dual-Layer Instanced Rendering Pipeline
1. **Layer 1: WebGL2 Instanced Block Renderer (`renderer.js`):**
   - Upload a single unit-cube / unit-bevelled-box mesh (`36` vertices with face normals) and a static-per-tree **Instance VBO** updated only when `/api/tree` or search heat-map state changes:
     - Per-instance attributes: `aRect (x, y, w, h)`, `aElev (z, height)`, `aColor (r, g, b, a)`, `aFlags (isDir, isSelected, isHovered, searchHeat)`.
   - **Single Draw Call:** `gl.drawArraysInstanced(gl.TRIANGLES, 0, 36, nodeCount)` draws 5,000–50,000 nested directories and files in `< 1 ms` per frame.
   - Vertex shader computes world position from `aRect` + `aElev * uExtrudeMix`, where `uExtrudeMix` smoothly animates `[0.0 -> 1.0]` during 2D <-> 3D transitions.
   - Fragment shader applies subtle directional top-down + rim lighting in 3D, crisp 1px screen-space border darkening (`fwidth`), selection glow, and pulsing/warm amber-cyan heat highlights on files matching the active search query.
2. **Layer 2: Synchronized HiDPI 2D Overlay Canvas:**
   - Shares exact viewport dimensions and projects world `(x, y, z)` coordinates through the identical `viewProj` `mat4` on CPU for nodes passing frustum + minimum screen-pixel culling (`screenW >= 24px`).
   - Ensures sub-pixel crisp text at any DPI without blurry texture atlas scaling.

### 7.2 Camera & 2D <-> 3D Smooth Interpolation
- **Unified Camera State:**
  - `centerX, centerY`: world focus point on the treemap plane.
  - `zoom`: world-to-screen scale factor (smoothly damped).
  - `mode3D` (`0.0` in 2D top-down, `1.0` in 3D city view) with current animated `blend` interpolating projection matrix between orthographic-equivalent flat view (`pitch = 0`, `extrude = 0`) and tilted perspective (`pitch ~ 48deg`, `yaw ~ -28deg`, `extrude = 1`).
- **Interactions:**
  - **Cursor-Centered Zoom:** Wheel delta scales `zoom` while keeping the world point under `(mouseX, mouseY)` pinned under the cursor.
  - **Pan & Orbit:** Left-drag pans along the ground plane; Right-drag (or Shift+drag / Alt+drag) orbits `yaw` and `pitch` in 3D mode (or auto-engages 3D mode smoothly).
  - **Ray-AABB / Quadtree Picking:** CPU spatial index maps cursor `(mouseX, mouseY)` to the deepest visible `LayoutNode` (and exact line number when hovering a LOD 2 file tile) in `< 0.2 ms`.
  - **Fly-To Transitions:** Double-clicking a directory/file or clicking a Search/History/Markdown item triggers a smooth ease-out cubic camera tween framing the target `Rect` (and scrolling to target line in LOD 2).

### 7.3 3-Tier Continuous Semantic Level of Detail (LOD)
Projected screen size `(screenW, screenH)` of each visible node's top face drives seamless visual transitions:
1. **LOD 0 — Macro / City View (`screenW < 64 px`):**
   - Colored directory containers and extruded 3D file pillars grouped by top-level module color.
   - Top-level and mid-level directory header labels rendered cleanly with collision avoidance so labels never overlap into unreadable clutter.
   - Search-hit heat highlights glow brightly across matching blocks with compact hit-count badges on directories.
2. **LOD 1 — Meso / File Block View (`64 px <= screenW < 280 px` or `screenH < 160 px`):**
   - File header bar rendered at top of tile with language dot, truncated filename, and SLOC badge.
   - Procedural/canvas minimap line strips inside the tile body: deterministic horizontal bars representing line indentation and line length profile (fetched lazily or synthesized from line-length metadata, and highlighting matched search line positions as bright horizontal ticks).
3. **LOD 2 — Micro / Source Code View (`screenW >= 280 px` and `screenH >= 160 px`):**
   - Automatically fetches `GET /api/file?path=...` (backed by an in-memory client LRU cache so revisiting files is zero-latency).
   - Renders crisp syntax-highlighted source code directly inside the zoomed file tile (via `tokenizer.js`), complete with gutter line numbers, active search-match line highlights, and hover row highlight.
   - **Clickable Line Selection:** Clicking any source line inside the LOD 2 tile selects `(file, lineNumber)`, highlights that line on the tile, updates the `Inspector` tab with the exact selected line number and enclosing symbol, and pushes an entry to the `History` stack.

---

## 8. Phase 6 — Makepad Scope UI Chrome & Right Sidebar (`web/`)

### 8.1 Visual Design Language
- Dark Makepad Scope workstation aesthetic: deep charcoal canvas (`#0f1116`), elevated panel surfaces (`#161922`), crisp `1px` hairlines (`#252a36`), monospace metrics (`JetBrains Mono` / `Cascadia Code` / system monospace fallback), high-contrast primary text (`#e6edf3`), and semantic module hues.
- Resizable right sidebar (`~360px` default width) + full-bleed WebGL2/2D viewport occupying the remaining screen space.

### 8.2 Top Toolbar
- **App Title & Workspace Switcher:** `Confine` brand badge + active project directory pill/input (`Open...` modal or inline path bar calling `POST /api/open`).
- **2D / 3D Mode Toggle:** Segmented `[2D | 3D]` toggle (shortcut `3` / `2` or `M`) triggering smooth camera + extrusion interpolation.
- **Fit / Reset Camera:** `Fit` button (shortcut `F` / `R`) framing the entire root treemap cleanly inside the viewport.
- **Active Selection Breadcrumb:** Interactive path trail (`my-project / internal / server / server.go`); clicking any ancestor segment selects that directory and flies the camera to frame it.
- **Layout Preset Dropdown:** Switch between `Source Lines (SLOC)`, `Byte Size`, and `Symbol Count`, re-querying `/api/tree?preset=...` and morphing/updating node rectangles smoothly.

### 8.3 Right Sidebar (4 Tabs)
1. **`Inspector` Tab:**
   - Selected item header: Name, Kind badge (`File` / `Directory`), Language badge, Full relative path (with copy-path action).
   - Selection line indicator: Selected line number (`Line 142` when clicked in LOD 2 or jumped via Search; `—` for directory or whole-file selection).
   - Size & Structure metrics grid: `Source Lines`, `Total Lines`, `Byte Size` (formatted B/KB/MB), `Immediate Children`, `Total Descendants`.
   - Static Analysis section: `Symbols` count + clickable symbol list (`func`/`class`/`struct` with line numbers that fly/scroll directly to that line), `Imports` count, and top referenced/called identifiers.
2. **`Search` Tab:**
   - Live search input box (auto-focused via `/` or `Ctrl+K`).
   - Status bar displaying exact hit count and server latency: `Search: N rows in X.X ms` (e.g., `Search: 42 rows in 1.4 ms`).
   - Scrollable hit list grouped or ordered by file/line, showing `relPath:line` and highlighted matching snippet.
   - Clicking any hit:
     1. Highlights all matching files on the 2D/3D treemap.
     2. Selects the clicked file and line number.
     3. Smoothly flies the camera directly to that file block at LOD 2 centered on the target line.
     4. Records the navigation jump in `History`.
3. **`History` Tab:**
   - Navigation history stack (`Back` / `Forward` buttons + `Alt+Left` / `Alt+Right` shortcuts).
   - Stores `{ relPath, line, cameraState, timestamp, actionKind }` whenever a user selects a file/directory, clicks a Search hit, clicks a Breadcrumb, or opens a Markdown file.
   - Displays clickable chronological entries to revisit prior inspection targets instantaneously.
4. **`Markdowns` Tab:**
   - Project documentation browser listing all indexed `.md` files (`README.md`, `PLAN.md`, `SPEC.md`, `ARCHITECTURE.md`, `docs/...`), filterable by name.
   - Each item provides:
     - **Jump to Map** button: locates the `.md` file on the treemap and flies the camera to it.
     - **Preview Pane**: fetches `/api/file?path=...` and renders formatted headings, code blocks, lists, tables, and links directly inside the sidebar for rapid project reading alongside the spatial map.

---

## 9. Phase 7 — Empirical Verification & Test Plan

### 9.1 Automated Go Unit & Benchmark Tests (`go test -race ./...`)
1. **`internal/indexer` Tests:**
   - `.gitignore` test suite: wildcard `*`, recursive `**`, directory-only `build/`, rooted `/secret.txt`, and negation `!important.txt` across nested directories.
   - Hard-skip verification: confirm `.git`, `node_modules`, `build`, `dist`, `target`, binary files (NUL bytes), and lockfiles (`go.sum`, `package-lock.json`, `Cargo.lock`) are excluded.
   - Language detection & SLOC tests: verify accurate `Language`, `LineCount`, `SourceLineCount`, `ChildrenCount`, and `DescendantCount` across fixtures for all 16+ supported languages.
2. **`internal/analyzer` Tests:**
   - Verify symbol extraction (`func`, `class`, `struct`, `def`, `fn`) and import extraction across Go, Dart, TypeScript, and Python test fixtures.
3. **`internal/treemap` Invariant Tests:**
   - Containment invariant: every child `Bounds` fits strictly inside its parent `ContentRect` within floating-point epsilon (`1e-6`).
   - Non-overlap invariant: no two sibling `Bounds` rectangles overlap.
   - Positive geometry & conservation: `W > 0`, `H > 0`, deterministic output across repeated runs, and bounded worst-case aspect ratio under squarification.
4. **`internal/search` Tests & Latency Benchmarks:**
   - Substring and case-insensitive match accuracy, line/column correctness, and treemap `Bounds` attachment.
   - `BenchmarkSearch`: benchmark queries on a 2,000+ file synthetic corpus and verify median/p95 query latency stays `< 10 ms`.
5. **`internal/server` HTTP & Concurrency Tests:**
   - `httptest` coverage for `GET /`, `GET /api/tree`, `GET /api/file`, `GET /api/search`, `GET /api/markdowns`, and `POST /api/open`.
   - Security test: confirm `GET /api/file?path=../../etc/passwd` and symlink escapes return `400`/`403` and never leak files outside the project root.
   - Race detector (`go test -race ./...`) exercising concurrent reads during `POST /api/open` snapshot swaps.

### 9.2 Zero-Comment & Static Quality Gates
- Run `go vet ./...` and `go build ./cmd/confine` (expect exit code `0`).
- Run a verification scan confirming zero comments (`//`, `/*`, `<!--`) exist in `.go`, `.html`, `.js`, and `.css` files (except strictly required `//go:embed` compiler directives if used).

### 9.3 End-to-End Verification Against `Confine` & Multi-Language Fixture Repositories
1. **CLI Launch:**
   - Run `confine.exe -path "<path-to-target-repo>" -port 8080` (verifying both against `Confine` itself and a multi-language fixture repository containing Go, Rust, TypeScript, Python, Dart, SQL, and Markdown files).
2. **Backend API Verification:**
   - `GET /api/tree?preset=sloc`: verify ignored directories (`.git`, `node_modules`, `build`, `dist`, `target`) and lockfiles are absent; verify source modules (`cmd/`, `internal/`, `web/`, `src/`, `docs/`) are present with non-zero SLOC and valid bounding boxes.
   - `GET /api/markdowns`: verify root and nested `.md` files (e.g., `PLAN.md`, `README.md`, `docs/**/*.md`) are cataloged with extracted titles and bounding boxes.
   - `GET /api/search?q=LayoutNode&limit=200` (and representative symbols across fixture languages): verify `elapsedMs < 10.0`, accurate match count, snippets, and valid bounding boxes.
   - `POST /api/open`: dynamically switch between `Confine` and a multi-language fixture repository, confirming clean atomic snapshot replacement.
3. **Live Browser / Viewport Verification:**
   - Verify 60+ FPS WebGL2 rendering with zero console/shader errors.
   - Toggle `2D <-> 3D` and verify smooth pitch/extrusion interpolation.
   - Zoom continuously through **LOD 0** (module city) -> **LOD 1** (file headers + minimap line strips) -> **LOD 2** (syntax-highlighted source inside a file tile such as `internal/treemap/treemap.go` or a fixture `.ts`/`.rs`/`.py` file), click a line, and verify `Inspector` displays the exact file path, selected line number, SLOC, byte size, descendants, and extracted symbols.
   - Execute a search in the `Search` tab, verify `Search: N rows in X.X ms`, click a hit to verify smooth camera fly-to + line selection, test `History` back/forward navigation, and preview `PLAN.md` / `README.md` in the `Markdowns` tab.
