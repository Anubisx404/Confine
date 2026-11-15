(function () {
  "use strict";

  const FILE_CACHE_MAX = 120;
  const HISTORY_MAX = 150;
  const SEARCH_DEBOUNCE_MS = 140;
  const DOUBLE_SLASH = String.fromCharCode(47, 47);

  const QUICK_EXCLUDE_PRESETS = {
    tests: [
      "*_test.go",
      "*.test.ts",
      "*.test.tsx",
      "*.test.js",
      "*.spec.ts",
      "*.spec.js",
      "*_test.py",
      "test_*.py",
      "*_test.dart"
    ],
    markdowns: ["*.md", "*.mdx"],
    configs: ["*.json", "*.yaml", "*.yml", "*.toml", "*.ini", "*.lock"],
    assets: ["*.css", "*.scss", "*.sass", "*.svg", "*.html"]
  };

  const state = {
    tree: null,
    preset: "sloc",
    excludes: [],
    nodeByPath: new Map(),
    nodeById: new Map(),
    selectedPath: null,
    selectedLine: null,
    is3D: false,
    activeTab: "inspector",
    dirsOpen: false,
    excludesOpen: false,
    fsCurrentPath: "",
    fsParentPath: "",
    fsRoots: [],
    fsEntries: [],
    fileCache: new Map(),
    inflightFiles: new Map(),
    searchQuery: "",
    searchAbort: null,
    searchDebounce: null,
    markdowns: [],
    activeMarkdownPath: null,
    history: [],
    historyIndex: -1,
    renderer: null,
    toastTimer: null,
    fpsFrames: 0,
    fpsLastTime: performance.now()
  };

  const els = {
    workspaceForm: document.getElementById("workspace-form"),
    workspacePath: document.getElementById("workspace-path"),
    projectTag: document.getElementById("project-tag"),
    openWorkspaceBtn: document.getElementById("open-workspace-btn"),
    browseFolderBtn: document.getElementById("browse-folder-btn"),
    dirsToggleBtn: document.getElementById("dirs-toggle-btn"),
    excludesToggleBtn: document.getElementById("excludes-toggle-btn"),
    excludesCount: document.getElementById("excludes-count"),
    workspaceSpinner: document.getElementById("workspace-spinner"),
    dirsDrawer: document.getElementById("dirs-drawer"),
    fsRoots: document.getElementById("fs-roots"),
    fsUpBtn: document.getElementById("fs-up-btn"),
    fsOpenCurrentBtn: document.getElementById("fs-open-current-btn"),
    dirsCloseBtn: document.getElementById("dirs-close-btn"),
    fsCurrentPath: document.getElementById("fs-current-path"),
    fsEntryCount: document.getElementById("fs-entry-count"),
    fsEntries: document.getElementById("fs-entries"),
    excludesDrawer: document.getElementById("excludes-drawer"),
    excludeForm: document.getElementById("exclude-form"),
    excludePatternInput: document.getElementById("exclude-pattern-input"),
    excludeAddBtn: document.getElementById("exclude-add-btn"),
    quickExcludeBtns: Array.from(document.querySelectorAll(".quick-exclude-btn")),
    excludeClearBtn: document.getElementById("exclude-clear-btn"),
    excludesCloseBtn: document.getElementById("excludes-close-btn"),
    excludesList: document.getElementById("excludes-list"),
    mode2dBtn: document.getElementById("mode-2d-btn"),
    mode3dBtn: document.getElementById("mode-3d-btn"),
    fitBtn: document.getElementById("fit-btn"),
    presetSelect: document.getElementById("preset-select"),
    statFiles: document.getElementById("stat-files"),
    statSloc: document.getElementById("stat-sloc"),
    statFps: document.getElementById("stat-fps"),
    statHover: document.getElementById("stat-hover"),
    breadcrumbTrail: document.getElementById("breadcrumb-trail"),
    breadcrumbLine: document.getElementById("breadcrumb-line"),
    viewportPane: document.getElementById("viewport-pane"),
    glCanvas: document.getElementById("gl-canvas"),
    overlayCanvas: document.getElementById("overlay-canvas"),
    viewportEmpty: document.getElementById("viewport-empty"),
    splitHandle: document.getElementById("split-handle"),
    sidebar: document.getElementById("sidebar"),
    tabBtns: Array.from(document.querySelectorAll(".tab-btn")),
    tabPanels: Array.from(document.querySelectorAll(".tab-panel")),
    searchTabCount: document.getElementById("search-tab-count"),
    historyTabCount: document.getElementById("history-tab-count"),
    markdownsTabCount: document.getElementById("markdowns-tab-count"),
    inspectorEmpty: document.getElementById("inspector-empty"),
    inspectorContent: document.getElementById("inspector-content"),
    inspName: document.getElementById("insp-name"),
    inspKind: document.getElementById("insp-kind"),
    inspFlyBtn: document.getElementById("insp-fly-btn"),
    inspExcludeBtn: document.getElementById("insp-exclude-btn"),
    inspPath: document.getElementById("insp-path"),
    copyPathBtn: document.getElementById("copy-path-btn"),
    inspLine: document.getElementById("insp-line"),
    inspLang: document.getElementById("insp-lang"),
    inspMetricsLines: document.getElementById("insp-metrics-lines"),
    inspMetricsTree: document.getElementById("insp-metrics-tree"),
    inspSymbolsCount: document.getElementById("insp-symbols-count"),
    inspSymbolsList: document.getElementById("insp-symbols-list"),
    inspImportsCount: document.getElementById("insp-imports-count"),
    inspImportsList: document.getElementById("insp-imports-list"),
    inspCallsCount: document.getElementById("insp-calls-count"),
    inspCallsList: document.getElementById("insp-calls-list"),
    searchInput: document.getElementById("search-input"),
    searchClearBtn: document.getElementById("search-clear-btn"),
    searchStatus: document.getElementById("search-status"),
    searchResults: document.getElementById("search-results"),
    historyBackBtn: document.getElementById("history-back-btn"),
    historyForwardBtn: document.getElementById("history-forward-btn"),
    historyClearBtn: document.getElementById("history-clear-btn"),
    historyList: document.getElementById("history-list"),
    markdownFilter: document.getElementById("markdown-filter"),
    markdownCount: document.getElementById("markdown-count"),
    markdownList: document.getElementById("markdown-list"),
    markdownPreviewWrap: document.getElementById("markdown-preview-wrap"),
    markdownPreviewTitle: document.getElementById("markdown-preview-title"),
    markdownPreviewMapBtn: document.getElementById("markdown-preview-map-btn"),
    markdownPreviewCloseBtn: document.getElementById("markdown-preview-close-btn"),
    markdownPreviewBody: document.getElementById("markdown-preview-body"),
    toast: document.getElementById("toast")
  };

  function showToast(message, kind) {
    if (state.toastTimer) {
      clearTimeout(state.toastTimer);
      state.toastTimer = null;
    }
    els.toast.textContent = message;
    els.toast.className = "toast" + (kind ? " " + kind : "");
    state.toastTimer = setTimeout(function () {
      els.toast.classList.add("hidden");
    }, 3400);
  }

  function formatNumber(n) {
    const val = Number(n || 0);
    return val.toLocaleString("en-US");
  }

  function formatBytes(bytes) {
    const b = Number(bytes || 0);
    if (b < 1024) {
      return b + " B";
    }
    if (b < 1024 * 1024) {
      return (b / 1024).toFixed(1) + " KB";
    }
    return (b / (1024 * 1024)).toFixed(2) + " MB";
  }

  function formatTime(ts) {
    const d = new Date(ts);
    const hh = String(d.getHours()).padStart(2, "0");
    const mm = String(d.getMinutes()).padStart(2, "0");
    const ss = String(d.getSeconds()).padStart(2, "0");
    return hh + ":" + mm + ":" + ss;
  }

  function escapeHtml(text) {
    return String(text || "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  async function apiGetJson(url, signal) {
    const resp = await fetch(url, { signal: signal });
    const data = await resp.json().catch(function () {
      return {};
    });
    if (!resp.ok) {
      throw new Error(data.error || "Request failed (" + resp.status + ")");
    }
    return data;
  }

  async function apiPostJson(url, body) {
    const resp = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body || {})
    });
    const data = await resp.json().catch(function () {
      return {};
    });
    if (!resp.ok) {
      throw new Error(data.error || "Request failed (" + resp.status + ")");
    }
    return data;
  }

  function lruSet(map, key, val, limit) {
    if (map.has(key)) {
      map.delete(key);
    } else if (map.size >= limit) {
      const oldestKey = map.keys().next().value;
      map.delete(oldestKey);
    }
    map.set(key, val);
  }

  async function fetchFile(relPath) {
    if (!relPath) {
      return null;
    }
    if (state.fileCache.has(relPath)) {
      const cached = state.fileCache.get(relPath);
      state.fileCache.delete(relPath);
      state.fileCache.set(relPath, cached);
      return cached;
    }
    if (state.inflightFiles.has(relPath)) {
      return state.inflightFiles.get(relPath);
    }
    const promise = apiGetJson("/api/file?path=" + encodeURIComponent(relPath))
      .then(function (data) {
        lruSet(state.fileCache, relPath, data, FILE_CACHE_MAX);
        state.inflightFiles.delete(relPath);
        return data;
      })
      .catch(function (err) {
        state.inflightFiles.delete(relPath);
        throw err;
      });
    state.inflightFiles.set(relPath, promise);
    return promise;
  }

  function resizeViewport() {
    if (state.renderer) {
      state.renderer.resize();
    }
  }

  function setDirsOpen(open) {
    state.dirsOpen = Boolean(open);
    if (state.dirsOpen && state.excludesOpen) {
      setExcludesOpen(false);
    }
    els.dirsDrawer.classList.toggle("hidden", !state.dirsOpen);
    els.dirsToggleBtn.classList.toggle("active", state.dirsOpen);
    els.dirsToggleBtn.setAttribute("aria-expanded", state.dirsOpen ? "true" : "false");
    if (state.dirsOpen) {
      const startPath = state.fsCurrentPath || (state.tree ? state.tree.rootPath : "");
      loadFs(startPath);
    }
    resizeViewport();
  }

  function setExcludesOpen(open) {
    state.excludesOpen = Boolean(open);
    if (state.excludesOpen && state.dirsOpen) {
      setDirsOpen(false);
    }
    els.excludesDrawer.classList.toggle("hidden", !state.excludesOpen);
    els.excludesToggleBtn.classList.toggle("active", state.excludesOpen);
    els.excludesToggleBtn.setAttribute("aria-expanded", state.excludesOpen ? "true" : "false");
    if (state.excludesOpen) {
      renderExcludesDrawer();
    }
    resizeViewport();
  }

  function syncExcludesState(excludes) {
    state.excludes = Array.isArray(excludes) ? excludes.slice() : [];
    els.excludesCount.textContent = String(state.excludes.length);
    syncQuickExcludeButtons();
    renderExcludesDrawer();
  }

  function syncQuickExcludeButtons() {
    const activeSet = new Set(state.excludes);
    for (const btn of els.quickExcludeBtns) {
      const key = btn.dataset.preset || "";
      const patterns = QUICK_EXCLUDE_PRESETS[key] || [];
      const isOn = patterns.length > 0 && patterns.every(function (p) {
        return activeSet.has(p);
      });
      btn.classList.toggle("active", isOn);
      btn.setAttribute("aria-pressed", isOn ? "true" : "false");
    }
  }

  function renderExcludesDrawer() {
    els.excludesList.innerHTML = "";
    els.excludeClearBtn.disabled = state.excludes.length === 0;

    if (state.excludes.length === 0) {
      const empty = document.createElement("span");
      empty.className = "exclude-empty";
      empty.textContent = "No active exclusions. Add a file path, folder/, or glob pattern.";
      els.excludesList.appendChild(empty);
      return;
    }

    for (const pat of state.excludes) {
      const tag = document.createElement("span");
      tag.className = "exclude-tag";

      const label = document.createElement("span");
      label.textContent = pat;

      const removeBtn = document.createElement("button");
      removeBtn.type = "button";
      removeBtn.className = "exclude-remove-btn";
      removeBtn.textContent = "×";
      removeBtn.title = "Remove exclusion " + pat;
      removeBtn.setAttribute("aria-label", "Remove exclusion " + pat);
      removeBtn.addEventListener("click", function () {
        applyExcludeMutation({ remove: pat }, "Removed exclusion: " + pat);
      });

      tag.appendChild(label);
      tag.appendChild(removeBtn);
      els.excludesList.appendChild(tag);
    }
  }

  async function applyExcludeMutation(payload, successMsg) {
    els.workspaceSpinner.classList.remove("hidden");
    try {
      const res = await apiPostJson("/api/excludes", payload);
      syncExcludesState(res.excludes || []);
      state.fileCache.clear();
      state.inflightFiles.clear();

      await Promise.all([loadTree(state.preset), loadMarkdowns()]);

      if (state.activeMarkdownPath && !state.markdowns.some(function (m) {
        return m.relPath === state.activeMarkdownPath;
      })) {
        state.activeMarkdownPath = null;
        els.markdownPreviewWrap.classList.add("hidden");
        renderMarkdownList();
      }

      if (state.searchQuery.trim()) {
        runSearch(state.searchQuery);
      }

      if (state.dirsOpen) {
        loadFs(state.fsCurrentPath);
      }

      if (successMsg) {
        showToast(successMsg, "success");
      }
    } catch (err) {
      showToast("Exclusions update failed: " + err.message, "error");
    } finally {
      els.workspaceSpinner.classList.add("hidden");
    }
  }

  function findMatchedExcludeForRelPath(relPath) {
    if (!relPath) {
      return "";
    }
    const withSlash = relPath + "/";
    for (const pat of state.excludes) {
      if (pat === relPath || pat === withSlash) {
        return pat;
      }
    }
    return "";
  }

  function renderFsBrowser() {
    els.fsCurrentPath.textContent = state.fsCurrentPath || "-";
    els.fsCurrentPath.title = state.fsCurrentPath || "";
    els.fsUpBtn.disabled = !state.fsParentPath;
    els.fsOpenCurrentBtn.disabled = !state.fsCurrentPath;
    els.fsEntryCount.textContent = formatNumber(state.fsEntries.length) + " items";

    els.fsRoots.innerHTML = "";
    for (const rootPath of state.fsRoots) {
      const btn = document.createElement("button");
      btn.type = "button";
      const isCurrentRoot = state.fsCurrentPath && state.fsCurrentPath.toLowerCase().startsWith(rootPath.toLowerCase());
      btn.className = "fs-root-btn" + (isCurrentRoot ? " active" : "");
      btn.textContent = rootPath;
      btn.title = "Browse " + rootPath;
      btn.addEventListener("click", function () {
        loadFs(rootPath);
      });
      els.fsRoots.appendChild(btn);
    }

    els.fsEntries.innerHTML = "";
    if (state.fsEntries.length === 0) {
      const empty = document.createElement("div");
      empty.className = "exclude-empty";
      empty.textContent = "Empty directory.";
      els.fsEntries.appendChild(empty);
      return;
    }

    const frag = document.createDocumentFragment();
    for (const entry of state.fsEntries) {
      const row = document.createElement("div");
      row.className = "fs-entry";

      const mainBtn = document.createElement("button");
      mainBtn.type = "button";
      mainBtn.className = "fs-entry-main";
      mainBtn.disabled = !entry.isDir;
      mainBtn.title = entry.path;

      const tag = document.createElement("span");
      tag.className = "fs-entry-tag" + (entry.isDir ? " dir" : "");
      tag.textContent = entry.isDir ? "dir" : "file";

      const name = document.createElement("span");
      name.className = "fs-entry-name";
      name.textContent = entry.name;

      mainBtn.appendChild(tag);
      mainBtn.appendChild(name);
      if (entry.isDir) {
        mainBtn.addEventListener("click", function () {
          loadFs(entry.path);
        });
      }

      const actions = document.createElement("div");
      actions.className = "fs-entry-actions";

      if (entry.relPath) {
        const matched = findMatchedExcludeForRelPath(entry.relPath);
        const toggleBtn = document.createElement("button");
        toggleBtn.type = "button";
        toggleBtn.className = "btn btn-xs" + (matched ? " active" : "");
        toggleBtn.textContent = matched ? "Include" : "Exclude";
        toggleBtn.title = matched ? "Remove " + matched + " from exclusions" : "Exclude " + entry.relPath;
        toggleBtn.addEventListener("click", function () {
          if (matched) {
            applyExcludeMutation({ remove: matched }, "Included " + entry.relPath);
          } else {
            applyExcludeMutation({ add: entry.relPath }, "Excluded " + entry.relPath);
          }
        });
        actions.appendChild(toggleBtn);
      }

      if (entry.isDir) {
        const openBtn = document.createElement("button");
        openBtn.type = "button";
        openBtn.className = "btn btn-xs";
        openBtn.textContent = "Open";
        openBtn.title = "Open workspace " + entry.path;
        openBtn.addEventListener("click", function () {
          openWorkspace(entry.path);
        });
        actions.appendChild(openBtn);
      }

      row.appendChild(mainBtn);
      row.appendChild(actions);
      frag.appendChild(row);
    }
    els.fsEntries.appendChild(frag);
  }

  async function loadFs(dirPath) {
    try {
      const qs = dirPath ? "?path=" + encodeURIComponent(dirPath) : "";
      const res = await apiGetJson("/api/fs" + qs);
      state.fsCurrentPath = res.currentPath || "";
      state.fsParentPath = res.parentPath || "";
      state.fsRoots = Array.isArray(res.roots) ? res.roots : [];
      state.fsEntries = Array.isArray(res.entries) ? res.entries : [];
      renderFsBrowser();
    } catch (err) {
      showToast("Failed to list directory: " + err.message, "error");
    }
  }

  async function pickFolderNative() {
    els.browseFolderBtn.disabled = true;
    els.workspaceSpinner.classList.remove("hidden");
    try {
      const res = await apiPostJson("/api/pick-folder", {});
      if (!res.canceled && res.path) {
        await openWorkspace(res.path);
      }
    } catch (err) {
      showToast("Folder picker failed: " + err.message, "error");
    } finally {
      els.browseFolderBtn.disabled = false;
      els.workspaceSpinner.classList.add("hidden");
    }
  }

  function excludeSelectedNode() {
    if (!state.selectedPath) {
      showToast("Select a file or subfolder to exclude.", "error");
      return;
    }
    const target = state.selectedPath;
    applyExcludeMutation({ add: target }, "Excluded " + target);
  }

  function switchTab(tabName) {
    state.activeTab = tabName;
    for (const btn of els.tabBtns) {
      const active = btn.dataset.tab === tabName;
      btn.classList.toggle("active", active);
      btn.setAttribute("aria-selected", active ? "true" : "false");
      btn.setAttribute("tabindex", active ? "0" : "-1");
    }
    for (const panel of els.tabPanels) {
      panel.classList.toggle("active", panel.id === "tab-" + tabName);
    }
  }

  function syncModeButtons(is3D) {
    state.is3D = Boolean(is3D);
    els.mode2dBtn.classList.toggle("active", !state.is3D);
    els.mode3dBtn.classList.toggle("active", state.is3D);
  }

  function renderBreadcrumb() {
    els.breadcrumbTrail.innerHTML = "";
    if (!state.tree) {
      return;
    }
    const projectName = state.tree.projectName || "workspace";
    const segments = [];
    segments.push({ label: projectName, relPath: "" });

    if (state.selectedPath) {
      const parts = state.selectedPath.split("/").filter(Boolean);
      let acc = "";
      for (let i = 0; i < parts.length; i++) {
        acc = acc ? acc + "/" + parts[i] : parts[i];
        segments.push({ label: parts[i], relPath: acc });
      }
    }

    for (let i = 0; i < segments.length; i++) {
      if (i > 0) {
        const sep = document.createElement("span");
        sep.className = "crumb-sep";
        sep.textContent = "/";
        els.breadcrumbTrail.appendChild(sep);
      }
      const seg = segments[i];
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "crumb-btn" + (i === segments.length - 1 ? " current" : "");
      btn.textContent = seg.label;
      btn.title = seg.relPath || projectName;
      btn.addEventListener("click", function () {
        selectAndFly(seg.relPath, null, "breadcrumb");
      });
      els.breadcrumbTrail.appendChild(btn);
    }

    if (state.selectedLine && state.selectedLine > 0) {
      els.breadcrumbLine.textContent = "line " + state.selectedLine;
      els.breadcrumbLine.classList.remove("hidden");
    } else {
      els.breadcrumbLine.classList.add("hidden");
    }
  }

  function renderInspectorAnalysis(symbols, imports, topCalls) {
    const symList = Array.isArray(symbols) ? symbols : [];
    const impList = Array.isArray(imports) ? imports : [];
    const callList = Array.isArray(topCalls) ? topCalls : [];

    els.inspSymbolsCount.textContent = String(symList.length);
    els.inspSymbolsList.innerHTML = "";
    if (symList.length === 0) {
      const empty = document.createElement("div");
      empty.className = "import-row";
      empty.textContent = "No symbols extracted";
      els.inspSymbolsList.appendChild(empty);
    } else {
      for (const sym of symList) {
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = "symbol-row";

        const main = document.createElement("span");
        main.className = "symbol-main";
        const kind = document.createElement("span");
        kind.className = "symbol-kind";
        kind.textContent = sym.kind || "sym";
        const name = document.createElement("span");
        name.className = "symbol-name";
        name.textContent = sym.name || "";
        main.appendChild(kind);
        main.appendChild(name);

        const lineEl = document.createElement("span");
        lineEl.className = "symbol-line";
        lineEl.textContent = ":" + sym.line;

        btn.appendChild(main);
        btn.appendChild(lineEl);
        btn.addEventListener("click", function () {
          if (state.selectedPath !== null) {
            selectAndFly(state.selectedPath, sym.line, "symbol");
          }
        });
        els.inspSymbolsList.appendChild(btn);
      }
    }

    els.inspImportsCount.textContent = String(impList.length);
    els.inspImportsList.innerHTML = "";
    if (impList.length === 0) {
      const empty = document.createElement("div");
      empty.className = "import-row";
      empty.textContent = "No imports";
      els.inspImportsList.appendChild(empty);
    } else {
      for (const imp of impList) {
        const row = document.createElement("div");
        row.className = "import-row";
        const target = document.createElement("span");
        target.className = "import-target";
        target.textContent = imp.target || "";
        target.title = imp.target || "";
        const lineEl = document.createElement("span");
        lineEl.className = "symbol-line";
        lineEl.textContent = ":" + imp.line;
        row.appendChild(target);
        row.appendChild(lineEl);
        els.inspImportsList.appendChild(row);
      }
    }

    els.inspCallsCount.textContent = String(callList.length);
    els.inspCallsList.innerHTML = "";
    if (callList.length === 0) {
      const empty = document.createElement("div");
      empty.className = "import-row";
      empty.textContent = "No call references";
      els.inspCallsList.appendChild(empty);
    } else {
      for (const call of callList) {
        const item = document.createElement("span");
        item.className = "call-item";
        const label = document.createElement("span");
        label.textContent = call.name || "";
        const count = document.createElement("span");
        count.className = "call-count";
        count.textContent = "(" + call.count + ")";
        item.appendChild(label);
        item.appendChild(count);
        els.inspCallsList.appendChild(item);
      }
    }
  }

  function renderInspector(node, line) {
    if (!node) {
      els.inspectorEmpty.classList.remove("hidden");
      els.inspectorContent.classList.add("hidden");
      return;
    }

    els.inspectorEmpty.classList.add("hidden");
    els.inspectorContent.classList.remove("hidden");

    const stats = node.stats || {};
    const displayName = node.name || (state.tree ? state.tree.projectName : "root");
    const displayPath = node.relPath || ".";
    const isRoot = !node.relPath;

    els.inspName.textContent = displayName;
    els.inspKind.textContent = node.isDir ? "directory" : "file";
    els.inspExcludeBtn.disabled = isRoot;
    els.inspExcludeBtn.title = isRoot ? "Cannot exclude workspace root" : "Exclude " + displayPath + " (X)";

    els.inspPath.textContent = displayPath;
    els.inspPath.title = displayPath;

    els.inspLine.textContent = line && line > 0 ? "line " + line : "line -";
    els.inspLang.textContent = node.isDir ? (node.moduleName || "folder") : (stats.language || "Text");

    els.inspMetricsLines.textContent =
      formatNumber(stats.sourceLineCount) + " source lines · " +
      formatNumber(stats.lineCount) + " total · " +
      formatBytes(stats.byteSize);

    els.inspMetricsTree.textContent =
      formatNumber(stats.childrenCount) + " children · " +
      formatNumber(stats.descendantCount) + " descendants";

    renderInspectorAnalysis(node.symbols || [], [], []);

    if (!node.isDir && node.relPath) {
      const targetPath = node.relPath;
      fetchFile(targetPath)
        .then(function (fileData) {
          if (!fileData || state.selectedPath !== targetPath) {
            return;
          }
          renderInspectorAnalysis(fileData.symbols, fileData.imports, fileData.topCalls);
        })
        .catch(function () {
        });
    }
  }

  function pushHistory(entry) {
    if (!entry) {
      return;
    }
    const trimmed = state.history.slice(0, state.historyIndex + 1);
    const prev = trimmed.length > 0 ? trimmed[trimmed.length - 1] : null;
    if (
      prev &&
      prev.relPath === entry.relPath &&
      prev.line === entry.line &&
      prev.actionKind === entry.actionKind
    ) {
      prev.cameraState = entry.cameraState;
      prev.timestamp = entry.timestamp;
      state.history = trimmed;
      state.historyIndex = trimmed.length - 1;
      renderHistory();
      return;
    }
    trimmed.push(entry);
    if (trimmed.length > HISTORY_MAX) {
      trimmed.shift();
    }
    state.history = trimmed;
    state.historyIndex = trimmed.length - 1;
    renderHistory();
  }

  function renderHistory() {
    els.historyBackBtn.disabled = state.historyIndex <= 0;
    els.historyForwardBtn.disabled = state.historyIndex < 0 || state.historyIndex >= state.history.length - 1;

    if (state.history.length === 0) {
      els.historyTabCount.classList.add("hidden");
      els.historyList.innerHTML = '<div class="empty-state">Navigation history will appear here as you explore files and search hits.</div>';
      return;
    }

    els.historyTabCount.textContent = String(state.history.length);
    els.historyTabCount.classList.remove("hidden");
    els.historyList.innerHTML = "";

    for (let i = state.history.length - 1; i >= 0; i--) {
      const item = state.history[i];
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "history-item" + (i === state.historyIndex ? " active" : "");

      const top = document.createElement("div");
      top.className = "history-top";
      const kind = document.createElement("span");
      kind.className = "history-kind";
      kind.textContent = item.actionKind || "select";
      const time = document.createElement("span");
      time.className = "history-time";
      time.textContent = formatTime(item.timestamp);
      top.appendChild(kind);
      top.appendChild(time);

      const pathEl = document.createElement("div");
      pathEl.className = "history-path";
      const label = item.relPath || (state.tree ? state.tree.projectName : "root");
      pathEl.textContent = item.line ? label + ":" + item.line : label;
      pathEl.title = pathEl.textContent;

      btn.appendChild(top);
      btn.appendChild(pathEl);

      btn.addEventListener("click", (function (idx) {
        return function () {
          restoreHistoryAt(idx);
        };
      })(i));

      els.historyList.appendChild(btn);
    }
  }

  function restoreHistoryAt(index) {
    if (index < 0 || index >= state.history.length) {
      return;
    }
    state.historyIndex = index;
    const entry = state.history[index];
    const node = state.nodeByPath.get(entry.relPath) || null;
    state.selectedPath = entry.relPath;
    state.selectedLine = entry.line || null;
    renderBreadcrumb();
    renderInspector(node, state.selectedLine);
    renderHistory();

    if (state.renderer) {
      state.renderer.setSelected(entry.relPath, entry.line || null);
      if (entry.cameraState) {
        if (typeof entry.cameraState.is3D === "boolean") {
          syncModeButtons(entry.cameraState.is3D);
        }
        state.renderer.setCameraState(entry.cameraState, true);
      } else {
        state.renderer.flyToNode(entry.relPath, entry.line || null, true);
      }
    }
  }

  function applySelection(node, line, pushEntry, actionKind, overrideCamera) {
    const relPath = node ? node.relPath : "";
    const resolvedLine = line && line > 0 ? line : null;
    state.selectedPath = relPath;
    state.selectedLine = resolvedLine;
    renderBreadcrumb();
    renderInspector(node, resolvedLine);

    if (pushEntry && node) {
      const cam = overrideCamera !== undefined
        ? overrideCamera
        : (state.renderer ? state.renderer.getCameraState() : null);
      pushHistory({
        relPath: relPath,
        name: node.name,
        isDir: Boolean(node.isDir),
        line: resolvedLine,
        actionKind: actionKind || (resolvedLine ? "line" : "select"),
        cameraState: cam,
        timestamp: Date.now()
      });
    }
  }

  function selectAndFly(relPath, line, actionKind) {
    const key = relPath || "";
    const node = state.nodeByPath.get(key) || (state.tree && state.tree.nodes && state.tree.nodes[0]) || null;
    if (!node) {
      return;
    }
    const targetLine = line && line > 0 ? line : null;
    if (state.renderer) {
      if (!node.isDir) {
        state.renderer.focusFile(node.relPath, targetLine, true);
      } else {
        state.renderer.clearFileFocus(false);
        state.renderer.setSelected(node.relPath, targetLine);
        state.renderer.flyToNode(node.relPath, targetLine, true);
      }
    }
    applySelection(node, targetLine, true, actionKind || "fly", null);
  }

  function highlightSnippetHtml(snippet, query) {
    const raw = String(snippet || "");
    const q = String(query || "").trim();
    if (!q) {
      return escapeHtml(raw);
    }
    const lowerRaw = raw.toLowerCase();
    const lowerQ = q.toLowerCase();
    let out = "";
    let cursor = 0;
    while (cursor < raw.length) {
      const idx = lowerRaw.indexOf(lowerQ, cursor);
      if (idx === -1) {
        out += escapeHtml(raw.slice(cursor));
        break;
      }
      out += escapeHtml(raw.slice(cursor, idx));
      out += "<mark>" + escapeHtml(raw.slice(idx, idx + q.length)) + "</mark>";
      cursor = idx + q.length;
    }
    return out;
  }

  async function runSearch(query) {
    const trimmed = String(query || "").trim();
    els.searchClearBtn.classList.toggle("hidden", query.length === 0);

    if (state.searchAbort) {
      state.searchAbort.abort();
      state.searchAbort = null;
    }

    if (!trimmed) {
      els.searchStatus.textContent = "Type to search indexed files";
      els.searchResults.innerHTML = "";
      els.searchTabCount.classList.add("hidden");
      if (state.renderer) {
        state.renderer.setSearchHighlights({}, {}, "");
      }
      return;
    }

    const controller = new AbortController();
    state.searchAbort = controller;

    try {
      const res = await apiGetJson(
        "/api/search?q=" + encodeURIComponent(trimmed) + "&limit=200",
        controller.signal
      );
      if (state.searchAbort !== controller) {
        return;
      }

      const matches = Array.isArray(res.matches) ? res.matches : [];
      const fileHits = res.fileHits || {};
      const matchedLinesMap = {};
      for (const m of matches) {
        if (!matchedLinesMap[m.relPath]) {
          matchedLinesMap[m.relPath] = new Set();
        }
        matchedLinesMap[m.relPath].add(m.line);
      }

      if (state.renderer) {
        state.renderer.setSearchHighlights(fileHits, matchedLinesMap, trimmed);
      }

      const elapsed = Number(res.elapsedMs || 0).toFixed(1);
      els.searchStatus.textContent = "Search: " + formatNumber(res.totalMatches) + " rows in " + elapsed + " ms";

      if (res.totalMatches > 0) {
        els.searchTabCount.textContent = res.totalMatches > 999 ? "999+" : String(res.totalMatches);
        els.searchTabCount.classList.remove("hidden");
      } else {
        els.searchTabCount.classList.add("hidden");
      }

      els.searchResults.innerHTML = "";
      if (matches.length === 0) {
        const empty = document.createElement("div");
        empty.className = "empty-state";
        empty.textContent = "No matches found for \"" + trimmed + "\".";
        els.searchResults.appendChild(empty);
        return;
      }

      const frag = document.createDocumentFragment();
      for (const hit of matches) {
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = "search-hit";

        const loc = document.createElement("div");
        loc.className = "search-hit-loc";
        loc.textContent = hit.relPath + ":" + hit.line;
        loc.title = loc.textContent;

        const snip = document.createElement("div");
        snip.className = "search-hit-snippet";
        snip.innerHTML = highlightSnippetHtml(hit.snippet, trimmed);

        btn.appendChild(loc);
        btn.appendChild(snip);
        btn.addEventListener("click", function () {
          selectAndFly(hit.relPath, hit.line, "search");
        });
        frag.appendChild(btn);
      }
      els.searchResults.appendChild(frag);
    } catch (err) {
      if (err && err.name === "AbortError") {
        return;
      }
      els.searchStatus.textContent = "Search failed: " + (err ? err.message : "error");
    }
  }

  function isSafeHref(href) {
    const trimmed = String(href || "").trim();
    if (!trimmed) {
      return false;
    }
    const lower = trimmed.toLowerCase();
    if (
      lower.startsWith("http:" + DOUBLE_SLASH) ||
      lower.startsWith("https:" + DOUBLE_SLASH) ||
      lower.startsWith("mailto:") ||
      lower.startsWith("#")
    ) {
      return true;
    }
    if (lower.startsWith("javascript:") || lower.startsWith("data:") || lower.startsWith("vbscript:")) {
      return false;
    }
    return true;
  }

  function formatInlineMarkdown(raw) {
    let text = escapeHtml(raw);
    const codeTokens = [];
    text = text.replace(/`([^`]+)`/g, function (_, code) {
      const idx = codeTokens.length;
      codeTokens.push("<code>" + code + "</code>");
      return "\u0000CODE" + idx + "\u0000";
    });

    text = text.replace(/\[([^\]]+)\]\(([^)]+)\)/g, function (_, label, url) {
      const decodedUrl = url.replace(/&amp;/g, "&");
      if (!isSafeHref(decodedUrl)) {
        return label;
      }
      const lower = decodedUrl.toLowerCase();
      const external = lower.startsWith("http:" + DOUBLE_SLASH) || lower.startsWith("https:" + DOUBLE_SLASH);
      if (external) {
        return '<a href="' + escapeHtml(decodedUrl) + '" target="_blank" rel="noopener noreferrer">' + label + "</a>";
      }
      return '<a href="' + escapeHtml(decodedUrl) + '" data-md-link="' + escapeHtml(decodedUrl) + '">' + label + "</a>";
    });

    text = text.replace(new RegExp("\\x2a\\x2a([^*]+)\\x2a\\x2a", "g"), "<strong>$1</strong>");
    text = text.replace(new RegExp("\\x2a([^*]+)\\x2a", "g"), "<em>$1</em>");

    text = text.replace(/\u0000CODE(\d+)\u0000/g, function (_, idx) {
      return codeTokens[Number(idx)] || "";
    });
    return text;
  }

  function renderCodeBlockHtml(lines, lang) {
    const tok = window.ConfineTokenizer;
    if (!tok || typeof tok.tokenizeLine !== "function") {
      return "<pre><code>" + escapeHtml(lines.join("\n")) + "</code></pre>";
    }
    const htmlLines = lines.map(function (line) {
      const tokens = tok.tokenizeLine(line, lang || "Text");
      return tokens
        .map(function (t) {
          const color = tok.tokenColor(t.type);
          return '<span style="color:' + escapeHtml(color) + '">' + escapeHtml(t.text) + "</span>";
        })
        .join("");
    });
    return "<pre><code>" + htmlLines.join("\n") + "</code></pre>";
  }

  function renderMarkdownToHtml(lines) {
    const arr = Array.isArray(lines) ? lines : String(lines || "").split(/\r?\n/);
    const out = [];
    let i = 0;

    while (i < arr.length) {
      const line = arr[i];
      const trimmed = line.trim();

      if (trimmed.startsWith("```")) {
        const lang = trimmed.slice(3).trim();
        const codeLines = [];
        i++;
        while (i < arr.length && !arr[i].trim().startsWith("```")) {
          codeLines.push(arr[i]);
          i++;
        }
        if (i < arr.length) {
          i++;
        }
        out.push(renderCodeBlockHtml(codeLines, lang));
        continue;
      }

      if (!trimmed) {
        i++;
        continue;
      }

      if (/^(-{3,}|\*{3,}|_{3,})$/.test(trimmed)) {
        out.push("<hr>");
        i++;
        continue;
      }

      const headingMatch = /^(#{1,6})\s+(.+)$/.exec(trimmed);
      if (headingMatch) {
        const level = headingMatch[1].length;
        out.push("<h" + level + ">" + formatInlineMarkdown(headingMatch[2]) + "</h" + level + ">");
        i++;
        continue;
      }

      if (
        trimmed.includes("|") &&
        i + 1 < arr.length &&
        /^\|?(\s*:?-{2,}:?\s*\|)+\s*:?-{2,}:?\s*\|?$/.test(arr[i + 1].trim())
      ) {
        const splitRow = function (rowStr) {
          return rowStr
            .trim()
            .replace(/^\|/, "")
            .replace(/\|$/, "")
            .split("|")
            .map(function (c) {
              return c.trim();
            });
        };
        const headers = splitRow(trimmed);
        i += 2;
        let tableHtml = "<table><thead><tr>" + headers.map(function (h) {
          return "<th>" + formatInlineMarkdown(h) + "</th>";
        }).join("") + "</tr></thead><tbody>";
        while (i < arr.length && arr[i].trim().includes("|") && arr[i].trim() !== "") {
          const cells = splitRow(arr[i]);
          tableHtml += "<tr>" + cells.map(function (c) {
            return "<td>" + formatInlineMarkdown(c) + "</td>";
          }).join("") + "</tr>";
          i++;
        }
        tableHtml += "</tbody></table>";
        out.push(tableHtml);
        continue;
      }

      if (trimmed.startsWith(">")) {
        const quoteLines = [];
        while (i < arr.length && arr[i].trim().startsWith(">")) {
          quoteLines.push(arr[i].trim().replace(/^>\s?/, ""));
          i++;
        }
        out.push("<blockquote>" + quoteLines.map(formatInlineMarkdown).join("<br>") + "</blockquote>");
        continue;
      }

      if (/^[-*+]\s+/.test(trimmed)) {
        const items = [];
        while (i < arr.length && /^[-*+]\s+/.test(arr[i].trim())) {
          items.push("<li>" + formatInlineMarkdown(arr[i].trim().replace(/^[-*+]\s+/, "")) + "</li>");
          i++;
        }
        out.push("<ul>" + items.join("") + "</ul>");
        continue;
      }

      if (/^\d+\.\s+/.test(trimmed)) {
        const items = [];
        while (i < arr.length && /^\d+\.\s+/.test(arr[i].trim())) {
          items.push("<li>" + formatInlineMarkdown(arr[i].trim().replace(/^\d+\.\s+/, "")) + "</li>");
          i++;
        }
        out.push("<ol>" + items.join("") + "</ol>");
        continue;
      }

      const paraLines = [];
      while (
        i < arr.length &&
        arr[i].trim() !== "" &&
        !arr[i].trim().startsWith("```") &&
        !/^(#{1,6})\s+/.test(arr[i].trim()) &&
        !/^[-*+]\s+/.test(arr[i].trim()) &&
        !/^\d+\.\s+/.test(arr[i].trim()) &&
        !arr[i].trim().startsWith(">")
      ) {
        paraLines.push(arr[i].trim());
        i++;
      }
      out.push("<p>" + paraLines.map(formatInlineMarkdown).join(" ") + "</p>");
    }

    return out.join("\n");
  }

  async function openMarkdownPreview(relPath) {
    if (!relPath) {
      return;
    }
    state.activeMarkdownPath = relPath;
    renderMarkdownList();
    els.markdownPreviewTitle.textContent = relPath;
    els.markdownPreviewWrap.classList.remove("hidden");
    els.markdownPreviewBody.innerHTML = '<div class="empty-state">Loading preview...</div>';

    try {
      const fileData = await fetchFile(relPath);
      if (state.activeMarkdownPath !== relPath) {
        return;
      }
      els.markdownPreviewBody.innerHTML = renderMarkdownToHtml(fileData.lines || []);
    } catch (err) {
      els.markdownPreviewBody.innerHTML = '<div class="empty-state">Failed to load markdown: ' + escapeHtml(err.message) + "</div>";
    }
  }

  function renderMarkdownList() {
    const filter = els.markdownFilter.value.trim().toLowerCase();
    const filtered = state.markdowns.filter(function (item) {
      if (!filter) {
        return true;
      }
      return (
        (item.title || "").toLowerCase().includes(filter) ||
        (item.relPath || "").toLowerCase().includes(filter) ||
        (item.name || "").toLowerCase().includes(filter)
      );
    });

    els.markdownCount.textContent = String(filtered.length);
    els.markdownList.innerHTML = "";

    if (filtered.length === 0) {
      const empty = document.createElement("div");
      empty.className = "empty-state";
      empty.textContent = state.markdowns.length === 0 ? "No markdown documents found in workspace." : "No matching markdown files.";
      els.markdownList.appendChild(empty);
      return;
    }

    for (const item of filtered) {
      const row = document.createElement("div");
      row.className = "markdown-item" + (state.activeMarkdownPath === item.relPath ? " active" : "");

      const openBtn = document.createElement("button");
      openBtn.type = "button";
      openBtn.className = "markdown-open-btn";
      const title = document.createElement("span");
      title.className = "markdown-title";
      title.textContent = item.title || item.name || item.relPath;
      const rel = document.createElement("span");
      rel.className = "markdown-rel";
      rel.textContent = item.relPath + " (" + formatNumber(item.lineCount) + " lines)";
      openBtn.appendChild(title);
      openBtn.appendChild(rel);
      openBtn.addEventListener("click", function () {
        openMarkdownPreview(item.relPath);
      });

      const mapBtn = document.createElement("button");
      mapBtn.type = "button";
      mapBtn.className = "btn btn-xs";
      mapBtn.textContent = "Map";
      mapBtn.title = "Fly to " + item.relPath;
      mapBtn.addEventListener("click", function () {
        selectAndFly(item.relPath, null, "markdown");
      });

      row.appendChild(openBtn);
      row.appendChild(mapBtn);
      els.markdownList.appendChild(row);
    }
  }

  async function loadMarkdowns() {
    try {
      const res = await apiGetJson("/api/markdowns");
      state.markdowns = Array.isArray(res.items) ? res.items : [];
      if (state.markdowns.length > 0) {
        els.markdownsTabCount.textContent = String(state.markdowns.length);
        els.markdownsTabCount.classList.remove("hidden");
      } else {
        els.markdownsTabCount.classList.add("hidden");
      }
      renderMarkdownList();
    } catch (err) {
      showToast("Failed to load markdown catalog: " + err.message, "error");
    }
  }

  async function loadTree(preset) {
    const activePreset = preset || state.preset || "sloc";
    state.preset = activePreset;
    els.presetSelect.value = activePreset;

    const tree = await apiGetJson("/api/tree?preset=" + encodeURIComponent(activePreset));
    state.tree = tree;
    state.nodeByPath.clear();
    state.nodeById.clear();

    if (Array.isArray(tree.excludes)) {
      syncExcludesState(tree.excludes);
    }

    const nodes = Array.isArray(tree.nodes) ? tree.nodes : [];
    for (const node of nodes) {
      state.nodeByPath.set(node.relPath || "", node);
      state.nodeById.set(node.id, node);
    }

    els.projectTag.textContent = tree.projectName || "workspace";
    els.workspacePath.value = tree.rootPath || "";
    const stats = tree.stats || {};
    els.statFiles.textContent = formatNumber(stats.totalFiles) + " files";
    els.statSloc.textContent = formatNumber(stats.totalSloc) + " SLOC";

    const hasNodes = nodes.length > 1 || (nodes.length === 1 && nodes[0].stats && nodes[0].stats.descendantCount > 0);
    els.viewportEmpty.classList.toggle("hidden", hasNodes);

    if (state.renderer) {
      state.renderer.setTree(tree);
    }

    if (state.selectedPath !== null && state.nodeByPath.has(state.selectedPath)) {
      const sel = state.nodeByPath.get(state.selectedPath);
      renderBreadcrumb();
      renderInspector(sel, state.selectedLine);
      if (state.renderer) {
        state.renderer.setSelected(state.selectedPath, state.selectedLine);
      }
    } else if (nodes.length > 0) {
      state.selectedPath = "";
      state.selectedLine = null;
      renderBreadcrumb();
      renderInspector(nodes[0], null);
      if (state.renderer) {
        state.renderer.setSelected("", null);
      }
    } else {
      state.selectedPath = null;
      state.selectedLine = null;
      renderBreadcrumb();
      renderInspector(null, null);
      if (state.renderer) {
        state.renderer.setSelected(null, null);
      }
    }
  }

  async function openWorkspace(dirPath) {
    const trimmed = String(dirPath || "").trim();
    if (!trimmed) {
      showToast("Enter a valid directory path.", "error");
      return;
    }

    els.openWorkspaceBtn.disabled = true;
    els.workspaceSpinner.classList.remove("hidden");

    try {
      const res = await apiPostJson("/api/open", { path: trimmed });
      state.fileCache.clear();
      state.inflightFiles.clear();
      state.selectedPath = "";
      state.selectedLine = null;
      state.activeMarkdownPath = null;
      els.markdownPreviewWrap.classList.add("hidden");
      els.searchInput.value = "";
      state.searchQuery = "";
      els.searchStatus.textContent = "Type to search indexed files";
      els.searchResults.innerHTML = "";
      els.searchTabCount.classList.add("hidden");
      els.searchClearBtn.classList.add("hidden");

      syncExcludesState(res.excludes || []);

      if (state.renderer) {
        state.renderer.setSearchHighlights({}, {}, "");
      }

      await Promise.all([loadTree(state.preset), loadMarkdowns()]);
      if (state.renderer) {
        state.renderer.fitView(false);
      }
      if (state.dirsOpen) {
        loadFs(res.rootPath || trimmed);
      }
      showToast("Opened " + (res.projectName || res.rootPath), "success");
    } catch (err) {
      showToast("Open failed: " + err.message, "error");
    } finally {
      els.openWorkspaceBtn.disabled = false;
      els.workspaceSpinner.classList.add("hidden");
    }
  }

  function setupResizer() {
    let dragging = false;
    let startX = 0;
    let startWidth = 368;

    function applyWidth(w) {
      const maxAllowed = Math.max(280, Math.min(720, window.innerWidth - 260));
      const clamped = Math.max(260, Math.min(maxAllowed, w));
      document.documentElement.style.setProperty("--sidebar-width", clamped + "px");
      els.splitHandle.setAttribute("aria-valuemax", String(maxAllowed));
      els.splitHandle.setAttribute("aria-valuenow", String(Math.round(clamped)));
      resizeViewport();
    }

    els.splitHandle.addEventListener("mousedown", function (e) {
      dragging = true;
      startX = e.clientX;
      startWidth = els.sidebar.getBoundingClientRect().width;
      els.splitHandle.classList.add("dragging");
      e.preventDefault();
    });

    window.addEventListener("mousemove", function (e) {
      if (!dragging) {
        return;
      }
      const delta = startX - e.clientX;
      applyWidth(startWidth + delta);
    });

    window.addEventListener("mouseup", function () {
      if (!dragging) {
        return;
      }
      dragging = false;
      els.splitHandle.classList.remove("dragging");
    });

    els.splitHandle.addEventListener("keydown", function (e) {
      const current = els.sidebar.getBoundingClientRect().width;
      if (e.key === "ArrowLeft") {
        applyWidth(current + 24);
        e.preventDefault();
      } else if (e.key === "ArrowRight") {
        applyWidth(current - 24);
        e.preventDefault();
      }
    });
  }

  function isEditingInput(target) {
    if (!target) {
      return false;
    }
    const tag = target.tagName;
    return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || target.isContentEditable;
  }

  function bindEvents() {
    for (let i = 0; i < els.tabBtns.length; i++) {
      const btn = els.tabBtns[i];
      btn.addEventListener("click", function () {
        switchTab(btn.dataset.tab);
      });
      btn.addEventListener("keydown", function (e) {
        let nextIndex = -1;
        if (e.key === "ArrowRight") {
          nextIndex = (i + 1) % els.tabBtns.length;
        } else if (e.key === "ArrowLeft") {
          nextIndex = (i - 1 + els.tabBtns.length) % els.tabBtns.length;
        } else if (e.key === "Home") {
          nextIndex = 0;
        } else if (e.key === "End") {
          nextIndex = els.tabBtns.length - 1;
        }
        if (nextIndex >= 0) {
          e.preventDefault();
          const targetBtn = els.tabBtns[nextIndex];
          switchTab(targetBtn.dataset.tab);
          targetBtn.focus();
        }
      });
    }

    els.workspaceForm.addEventListener("submit", function (e) {
      e.preventDefault();
      openWorkspace(els.workspacePath.value);
    });

    els.browseFolderBtn.addEventListener("click", function () {
      pickFolderNative();
    });

    els.dirsToggleBtn.addEventListener("click", function () {
      setDirsOpen(!state.dirsOpen);
    });

    els.dirsCloseBtn.addEventListener("click", function () {
      setDirsOpen(false);
    });

    els.fsUpBtn.addEventListener("click", function () {
      if (state.fsParentPath) {
        loadFs(state.fsParentPath);
      }
    });

    els.fsOpenCurrentBtn.addEventListener("click", function () {
      if (state.fsCurrentPath) {
        openWorkspace(state.fsCurrentPath);
      }
    });

    els.excludesToggleBtn.addEventListener("click", function () {
      setExcludesOpen(!state.excludesOpen);
    });

    els.excludesCloseBtn.addEventListener("click", function () {
      setExcludesOpen(false);
    });

    els.excludeForm.addEventListener("submit", function (e) {
      e.preventDefault();
      const pat = els.excludePatternInput.value.trim();
      if (!pat) {
        return;
      }
      els.excludePatternInput.value = "";
      applyExcludeMutation({ add: pat }, "Excluded " + pat);
    });

    for (const btn of els.quickExcludeBtns) {
      btn.addEventListener("click", function () {
        const key = btn.dataset.preset || "";
        const patterns = QUICK_EXCLUDE_PRESETS[key] || [];
        if (patterns.length === 0) {
          return;
        }
        const activeSet = new Set(state.excludes);
        const allPresent = patterns.every(function (p) {
          return activeSet.has(p);
        });
        let next;
        if (allPresent) {
          const removeSet = new Set(patterns);
          next = state.excludes.filter(function (p) {
            return !removeSet.has(p);
          });
        } else {
          next = state.excludes.slice();
          for (const p of patterns) {
            if (!activeSet.has(p)) {
              next.push(p);
            }
          }
        }
        applyExcludeMutation(
          { excludes: next },
          (allPresent ? "Removed " : "Applied ") + btn.textContent + " exclusions"
        );
      });
    }

    els.excludeClearBtn.addEventListener("click", function () {
      if (state.excludes.length === 0) {
        return;
      }
      applyExcludeMutation({ clear: true }, "Cleared all exclusions");
    });

    els.mode2dBtn.addEventListener("click", function () {
      if (state.renderer) {
        state.renderer.setMode3D(false);
      }
      syncModeButtons(false);
    });

    els.mode3dBtn.addEventListener("click", function () {
      if (state.renderer) {
        state.renderer.setMode3D(true);
      }
      syncModeButtons(true);
    });

    els.fitBtn.addEventListener("click", function () {
      if (state.renderer) {
        state.renderer.fitView(true);
      }
    });

    els.presetSelect.addEventListener("change", function () {
      loadTree(els.presetSelect.value).catch(function (err) {
        showToast("Preset switch failed: " + err.message, "error");
      });
    });

    els.inspFlyBtn.addEventListener("click", function () {
      if (state.selectedPath !== null) {
        selectAndFly(state.selectedPath, state.selectedLine, "fly");
      }
    });

    els.inspExcludeBtn.addEventListener("click", function () {
      excludeSelectedNode();
    });

    els.copyPathBtn.addEventListener("click", function () {
      const text = state.selectedPath || (state.tree ? state.tree.rootPath : "");
      if (!text) {
        return;
      }
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(function () {
          showToast("Copied path: " + text, "success");
        }).catch(function () {
          showToast("Clipboard permission denied", "error");
        });
      } else {
        showToast(text, "");
      }
    });

    els.searchInput.addEventListener("input", function () {
      const q = els.searchInput.value;
      state.searchQuery = q;
      if (state.searchDebounce) {
        clearTimeout(state.searchDebounce);
      }
      state.searchDebounce = setTimeout(function () {
        runSearch(q);
      }, SEARCH_DEBOUNCE_MS);
    });

    els.searchClearBtn.addEventListener("click", function () {
      els.searchInput.value = "";
      state.searchQuery = "";
      runSearch("");
      els.searchInput.focus();
    });

    els.historyBackBtn.addEventListener("click", function () {
      if (state.historyIndex > 0) {
        restoreHistoryAt(state.historyIndex - 1);
      }
    });

    els.historyForwardBtn.addEventListener("click", function () {
      if (state.historyIndex < state.history.length - 1) {
        restoreHistoryAt(state.historyIndex + 1);
      }
    });

    els.historyClearBtn.addEventListener("click", function () {
      state.history = [];
      state.historyIndex = -1;
      renderHistory();
    });

    els.markdownFilter.addEventListener("input", function () {
      renderMarkdownList();
    });

    els.markdownPreviewMapBtn.addEventListener("click", function () {
      if (state.activeMarkdownPath) {
        selectAndFly(state.activeMarkdownPath, null, "markdown");
      }
    });

    els.markdownPreviewCloseBtn.addEventListener("click", function () {
      state.activeMarkdownPath = null;
      els.markdownPreviewWrap.classList.add("hidden");
      renderMarkdownList();
    });

    els.markdownPreviewBody.addEventListener("click", function (e) {
      const link = e.target.closest("a[data-md-link]");
      if (!link) {
        return;
      }
      e.preventDefault();
      const target = link.getAttribute("data-md-link") || "";
      if (!target || target.startsWith("#")) {
        return;
      }
      const rawTarget = target.split("#")[0];
      const cleanTarget = rawTarget.startsWith("./") ? rawTarget.slice(2) : rawTarget;
      const found = state.markdowns.find(function (m) {
        return m.relPath === cleanTarget || m.name === cleanTarget;
      });
      if (found) {
        openMarkdownPreview(found.relPath);
      }
    });

    window.addEventListener("keydown", function (e) {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        switchTab("search");
        els.searchInput.focus();
        els.searchInput.select();
        return;
      }

      if (e.altKey && e.key === "ArrowLeft") {
        if (state.historyIndex > 0) {
          e.preventDefault();
          restoreHistoryAt(state.historyIndex - 1);
        }
        return;
      }

      if (e.altKey && e.key === "ArrowRight") {
        if (state.historyIndex < state.history.length - 1) {
          e.preventDefault();
          restoreHistoryAt(state.historyIndex + 1);
        }
        return;
      }

      if (e.key === "Escape") {
        if (document.activeElement && isEditingInput(document.activeElement)) {
          if (document.activeElement === els.searchInput && els.searchInput.value) {
            els.searchInput.value = "";
            state.searchQuery = "";
            runSearch("");
            return;
          }
          document.activeElement.blur();
          return;
        }
        if (state.dirsOpen) {
          setDirsOpen(false);
          return;
        }
        if (state.excludesOpen) {
          setExcludesOpen(false);
          return;
        }
        if (state.renderer && state.renderer.isFileFocused()) {
          state.renderer.clearFileFocus(true);
          return;
        }
        return;
      }

      if (isEditingInput(e.target) || e.ctrlKey || e.metaKey || e.altKey) {
        return;
      }

      const keyLower = e.key.toLowerCase();
      if (e.key === "/") {
        e.preventDefault();
        switchTab("search");
        els.searchInput.focus();
        els.searchInput.select();
      } else if (e.key === "2") {
        if (state.renderer) {
          state.renderer.setMode3D(false);
        }
        syncModeButtons(false);
      } else if (e.key === "3") {
        if (state.renderer) {
          state.renderer.setMode3D(true);
        }
        syncModeButtons(true);
      } else if (keyLower === "m") {
        const next = !state.is3D;
        if (state.renderer) {
          state.renderer.setMode3D(next);
        }
        syncModeButtons(next);
      } else if (keyLower === "f" || keyLower === "r") {
        if (state.renderer) {
          state.renderer.fitView(true);
        }
      } else if (keyLower === "x") {
        e.preventDefault();
        excludeSelectedNode();
      }
    });
  }

  function startFpsSampler() {
    function tick(now) {
      state.fpsFrames++;
      const elapsed = now - state.fpsLastTime;
      if (elapsed >= 500) {
        const fps = Math.round((state.fpsFrames * 1000) / elapsed);
        els.statFps.textContent = fps + " FPS";
        state.fpsFrames = 0;
        state.fpsLastTime = now;
      }
      requestAnimationFrame(tick);
    }
    requestAnimationFrame(tick);
  }

  function initRenderer() {
    if (typeof window.ConfineRenderer !== "function") {
      showToast("Renderer script failed to load.", "error");
      return;
    }

    try {
      state.renderer = new window.ConfineRenderer(els.glCanvas, els.overlayCanvas, {
        onSelectNode: function (node, line, pushHistoryFlag) {
          applySelection(node, line, pushHistoryFlag !== false, line ? "line" : "select");
        },
        onHoverNode: function (node, line) {
          if (!node) {
            els.statHover.textContent = "Ready";
            els.statHover.title = "";
            return;
          }
          const pathText = node.relPath || (state.tree ? state.tree.projectName : "root");
          const label = line && line > 0 ? pathText + ":" + line : pathText;
          els.statHover.textContent = label;
          els.statHover.title = label;
        },
        onModeChange: function (is3D) {
          syncModeButtons(is3D);
        },
        onRequestFile: function (relPath) {
          return fetchFile(relPath);
        }
      });

      const ro = new ResizeObserver(function () {
        resizeViewport();
      });
      ro.observe(els.viewportPane);
    } catch (err) {
      showToast("WebGL2 renderer error: " + err.message, "error");
    }
  }

  async function init() {
    setupResizer();
    bindEvents();
    initRenderer();
    startFpsSampler();

    try {
      await Promise.all([loadTree("sloc"), loadMarkdowns()]);
    } catch (err) {
      showToast("Initial load failed: " + err.message, "error");
    }
  }

  init();
})();
