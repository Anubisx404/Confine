<p align="center">
  <img src="docs/main_project_icon.png" alt="Confine logo" width="180" />
</p>

# Confine

> **Fast, dense, unimpressed spatial codebase explorer and static analyzer.**  
> Inspect any local repository as a GPU-accelerated 2D/3D treemap down to syntax-highlighted source lines inside a single zero-dependency Go binary.

![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white)
![WebGL2 + Canvas 2D](https://img.shields.io/badge/Renderer-WebGL2%20%2B%20Canvas%202D-d4964e?style=flat-square)
![Dependencies](https://img.shields.io/badge/Runtime%20Deps-Zero-383e47?style=flat-square)
![Binary](https://img.shields.io/badge/Build-Single%20Binary%20(go%3Aembed)-2b3036?style=flat-square)

---

## Overview

**Confine** indexes a target directory in parallel, extracts static analysis telemetry (SLOC, bytes, symbol declarations, imports, function calls), computes nested squarified 2D/3D treemaps, and serves a single-binary workstation UI driven by **WebGL2** instanced geometry and a **HiDPI Canvas 2D** semantic-zoom overlay.

Clicking any file enters **Focus Mode**—smoothly framing a sharp source card while softly blurring surrounding treemap blocks via a cached downsampled canvas pyramid so long files remain fluid to scroll and inspect at 60+ FPS.

---

## Key Features

- **Dual 2D / 3D Spatial Treemap:** Instanced WebGL2 rendering with continuous zoom across three levels of detail—from top-level modules (`LOD 0`), to file headers and dense minimap line strips (`LOD 1`), to clickable syntax-highlighted source (`LOD 2`).
- **Focused File Inspection (Focus Mode):** Click any file tile to isolate its source card over a cached multi-pass downsampled background, scroll directly inside the card, select individual lines, and press `Esc` to return.
- **Sub-10ms Code & Path Search:** In-memory line/path search (`/`, `Ctrl+K`) with instant snippet results, spatial match counts, live canvas highlights, and camera fly-to.
- **Static Analysis & Inspector:** View `SLOC`, `Bytes`, or `Symbols` layout presets alongside extracted functions, types, imports, calls, Markdown document previews, and Alt+Left/Right navigation history.
- **Runtime Folder Picker & Live Exclusions:** Open workspaces via CLI flags, path bar, inline Disk Navigator (`Dirs`), or native OS folder dialog (`Browse…`); exclude files, directories, or glob patterns (`Excludes` drawer or `X` key) with atomic snapshot swaps.
- **Industrial Workstation Interface:** Built on OKLCH graphite (`oklch(0.155 0.005 245)`) and restrained warm ochre (`oklch(0.76 0.14 72)`), `1px` structural hairlines, tabular numerals, keyboard-roving tabs, and `prefers-reduced-motion` support.

---

## Quick Start

### Prerequisites
- **Go 1.26+** (no Node.js, npm, or frontend bundler required; static web assets in `web/` are embedded into the executable via `//go:embed`).

### Build & Run

```powershell
# Clone repository
git clone https://github.com/Anubisx404/Confine.git
cd Confine

# Run tests & build single executable
go test ./...
go build -o confine.exe ./cmd/confine

# Launch pointed at any codebase (auto-opens default browser)
.\confine.exe -path "C:\path\to\your\project" -port 8080
```

On Linux / macOS:

```bash
go build -o confine ./cmd/confine
./confine -path /path/to/your/project -port 8080
```

---

## CLI Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-path` | `.` | Target codebase directory to index and serve |
| `-port` | `8080` | Local HTTP server port |
| `-open` | `true` | Automatically open the UI on startup |
| `-app` | `true` | Launch in a standalone desktop app window (`msedge`/`chrome --app`) when available; set `-app=false` for regular browser tab |
| `-exclude` | `""` | Comma-separated initial exclude patterns (paths, folders, or globs) |

---

## Keyboard & Pointer Controls

| Input | Action |
| :--- | :--- |
| `Click` file tile | Enter Focus Mode & inspect source |
| `Esc` | Unfocus file, close open drawer, or clear active input |
| `Wheel` | Zoom viewport / scroll inside focused code card |
| `Drag` | Pan map |
| `Right-Drag` / `Shift+Drag` | Orbit camera in `3D` mode |
| `/` or `Ctrl+K` | Jump to search input |
| `2` / `3` / `M` | Switch to `2D`, `3D`, or toggle view mode |
| `F` or `R` | Fit entire treemap in viewport |
| `X` | Exclude currently selected file or directory |
| `Alt+Left` / `Alt+Right` | Step backward / forward in selection history |
| `ArrowLeft` / `ArrowRight` | Cycle sidebar tabs (when focused on tab bar) or resize split pane (on handle) |

---

## HTTP API

| Endpoint | Method | Purpose |
| :--- | :--- | :--- |
| `/api/tree` | `GET` | Hierarchical squarified treemap + metrics (`?preset=sloc\|bytes\|symbols`) |
| `/api/file` | `GET` | File contents, line telemetry, symbols, imports, and calls (`?path=...`) |
| `/api/search` | `GET` | Fast substring/regex line & path search with bounding boxes (`?q=...&limit=...`) |
| `/api/markdowns` | `GET` | Discovered Markdown files with extracted titles and coordinates |
| `/api/open` | `POST` | Dynamically switch active workspace directory cleanly (`{"path": "..."}`) |
| `/api/excludes` | `GET` / `POST` | Inspect or mutate runtime exclude globs/paths and trigger snapshot rebuild |
| `/api/fs` | `GET` | Browse local drives and subdirectories for the inline `Dirs` drawer |
| `/api/pick-folder` | `POST` | Launch native OS directory selection dialog |

---

## Project Layout

```text
Confine/
├── cmd/confine/          # CLI entrypoint & HTTP server bootstrap
├── internal/
│   ├── analyzer/         # Static symbol, import, call, and markdown parser
│   ├── indexer/          # Parallel walker, scoped .gitignore, language & SLOC engine
│   ├── search/           # In-memory indexed query engine
│   ├── server/           # Embedded asset serving, API handlers, atomic Snapshot swap
│   └── treemap/          # Nested squarified layout & 3D extrusion calculator
├── web/                  # Embedded SPA (index.html, styles.css, app.js, renderer.js, tokenizer.js)
├── docs/plans/           # Future architecture & Git/GitHub spatial intelligence plans
├── PLAN.md               # Core architecture & design specification
└── go.mod
```

---

## Roadmap

- **[Git & GitHub Spatial Intelligence Plan](docs/plans/git-and-github-spatial-intelligence-plan.md)** — Working-tree status overlays, `Changed Only` spotlight filter, `churn` / `hotspot` / `freshness` / `ownership` presets, zero-checkout branch/tag snapshot switcher, `base..head` treemap diff explorer, Focus Card diff gutter + `[c`/`]c` hunk navigation + blame strip, co-change coupling inspector, and `.git/config` GitHub permalinks.

---

## Tags / Topics

`go` · `webgl2` · `canvas-2d` · `treemap` · `codebase-explorer` · `static-analysis` · `developer-tools` · `visualization` · `spatial-ui` · `single-binary`
