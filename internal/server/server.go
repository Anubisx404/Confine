package server

import (
	"bytes"
	"confine/internal/analyzer"
	"confine/internal/indexer"
	"confine/internal/search"
	"confine/internal/treemap"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type WorkspaceStats struct {
	TotalFiles int   `json:"totalFiles"`
	TotalDirs  int   `json:"totalDirs"`
	TotalLines int   `json:"totalLines"`
	TotalSloc  int   `json:"totalSloc"`
	TotalBytes int64 `json:"totalBytes"`
}

type TreeResponse struct {
	RootPath    string               `json:"rootPath"`
	ProjectName string               `json:"projectName"`
	Preset      string               `json:"preset"`
	Excludes    []string             `json:"excludes"`
	Stats       WorkspaceStats       `json:"stats"`
	CanvasRect  treemap.Rect         `json:"canvasRect"`
	Nodes       []treemap.LayoutNode `json:"nodes"`
}

type FileResponse struct {
	Path            string                `json:"path"`
	Name            string                `json:"name"`
	Language        string                `json:"language"`
	ByteSize        int64                 `json:"byteSize"`
	LineCount       int                   `json:"lineCount"`
	SourceLineCount int                   `json:"sourceLineCount"`
	Lines           []string              `json:"lines"`
	Symbols         []analyzer.Symbol     `json:"symbols"`
	Imports         []analyzer.ImportRef  `json:"imports"`
	TopCalls        []analyzer.CallMetric `json:"topCalls"`
}

type MarkdownsResponse struct {
	Count int                    `json:"count"`
	Items []search.MarkdownEntry `json:"items"`
}

type OpenRequest struct {
	Path     string   `json:"path"`
	Excludes []string `json:"excludes"`
}

type OpenResponse struct {
	Ok          bool           `json:"ok"`
	RootPath    string         `json:"rootPath"`
	ProjectName string         `json:"projectName"`
	Excludes    []string       `json:"excludes"`
	Stats       WorkspaceStats `json:"stats"`
}

type ExcludesRequest struct {
	Excludes []string `json:"excludes"`
	Add      string   `json:"add"`
	Remove   string   `json:"remove"`
	Clear    bool     `json:"clear"`
}

type FSEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	RelPath string `json:"relPath"`
	IsDir   bool   `json:"isDir"`
}

type FSListResponse struct {
	CurrentPath string    `json:"currentPath"`
	ParentPath  string    `json:"parentPath"`
	Roots       []string  `json:"roots"`
	Entries     []FSEntry `json:"entries"`
}

type PickFolderRequest struct {
	InitialDir string `json:"initialDir"`
}

type PickFolderResponse struct {
	Ok       bool   `json:"ok"`
	Canceled bool   `json:"canceled"`
	Path     string `json:"path"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type Snapshot struct {
	Tree     *indexer.TreeResult
	Excludes []string
	Layouts  map[string]*treemap.LayoutResult
	Search   *search.Index
	mu       sync.RWMutex
}

type Server struct {
	Addr             string
	Root             string
	WebDir           string
	FolderPickerFunc func(initialDir string) (string, bool, error)
	openMu           sync.Mutex
	snapshot         atomic.Pointer[Snapshot]
	srvMu            sync.Mutex
	httpSrv          *http.Server
}

func NewServer(rootPath string, addr string) (*Server, error) {
	return NewServerWithOptions(rootPath, addr, nil)
}

func NewServerWithOptions(rootPath string, addr string, excludes []string) (*Server, error) {
	s := &Server{
		Addr:             addr,
		WebDir:           resolveWebDir(""),
		FolderPickerFunc: defaultPickFolder,
	}
	if err := s.OpenWorkspaceWithExcludes(rootPath, excludes); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) OpenWorkspace(rootPath string) error {
	return s.OpenWorkspaceWithExcludes(rootPath, nil)
}

func (s *Server) OpenWorkspaceWithExcludes(rootPath string, excludes []string) error {
	trimmed := strings.TrimSpace(rootPath)
	if trimmed == "" {
		return errors.New("workspace path cannot be empty")
	}
	absRoot, err := filepath.Abs(trimmed)
	if err != nil {
		return fmt.Errorf("invalid workspace path: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return fmt.Errorf("workspace path not accessible: %w", err)
	}
	if !info.IsDir() {
		return errors.New("workspace path must be a directory")
	}

	normExcludes := indexer.NormalizeExcludes(excludes)

	s.openMu.Lock()
	defer s.openMu.Unlock()

	snap, err := buildSnapshot(absRoot, normExcludes)
	if err != nil {
		return fmt.Errorf("failed to index workspace: %w", err)
	}
	s.Root = snap.Tree.RootPath
	s.snapshot.Store(snap)
	return nil
}

func buildSnapshot(rootPath string, excludes []string) (*Snapshot, error) {
	normExcludes := indexer.NormalizeExcludes(excludes)
	tree, err := indexer.IndexDirectoryWithOptions(rootPath, normExcludes)
	if err != nil {
		return nil, err
	}
	slocLayout := treemap.ComputeLayout(tree, "sloc")
	searchIdx := search.NewIndex(tree, slocLayout)
	if extraDocs, scanErr := indexer.ScanProjectReadmesAndPlans(tree.RootPath, normExcludes); scanErr == nil && len(extraDocs) > 0 {
		searchIdx.AddSupplementalMarkdowns(extraDocs)
	}

	return &Snapshot{
		Tree:     tree,
		Excludes: normExcludes,
		Layouts:  map[string]*treemap.LayoutResult{"sloc": slocLayout},
		Search:   searchIdx,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tree", s.handleTree)
	mux.HandleFunc("/api/file", s.handleFile)
	mux.HandleFunc("/api/search", s.handleSearch)
	mux.HandleFunc("/api/markdowns", s.handleMarkdowns)
	mux.HandleFunc("/api/open", s.handleOpen)
	mux.HandleFunc("/api/excludes", s.handleExcludes)
	mux.HandleFunc("/api/fs", s.handleFS)
	mux.HandleFunc("/api/pick-folder", s.handlePickFolder)
	mux.HandleFunc("/", s.handleStatic)
	return mux
}

func (s *Server) ListenAndServe() error {
	addr := s.Addr
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

func (s *Server) Serve(ln net.Listener) error {
	httpSrv := &http.Server{
		Addr:              ln.Addr().String(),
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.srvMu.Lock()
	s.httpSrv = httpSrv
	s.srvMu.Unlock()
	return httpSrv.Serve(ln)
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.srvMu.Lock()
	httpSrv := s.httpSrv
	s.srvMu.Unlock()
	if httpSrv == nil {
		return nil
	}
	return httpSrv.Shutdown(ctx)
}

func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	snap := s.snapshot.Load()
	if snap == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace not initialized")
		return
	}
	preset := normalizePreset(r.URL.Query().Get("preset"))
	layout := snap.getOrComputeLayout(preset)
	if snap.Search != nil {
		snap.Search.UpdateLayout(layout)
	}

	nodes := layout.Nodes
	if nodes == nil {
		nodes = []treemap.LayoutNode{}
	}
	respPreset := layout.Preset
	if respPreset == "" {
		respPreset = preset
	}
	writeJSON(w, http.StatusOK, TreeResponse{
		RootPath:    snap.Tree.RootPath,
		ProjectName: snap.Tree.ProjectName,
		Preset:      respPreset,
		Excludes:    copyStrings(snap.Excludes),
		Stats:       statsFromTree(snap.Tree),
		CanvasRect:  layout.CanvasRect,
		Nodes:       nodes,
	})
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	snap := s.snapshot.Load()
	if snap == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace not initialized")
		return
	}
	rawPath := r.URL.Query().Get("path")
	cleaned, statusCode, errMsg := validateRelativePath(rawPath)
	if errMsg != "" {
		writeError(w, statusCode, errMsg)
		return
	}

	rootAbs := snap.Tree.RootPath
	joined := filepath.Join(rootAbs, cleaned)
	relCheck, err := filepath.Rel(rootAbs, joined)
	if err != nil || isEscapingRel(relCheck) {
		writeError(w, http.StatusForbidden, "path outside workspace root")
		return
	}

	realPath, statusCode, errMsg := resolveSafeFilePath(rootAbs, joined)
	if errMsg != "" {
		writeError(w, statusCode, errMsg)
		return
	}

	relSlash := filepath.ToSlash(cleaned)
	if node, ok := snap.Tree.FilesByPath[relSlash]; ok && node != nil && !node.IsDir {
		writeJSON(w, http.StatusOK, fileResponseFromNode(node))
		return
	}

	data, err := os.ReadFile(realPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to read file")
		return
	}
	fileName := filepath.Base(realPath)
	lang, isBinary := indexer.DetectLanguage(fileName, data)
	if isBinary {
		writeJSON(w, http.StatusOK, FileResponse{
			Path:            relSlash,
			Name:            fileName,
			Language:        "Binary",
			ByteSize:        int64(len(data)),
			LineCount:       0,
			SourceLineCount: 0,
			Lines:           []string{},
			Symbols:         []analyzer.Symbol{},
			Imports:         []analyzer.ImportRef{},
			TopCalls:        []analyzer.CallMetric{},
		})
		return
	}

	totalLines, sloc, lines := indexer.CountLines(lang, data)
	analysis := analyzer.Analyze(lang, string(data))
	writeJSON(w, http.StatusOK, FileResponse{
		Path:            relSlash,
		Name:            fileName,
		Language:        lang,
		ByteSize:        int64(len(data)),
		LineCount:       totalLines,
		SourceLineCount: sloc,
		Lines:           nonNilStrings(lines),
		Symbols:         nonNilSymbols(analysis.Symbols),
		Imports:         nonNilImports(analysis.Imports),
		TopCalls:        nonNilCalls(analysis.TopCalls),
	})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	snap := s.snapshot.Load()
	if snap == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace not initialized")
		return
	}
	q := r.URL.Query().Get("q")
	limit := 200
	if limitStr := strings.TrimSpace(r.URL.Query().Get("limit")); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit parameter")
			return
		}
		limit = parsed
	}
	res := snap.Search.Search(q, limit)
	if res.Matches == nil {
		res.Matches = []search.Match{}
	}
	if res.FileHits == nil {
		res.FileHits = map[string]int{}
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleMarkdowns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	snap := s.snapshot.Load()
	if snap == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace not initialized")
		return
	}
	items := snap.Search.Markdowns()
	if items == nil {
		items = []search.MarkdownEntry{}
	}
	writeJSON(w, http.StatusOK, MarkdownsResponse{
		Count: len(items),
		Items: items,
	})
}

func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req OpenRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if err := s.OpenWorkspaceWithExcludes(req.Path, req.Excludes); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	snap := s.snapshot.Load()
	writeJSON(w, http.StatusOK, OpenResponse{
		Ok:          true,
		RootPath:    snap.Tree.RootPath,
		ProjectName: snap.Tree.ProjectName,
		Excludes:    copyStrings(snap.Excludes),
		Stats:       statsFromTree(snap.Tree),
	})
}

func (s *Server) handleExcludes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req ExcludesRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	s.openMu.Lock()
	defer s.openMu.Unlock()

	snap := s.snapshot.Load()
	if snap == nil || snap.Tree == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace not initialized")
		return
	}

	next := copyStrings(snap.Excludes)
	if req.Clear {
		next = []string{}
	}
	if req.Excludes != nil {
		next = copyStrings(req.Excludes)
	}
	if strings.TrimSpace(req.Add) != "" {
		next = append(next, req.Add)
	}
	if strings.TrimSpace(req.Remove) != "" {
		remNorm := indexer.NormalizeExcludes([]string{req.Remove})
		if len(remNorm) > 0 {
			target := remNorm[0]
			currentNorm := indexer.NormalizeExcludes(next)
			filtered := make([]string, 0, len(currentNorm))
			for _, item := range currentNorm {
				if item == target {
					continue
				}
				filtered = append(filtered, item)
			}
			next = filtered
		}
	}

	newSnap, err := buildSnapshot(snap.Tree.RootPath, next)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to re-index workspace: %v", err))
		return
	}
	s.snapshot.Store(newSnap)

	writeJSON(w, http.StatusOK, OpenResponse{
		Ok:          true,
		RootPath:    newSnap.Tree.RootPath,
		ProjectName: newSnap.Tree.ProjectName,
		Excludes:    copyStrings(newSnap.Excludes),
		Stats:       statsFromTree(newSnap.Tree),
	})
}

func (s *Server) handleFS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	snap := s.snapshot.Load()
	rawPath := strings.TrimSpace(r.URL.Query().Get("path"))
	targetPath := ""
	if rawPath == "" {
		if snap != nil && snap.Tree != nil {
			targetPath = snap.Tree.RootPath
		} else if cwd, err := os.Getwd(); err == nil {
			targetPath = cwd
		}
	} else {
		if runtime.GOOS == "windows" && len(rawPath) == 2 && rawPath[1] == ':' &&
			((rawPath[0] >= 'A' && rawPath[0] <= 'Z') || (rawPath[0] >= 'a' && rawPath[0] <= 'z')) {
			rawPath += "\\"
		}
		abs, err := filepath.Abs(rawPath)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid path")
			return
		}
		targetPath = abs
	}

	if targetPath == "" {
		writeError(w, http.StatusBadRequest, "no directory path available")
		return
	}
	targetPath = filepath.Clean(targetPath)

	info, err := os.Stat(targetPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("directory not accessible: %v", err))
		return
	}
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, "path is not a directory")
		return
	}

	dirEntries, err := os.ReadDir(targetPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to read directory: %v", err))
		return
	}

	activeRoot := ""
	if snap != nil && snap.Tree != nil {
		activeRoot = snap.Tree.RootPath
	}

	entries := make([]FSEntry, 0, len(dirEntries))
	for _, de := range dirEntries {
		name := de.Name()
		full := filepath.Join(targetPath, name)
		isDir := de.IsDir()
		if !isDir && (de.Type()&os.ModeSymlink != 0) {
			if st, statErr := os.Stat(full); statErr == nil {
				isDir = st.IsDir()
			}
		}

		relPath := ""
		if activeRoot != "" {
			if rel, relErr := filepath.Rel(activeRoot, full); relErr == nil && rel != "." && !isEscapingRel(rel) {
				relPath = filepath.ToSlash(rel)
			}
		}

		entries = append(entries, FSEntry{
			Name:    name,
			Path:    full,
			RelPath: relPath,
			IsDir:   isDir,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		li := strings.ToLower(entries[i].Name)
		lj := strings.ToLower(entries[j].Name)
		if li != lj {
			return li < lj
		}
		return entries[i].Name < entries[j].Name
	})

	const maxFSEntries = 500
	if len(entries) > maxFSEntries {
		entries = entries[:maxFSEntries]
	}

	parentPath := filepath.Dir(targetPath)
	if parentPath == targetPath {
		parentPath = ""
	}

	writeJSON(w, http.StatusOK, FSListResponse{
		CurrentPath: targetPath,
		ParentPath:  parentPath,
		Roots:       enumerateFSRoots(),
		Entries:     entries,
	})
}

func (s *Server) handlePickFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req PickFolderRequest
	if r.Body != nil {
		dec := json.NewDecoder(io.LimitReader(r.Body, 1<<16))
		if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid JSON payload")
			return
		}
	}

	initialDir := strings.TrimSpace(req.InitialDir)
	if initialDir == "" {
		snap := s.snapshot.Load()
		if snap != nil && snap.Tree != nil {
			initialDir = snap.Tree.RootPath
		} else if cwd, err := os.Getwd(); err == nil {
			initialDir = cwd
		}
	}

	chosen, canceled, err := s.pickFolder(initialDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if canceled || strings.TrimSpace(chosen) == "" {
		writeJSON(w, http.StatusOK, PickFolderResponse{
			Ok:       true,
			Canceled: true,
			Path:     "",
		})
		return
	}

	writeJSON(w, http.StatusOK, PickFolderResponse{
		Ok:       true,
		Canceled: false,
		Path:     chosen,
	})
}

func (s *Server) pickFolder(initialDir string) (string, bool, error) {
	if s.FolderPickerFunc != nil {
		return s.FolderPickerFunc(initialDir)
	}
	return defaultPickFolder(initialDir)
}

func enumerateFSRoots() []string {
	roots := make([]string, 0, 8)
	seen := make(map[string]struct{}, 8)
	addRoot := func(p string) {
		if p == "" {
			return
		}
		clean := filepath.Clean(p)
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		roots = append(roots, clean)
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" && dirExists(home) {
		addRoot(home)
	}

	if runtime.GOOS == "windows" {
		for letter := byte('A'); letter <= byte('Z'); letter++ {
			drive := string([]byte{letter, ':', '\\'})
			if info, err := os.Stat(drive); err == nil && info.IsDir() {
				addRoot(drive)
			}
		}
	} else {
		addRoot("/")
	}

	return roots
}

func defaultPickFolder(initialDir string) (string, bool, error) {
	switch runtime.GOOS {
	case "windows":
		return pickFolderWindows(initialDir)
	case "darwin":
		return pickFolderDarwin(initialDir)
	default:
		return pickFolderLinux(initialDir)
	}
}

func escapePSLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func pickFolderWindows(initialDir string) (string, bool, error) {
	var script strings.Builder
	script.WriteString("[void][System.Reflection.Assembly]::LoadWithPartialName('System.Windows.Forms'); ")
	script.WriteString("$owner = New-Object System.Windows.Forms.Form -Property @{TopMost=$true}; ")
	script.WriteString("$dlg = New-Object System.Windows.Forms.FolderBrowserDialog; ")
	script.WriteString("$dlg.Description = 'Select workspace folder'; ")
	if trimmed := strings.TrimSpace(initialDir); trimmed != "" {
		escaped := escapePSLiteral(filepath.Clean(trimmed))
		script.WriteString("if (Test-Path -LiteralPath '" + escaped + "') { $dlg.SelectedPath = '" + escaped + "' }; ")
	}
	script.WriteString("$res = $dlg.ShowDialog($owner); ")
	script.WriteString("$owner.Dispose(); ")
	script.WriteString("if ($res -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $dlg.SelectedPath }")

	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-Command", script.String())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", false, fmt.Errorf("folder picker failed: %s", msg)
		}
		return "", false, fmt.Errorf("folder picker failed: %w", err)
	}
	chosen := strings.TrimSpace(stdout.String())
	if chosen == "" {
		return "", true, nil
	}
	return chosen, false, nil
}

func pickFolderDarwin(initialDir string) (string, bool, error) {
	script := `POSIX path of (choose folder with prompt "Select workspace folder")`
	if trimmed := strings.TrimSpace(initialDir); trimmed != "" {
		escaped := strings.ReplaceAll(filepath.Clean(trimmed), `"`, `\"`)
		script = fmt.Sprintf(`POSIX path of (choose folder with prompt "Select workspace folder" default location POSIX file "%s")`, escaped)
	}
	cmd := exec.Command("osascript", "-e", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errText := strings.ToLower(stderr.String())
		if strings.Contains(errText, "-128") || strings.Contains(errText, "cancel") {
			return "", true, nil
		}
		return "", false, fmt.Errorf("folder picker failed: %w", err)
	}
	chosen := strings.TrimSpace(stdout.String())
	if chosen == "" {
		return "", true, nil
	}
	return chosen, false, nil
}

func pickFolderLinux(initialDir string) (string, bool, error) {
	if zenityPath, err := exec.LookPath("zenity"); err == nil {
		args := []string{"--file-selection", "--directory", "--title=Select workspace folder"}
		if trimmed := strings.TrimSpace(initialDir); trimmed != "" {
			args = append(args, "--filename="+filepath.Clean(trimmed)+"/")
		}
		cmd := exec.Command(zenityPath, args...)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if runErr := cmd.Run(); runErr != nil {
			var exitErr *exec.ExitError
			if errors.As(runErr, &exitErr) && exitErr.ExitCode() == 1 {
				return "", true, nil
			}
			return "", false, fmt.Errorf("zenity failed: %w", runErr)
		}
		chosen := strings.TrimSpace(stdout.String())
		if chosen == "" {
			return "", true, nil
		}
		return chosen, false, nil
	}
	if kdialogPath, err := exec.LookPath("kdialog"); err == nil {
		args := []string{"--getexistingdirectory"}
		if trimmed := strings.TrimSpace(initialDir); trimmed != "" {
			args = append(args, filepath.Clean(trimmed))
		} else {
			args = append(args, ".")
		}
		cmd := exec.Command(kdialogPath, args...)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if runErr := cmd.Run(); runErr != nil {
			var exitErr *exec.ExitError
			if errors.As(runErr, &exitErr) && exitErr.ExitCode() == 1 {
				return "", true, nil
			}
			return "", false, fmt.Errorf("kdialog failed: %w", runErr)
		}
		chosen := strings.TrimSpace(stdout.String())
		if chosen == "" {
			return "", true, nil
		}
		return chosen, false, nil
	}
	return "", false, errors.New("no graphical folder picker available (zenity or kdialog required)")
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "endpoint not found")
		return
	}
	webDir := s.resolveActiveWebDir()
	if webDir == "" {
		writeError(w, http.StatusNotFound, "web assets directory not found")
		return
	}

	reqPath := r.URL.Path
	if strings.ContainsRune(reqPath, 0) || strings.Contains(reqPath, "\\") {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	if reqPath == "" || reqPath == "/" {
		reqPath = "/index.html"
	}
	if hasTraversalSegment(reqPath) {
		writeError(w, http.StatusForbidden, "path traversal not allowed")
		return
	}
	cleanedURL := path.Clean("/" + reqPath)
	rel := strings.TrimPrefix(cleanedURL, "/")
	if rel == "" || rel == "." {
		rel = "index.html"
	}

	absWebDir, err := filepath.Abs(webDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid web directory")
		return
	}
	target := filepath.Join(absWebDir, filepath.FromSlash(rel))
	relCheck, err := filepath.Rel(absWebDir, target)
	if err != nil || isEscapingRel(relCheck) {
		writeError(w, http.StatusForbidden, "path outside web directory")
		return
	}

	realTarget, statusCode, errMsg := resolveSafeFilePath(absWebDir, target)
	if errMsg != "" {
		writeError(w, statusCode, errMsg)
		return
	}

	f, err := os.Open(realTarget)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "asset not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to open asset")
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		writeError(w, http.StatusNotFound, "asset not found")
		return
	}

	contentType := detectAssetContentType(realTarget)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, filepath.Base(realTarget), stat.ModTime(), f)
}

func (s *Server) resolveActiveWebDir() string {
	if s.WebDir != "" && dirExists(s.WebDir) {
		return s.WebDir
	}
	return resolveWebDir(s.WebDir)
}

func (snap *Snapshot) getOrComputeLayout(preset string) *treemap.LayoutResult {
	snap.mu.RLock()
	layout, ok := snap.Layouts[preset]
	snap.mu.RUnlock()
	if ok && layout != nil {
		return layout
	}

	snap.mu.Lock()
	defer snap.mu.Unlock()
	if existing, exists := snap.Layouts[preset]; exists && existing != nil {
		return existing
	}
	computed := treemap.ComputeLayout(snap.Tree, preset)
	if snap.Layouts == nil {
		snap.Layouts = make(map[string]*treemap.LayoutResult)
	}
	snap.Layouts[preset] = computed
	return computed
}

func normalizePreset(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "bytes":
		return "bytes"
	case "symbols":
		return "symbols"
	default:
		return "sloc"
	}
}

func statsFromTree(tree *indexer.TreeResult) WorkspaceStats {
	if tree == nil {
		return WorkspaceStats{}
	}
	return WorkspaceStats{
		TotalFiles: tree.TotalFiles,
		TotalDirs:  tree.TotalDirs,
		TotalLines: tree.TotalLines,
		TotalSloc:  tree.TotalSLOC,
		TotalBytes: tree.TotalBytes,
	}
}

func fileResponseFromNode(node *indexer.Node) FileResponse {
	return FileResponse{
		Path:            node.RelPath,
		Name:            node.Name,
		Language:        node.Stats.Language,
		ByteSize:        node.Stats.ByteSize,
		LineCount:       node.Stats.LineCount,
		SourceLineCount: node.Stats.SourceLineCount,
		Lines:           nonNilStrings(node.Lines),
		Symbols:         nonNilSymbols(node.Analysis.Symbols),
		Imports:         nonNilImports(node.Analysis.Imports),
		TopCalls:        nonNilCalls(node.Analysis.TopCalls),
	}
}

func validateRelativePath(rawPath string) (string, int, string) {
	if strings.TrimSpace(rawPath) == "" {
		return "", http.StatusBadRequest, "missing path parameter"
	}
	if strings.ContainsRune(rawPath, 0) {
		return "", http.StatusBadRequest, "invalid path parameter"
	}
	if strings.HasPrefix(rawPath, "/") || strings.HasPrefix(rawPath, "\\") || filepath.IsAbs(rawPath) || filepath.VolumeName(rawPath) != "" {
		return "", http.StatusBadRequest, "path must be relative"
	}
	slashInput := strings.ReplaceAll(rawPath, "\\", "/")
	if hasTraversalSegment(slashInput) {
		return "", http.StatusForbidden, "path traversal not allowed"
	}
	cleaned := filepath.Clean(filepath.FromSlash(slashInput))
	if cleaned == "." || cleaned == ".." || filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", http.StatusForbidden, "path traversal not allowed"
	}
	return cleaned, 0, ""
}

func hasTraversalSegment(slashPath string) bool {
	parts := strings.Split(slashPath, "/")
	for _, p := range parts {
		if p == ".." {
			return true
		}
	}
	return false
}

func isEscapingRel(rel string) bool {
	if rel == ".." || filepath.IsAbs(rel) {
		return true
	}
	return strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func resolveSafeFilePath(rootAbs string, joined string) (string, int, string) {
	rootEval, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		rootEval = rootAbs
	}

	linfo, lerr := os.Lstat(joined)
	if lerr != nil {
		if errors.Is(lerr, os.ErrNotExist) {
			if ancestorEscapesRoot(rootAbs, rootEval, filepath.Dir(joined)) {
				return "", http.StatusForbidden, "symlink escape outside workspace root"
			}
			return "", http.StatusNotFound, "file not found"
		}
		return "", http.StatusForbidden, "path not accessible"
	}

	realPath, err := filepath.EvalSymlinks(joined)
	if err != nil {
		if (linfo.Mode() & os.ModeSymlink) != 0 {
			return "", http.StatusForbidden, "unresolvable symlink outside workspace root"
		}
		if errors.Is(err, os.ErrNotExist) {
			return "", http.StatusNotFound, "file not found"
		}
		return "", http.StatusForbidden, "path not accessible"
	}

	relReal, err := filepath.Rel(rootEval, realPath)
	if err != nil || isEscapingRel(relReal) {
		return "", http.StatusForbidden, "symlink escape outside workspace root"
	}

	statInfo, err := os.Stat(realPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", http.StatusNotFound, "file not found"
		}
		return "", http.StatusForbidden, "file not accessible"
	}
	if statInfo.IsDir() {
		return "", http.StatusBadRequest, "path is a directory"
	}
	return realPath, 0, ""
}

func ancestorEscapesRoot(rootAbs string, rootEval string, dir string) bool {
	cur := dir
	for {
		if len(cur) < len(rootAbs) {
			return false
		}
		if _, err := os.Lstat(cur); err == nil {
			evalDir, evalErr := filepath.EvalSymlinks(cur)
			if evalErr != nil {
				return true
			}
			rel, relErr := filepath.Rel(rootEval, evalDir)
			return relErr != nil || isEscapingRel(rel)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return false
		}
		cur = parent
	}
}

func resolveWebDir(override string) string {
	if override != "" && dirExists(override) {
		return filepath.Clean(override)
	}

	var starts []string
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		starts = append(starts, cwd)
	}
	if exePath, err := os.Executable(); err == nil && exePath != "" {
		starts = append(starts, filepath.Dir(exePath))
	}

	for _, start := range starts {
		direct := filepath.Join(start, "web")
		if hasIndexHTML(direct) {
			return direct
		}
	}

	for _, start := range starts {
		cur := start
		for {
			cand := filepath.Join(cur, "web")
			if hasIndexHTML(cand) {
				return cand
			}
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
		}
	}

	for _, start := range starts {
		direct := filepath.Join(start, "web")
		if dirExists(direct) {
			return direct
		}
	}
	return ""
}

func hasIndexHTML(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "index.html"))
	return err == nil && !info.IsDir()
}

func dirExists(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

func detectAssetContentType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js", ".mjs":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".ico":
		return "image/x-icon"
	case ".wasm":
		return "application/wasm"
	case ".txt", ".md":
		return "text/plain; charset=utf-8"
	default:
		return mime.TypeByExtension(ext)
	}
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func nonNilSymbols(v []analyzer.Symbol) []analyzer.Symbol {
	if v == nil {
		return []analyzer.Symbol{}
	}
	return v
}

func nonNilImports(v []analyzer.ImportRef) []analyzer.ImportRef {
	if v == nil {
		return []analyzer.ImportRef{}
	}
	return v
}

func nonNilCalls(v []analyzer.CallMetric) []analyzer.CallMetric {
	if v == nil {
		return []analyzer.CallMetric{}
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, `{"error":"internal serialization error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
