# Confine — Git & GitHub Spatial Intelligence Plan

## 1. Objective & Design Alignment

Extend **Confine** so any workspace backed by a `.git` directory or GitHub remote surfaces live version-control topology, change hotspots, zero-checkout branch inspection, diff gutters, co-change coupling, and direct GitHub permalinks—without mutating the working directory or adding heavy build dependencies.

All visual additions follow `.impeccable.md` (`fast, dense, unimpressed`): OKLCH graphite surfaces, warm ochre (`oklch(0.76 0.14 72)`) strictly for active selection/hunk focus, restrained status pigments, `1px` structural hairlines, and inline compact ledgers instead of modal popups or dashboard cards.

---

## 2. Backend Architecture (`internal/gitinfo`)

### 2.1 Package Boundaries & Safety Invariants
Create an acyclic package `internal/gitinfo` used alongside `internal/indexer`, `internal/treemap`, and `internal/server`:

- **Read-Only Plumbing Only:** Never run `git checkout`, `git switch`, `git stash`, or `git fetch` automatically unless the user explicitly invokes an explicit refresh action. All inspection operates via non-mutating local plumbing (`git rev-parse`, `git status --porcelain=v2 -z`, `git for-each-ref`, `git ls-tree`, `git show`, `git diff`, `git log`, `git blame --porcelain`) with explicit `-C <root>` and strict per-command timeouts.
- **Graceful Non-Git Fallback:** When `git rev-parse --is-inside-work-tree` returns false or `git` is absent on `PATH`, return `IsGitRepo: false` with empty maps so Confine behaves identically to plain filesystem mode.
- **Snapshot Integration:** Attach an immutable `GitSnapshot` pointer (or `nil`) inside `server.Snapshot` built asynchronously or during `loadSnapshot`, swapped atomically on `POST /api/open` and `POST /api/excludes`.
- **`.gitignore` Awareness & Ref Tree Indexing:**
  - In `WorkingTree` mode, Confine indexes on-disk files as today while annotating every `FileStats` / treemap node with Git status flags.
  - In **Zero-Checkout Ref Mode** (`?ref=<branch|tag|sha>`), `internal/indexer` exposes a helper `WalkGitRef(root, ref, opts)` streaming `git ls-tree -r -z` + `git cat-file --batch` in parallel to construct an in-memory `TreeResult` + `SearchIndex` snapshot for any commit/branch without touching checked-out files on disk.

### 2.2 Core Data Structures
```go
type RepoSummary struct {
    IsGitRepo    bool         `json:"isGitRepo"`
    RepoRoot     string       `json:"repoRoot"`
    HeadRef      string       `json:"headRef"`
    HeadSHA      string       `json:"headSha"`
    Detached     bool         `json:"detached"`
    DirtyCount   int          `json:"dirtyCount"`
    Remotes      []RemoteInfo `json:"remotes"`
    DefaultWeb   string       `json:"defaultWebUrl"`
}

type FileGitMeta struct {
    WorkStatus    string `json:"workStatus,omitempty"`
    IndexStatus   string `json:"indexStatus,omitempty"`
    Changed       bool   `json:"changed"`
    Untracked     bool   `json:"untracked"`
    Conflicted    bool   `json:"conflicted"`
    AddedLines    int    `json:"addedLines"`
    DeletedLines  int    `json:"deletedLines"`
    ChurnCommits  int    `json:"churnCommits"`
    LastCommitAt  int64  `json:"lastCommitAt"`
    TopAuthor     string `json:"topAuthor"`
    TopAuthorPct  int    `json:"topAuthorPct"`
}

type DiffHunk struct {
    OldStart int          `json:"oldStart"`
    OldCount int          `json:"oldCount"`
    NewStart int          `json:"newStart"`
    NewCount int          `json:"newCount"`
    Lines    []DiffLine   `json:"lines"`
}

type CouplingPair struct {
    Path       string  `json:"path"`
    CoCommits  int     `json:"coCommits"`
    Score      float64 `json:"score"`
}
```

### 2.3 `.git/config` Remote -> GitHub Permalink Resolver
Parse `git remote -v` (or `.git/config` directly) supporting:
- SSH SCP syntax: `git@github.com:Owner/Repo.git` -> `https://github.com/Owner/Repo`
- SSH URL: `ssh://git@github.com/Owner/Repo.git` -> `https://github.com/Owner/Repo`
- HTTPS URL: `https://github.com/Owner/Repo.git` -> `https://github.com/Owner/Repo`

Generate deterministic URLs with zero API calls or tokens:
- File permalink: `<web>/blob/<headSha>/<relPath>#L<line>`
- Commit URL: `<web>/commit/<sha>`
- Branch compare URL: `<web>/compare/<base>...<head>`

Optional authenticated GitHub metadata (`PR` status, open PR list, check runs) may be layered later via an opt-in adapter calling `gh api` when `gh auth status` succeeds, never required for core Git spatial workflows.

---

## 3. REST API Contract Additions (`internal/server`)

| Method & Path | Query / Body | Response | Purpose |
| :--- | :--- | :--- | :--- |
| `GET /api/git/summary` | — | `RepoSummary` + per-file status map | Toolbar branch chip, dirty count, remote base URL |
| `GET /api/git/refs` | — | Branches (local/remote), tags, recent 30 commits | Populate zero-checkout ref switcher & compare base selector |
| `GET /api/git/tree-metrics` | `?window=90d&base=<ref>&head=<ref>` | Per-path `FileGitMeta`, churn, freshness, ownership, diff deltas | Drive treemap presets (`churn`, `hotspot`, `freshness`, `ownership`, `diff`) |
| `GET /api/git/diff` | `?path=...&base=<ref>&head=<ref>` | Parsed `[]DiffHunk`, added/modified line bitmap | Focus Card diff gutter, inline diff mode, and `[c`/`]c` jump |
| `GET /api/git/blame` | `?path=...&ref=<ref>` | Compact per-line commit SHA, author, timestamp | Optional compact blame strip in Focus Card |
| `GET /api/git/coupling` | `?path=...&limit=12` | `[]CouplingPair` + recent file commits | Inspector co-change companions & commit timeline |
| `POST /api/git/ref-snapshot` | `{"ref":"origin/main"}` or `{"ref":"WORKTREE"}` | Updated workspace snapshot + treemap | Switch active in-memory snapshot to any ref without checkout |

Existing endpoints (`GET /api/tree`, `GET /api/file`, `GET /api/search`, `GET /api/markdowns`) remain unchanged when viewing `WORKTREE`, and naturally read from the selected ref snapshot once switched via `POST /api/git/ref-snapshot`.

---

## 4. Five Spatial & Focus Intelligence Features

### 4.1 Feature 1 — Working-Tree Status Badges & Changed-Only Spotlight
- **Treemap Status Markers:** Render crisp 1px status perimeter highlights and tiny monospaced status indicators (`M`, `A`, `D`, `U`, `!`) on modified, staged, untracked, and conflicted tiles in `renderer.js`.
- **Changed-Only Spotlight (`C` shortcut or toolbar pill):** Filter/dim unchanged files smoothly while elevating modified directories and files to full contrast so code reviews and pre-commit audits isolate the blast radius immediately across the city map.
- **Excludes Synergy:** Clicking a dirty file or folder still respects `X` exclusions, while git status respects scoped `.gitignore`.

### 4.2 Feature 2 — Churn, Hotspot (`SLOC × Churn`), Freshness & Ownership Presets
Extend `<select id="preset-select">` in `web/index.html` and sizing/extrusion normalization in `internal/treemap`:
- **`churn`:** Size and/or 3D building height scaled by commit touch count over configurable window (`30d` / `90d` / `all`).
- **`hotspot`:** Area = `SLOC`, Height (3D) & Luminance (2D) = `SLOC * log2(1 + churn)` — immediately reveals overgrown, high-churn maintenance magnets.
- **`freshness`:** Age-decay color ramp from bright ochre (touched today) to cool muted graphite (untouched > 1 year), ideal for spotting dead modules vs active subsystems.
- **`ownership`:** Deterministic hue encoding by dominant author + border opacity indicating bus-factor concentration (`topAuthorPct >= 80%`).

### 4.3 Feature 3 — Zero-Checkout Branch, Tag & Commit Switcher + Diff Explorer
- **Toolbar Ref Selector:** Compact inline dropdown next to the workspace path displaying current `HEAD` (`main · 7a3f1c2 · 4 dirty`) and letting the user pick any local branch, remote tracking branch, tag, or recent commit.
- **In-Memory Ref Switching:** Reads `git ls-tree` + `git cat-file --batch` directly into a Confine `Snapshot`, updating treemap, search, symbols, and Focus Mode cleanly with zero disk modifications and zero risk to uncommitted local changes.
- **Compare Mode (`Base .. Head`):** Select e.g. `main..feature/focus` to colorize the entire repository treemap as a structural diff: added files highlighted in emerald, deleted files rendered as ghost outlines, modified files scaled by `addedLines + deletedLines`.

### 4.4 Feature 4 — Focus Card Diff Gutter, Inline Diff Toggle, Hunk Jump & Blame
Inside Confine's Canvas 2D LOD-2 Focus Card (`web/renderer.js`):
- **Diff Gutter Marks:** Thin `2px` hairline channel inside the line-number column (`+` added lines, `~` modified lines, `-` deletion tick markers).
- **`[c` / `]c` Hunk Navigation:** Jump smoothly (`scrollOffsets` clamp + subtle line flash) to the previous/next changed hunk inside the focused file.
- **Inline Diff Toggle (`D` inside Focus Mode):** Toggle between normal syntax-highlighted file view and inline unified hunk display showing deleted baseline lines in context.
- **Optional Blame Strip (`B` inside Focus Mode):** Toggles a compact `hash · author · age` column on the left margin using tabular numerals (`font-variant-numeric: tabular-nums`) without widening Focus Card blur passes.

### 4.5 Feature 5 — Inspector Co-Change Coupling, Commit Timeline & GitHub Permalinks
Inside `#tab-inspector` (`web/index.html` & `web/app.js`):
- **Co-Change Partners:** Typographic ledger listing files that most frequently change in the same commit as the selected file (`internal/treemap/treemap.go — 14 co-commits`), clickable to fly directly to the coupled file.
- **File Commit Timeline:** Compact chronological list (`shortSha · age · author · subject`); clicking a commit opens diff preview for that commit's changes in the file.
- **Direct GitHub Actions:** One-click `Open on GitHub` and `Copy Permalink` links resolved from `.git/config` remote + active `HeadSHA` + selected `#L<line>`.

---

## 5. Performance & Caching Strategy

- **Bound Expensive Git Operations:** Compute `WorkingTree` status (`< 50ms` even on large repos) synchronously or on open; build 90-day `churn` / `coupling` index once per repo open in a background worker bounded by `--max-count=1000` (configurable), streaming results cleanly or returning cached partial results with an inline indicator.
- **No Render-Loop Blocking:** Pre-upload per-instance git state (`gitStatusFlags`, `diffIntensity`, `freshnessScalar`) into the existing WebGL2 instanced float buffers so `renderWebGL` stays at 60–120 FPS with zero per-frame string lookups.
- **Pyramid Blur Preservation:** Keep the cached downsampled surround canvas pipeline untouched — status highlights, diff gutters, and blame columns render cleanly inside `drawFocusedCard` / `overlayCtx` at full FPS.

---

## 6. Phased Delivery & Verification Plan

| Phase | Scope | Verification Gate |
| :--- | :--- | :--- |
| **Phase A** | `internal/gitinfo` (`RepoSummary`, `.git/config` parser, `status --porcelain=v2`, `/api/git/summary`, `/api/git/diff`) | Unit tests with temporary `git init` fixture repos (clean, dirty, untracked, conflicted, non-git dir, SSH/HTTPS remotes) |
| **Phase B** | Treemap `Changed Only` filter, working-tree badges, Focus Card diff gutter + `[c`/`]c` hunk jump + inline diff + blame | `go test -race ./...` + live browser verification on Confine repo with unstaged edits |
| **Phase C** | Background churn/coupling/freshness/ownership indexer + `hotspot`/`freshness`/`ownership` presets + Inspector co-change ledger | Benchmark churn indexing latency on 1,000-commit repo (`< 400ms`), verify `go vet ./...` & zero comments |
| **Phase D** | Zero-checkout `git ls-tree` + `git cat-file --batch` `RefSnapshot` loader + `Base..Head` structural diff explorer | Integration test switching branches without altering dirty working tree files + verifying snapshot atomic swap |
