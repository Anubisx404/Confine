package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"confine/internal/search"
)

func createSampleWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	sub := filepath.Join(root, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}

	mainGo := "package main\n\nimport \"fmt\"\n\nfunc Greet(name string) string {\n\treturn fmt.Sprintf(\"Hello, %s\", name)\n}\n\nfunc main() {\n\tprintln(Greet(\"World\"))\n}\n"
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(mainGo), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	helperGo := "package pkg\n\nfunc Add(a int, b int) int {\n\treturn a + b\n}\n"
	if err := os.WriteFile(filepath.Join(sub, "helper.go"), []byte(helperGo), 0o644); err != nil {
		t.Fatalf("write helper.go: %v", err)
	}

	readme := "# Sample Project\n\nThis project demonstrates Greet and Add.\n"
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}

	return root
}

func createTempWebDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><html><body>Confine</body></html>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "styles.css"), []byte("body { margin: 0; }"), 0o644); err != nil {
		t.Fatalf("write styles.css: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("window.ConfineReady = true;"), 0o644); err != nil {
		t.Fatalf("write app.js: %v", err)
	}
	return dir
}

func TestStaticAssetsAndRoot(t *testing.T) {
	ws := createSampleWorkspace(t)
	srv, err := NewServer(ws, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	srv.WebDir = createTempWebDir(t)
	h := srv.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Confine") {
		t.Fatalf("GET / body missing Confine: %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("GET / Content-Type = %q, want text/html", ct)
	}

	recCSS := httptest.NewRecorder()
	h.ServeHTTP(recCSS, httptest.NewRequest(http.MethodGet, "/styles.css", nil))
	if recCSS.Code != http.StatusOK {
		t.Fatalf("GET /styles.css status = %d, want 200", recCSS.Code)
	}
	if ct := recCSS.Header().Get("Content-Type"); !strings.Contains(ct, "text/css") {
		t.Fatalf("GET /styles.css Content-Type = %q, want text/css", ct)
	}

	recJS := httptest.NewRecorder()
	h.ServeHTTP(recJS, httptest.NewRequest(http.MethodGet, "/app.js", nil))
	if recJS.Code != http.StatusOK {
		t.Fatalf("GET /app.js status = %d, want 200", recJS.Code)
	}
	if ct := recJS.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("GET /app.js Content-Type = %q, want javascript", ct)
	}

	recMissing := httptest.NewRecorder()
	h.ServeHTTP(recMissing, httptest.NewRequest(http.MethodGet, "/does-not-exist.js", nil))
	if recMissing.Code != http.StatusNotFound {
		t.Fatalf("GET missing asset status = %d, want 404", recMissing.Code)
	}

	recTrav := httptest.NewRecorder()
	h.ServeHTTP(recTrav, httptest.NewRequest(http.MethodGet, "/../go.mod", nil))
	if recTrav.Code == http.StatusOK {
		t.Fatalf("static traversal unexpectedly returned 200")
	}
}

func TestTreePresets(t *testing.T) {
	ws := createSampleWorkspace(t)
	srv, err := NewServer(ws, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	h := srv.Handler()

	presets := []string{"", "sloc", "bytes", "symbols", "sloc"}
	for _, p := range presets {
		target := "/api/tree"
		if p != "" {
			target += "?preset=" + p
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, body = %s", target, rec.Code, rec.Body.String())
		}
		var resp TreeResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode TreeResponse for %s: %v", target, err)
		}
		if len(resp.Nodes) == 0 {
			t.Fatalf("expected non-empty Nodes for %s", target)
		}
		if resp.Stats.TotalFiles < 3 {
			t.Fatalf("expected >= 3 files, got %d", resp.Stats.TotalFiles)
		}
		if resp.CanvasRect.W <= 0 || resp.CanvasRect.H <= 0 {
			t.Fatalf("expected positive CanvasRect, got %+v", resp.CanvasRect)
		}
		expectedPreset := p
		if expectedPreset == "" {
			expectedPreset = "sloc"
		}
		if resp.Preset != expectedPreset {
			t.Fatalf("Preset = %q, want %q", resp.Preset, expectedPreset)
		}
	}
}

func TestFileEndpointAndTraversal(t *testing.T) {
	base := t.TempDir()
	outsideFile := filepath.Join(base, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("secret-outside"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	ws := createSampleWorkspace(t)
	srv, err := NewServer(ws, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	h := srv.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/file?path=main.go", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET main.go status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var fileResp FileResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &fileResp); err != nil {
		t.Fatalf("decode FileResponse: %v", err)
	}
	if fileResp.Path != "main.go" || len(fileResp.Lines) == 0 || fileResp.Language != "Go" {
		t.Fatalf("unexpected FileResponse: %+v", fileResp)
	}

	fallbackPath := filepath.Join(ws, "added.txt")
	if err := os.WriteFile(fallbackPath, []byte("fallback line\n"), 0o644); err != nil {
		t.Fatalf("write fallback file: %v", err)
	}
	recFallback := httptest.NewRecorder()
	h.ServeHTTP(recFallback, httptest.NewRequest(http.MethodGet, "/api/file?path=added.txt", nil))
	if recFallback.Code != http.StatusOK {
		t.Fatalf("GET fallback file status = %d, body = %s", recFallback.Code, recFallback.Body.String())
	}

	badRequests := []struct {
		query string
		codes []int
	}{
		{"", []int{http.StatusBadRequest}},
		{"?path=", []int{http.StatusBadRequest}},
		{"?path=/etc/passwd", []int{http.StatusBadRequest, http.StatusForbidden}},
		{"?path=" + outsideFile, []int{http.StatusBadRequest, http.StatusForbidden}},
		{"?path=../outside.txt", []int{http.StatusForbidden}},
		{"?path=pkg/../../outside.txt", []int{http.StatusForbidden}},
		{"?path=..%5Coutside.txt", []int{http.StatusForbidden}},
		{"?path=pkg", []int{http.StatusBadRequest}},
		{"?path=missing.go", []int{http.StatusNotFound}},
	}

	for _, tc := range badRequests {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/file"+tc.query, nil))
		matched := false
		for _, want := range tc.codes {
			if r.Code == want {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("GET /api/file%s status = %d, want one of %v, body=%s", tc.query, r.Code, tc.codes, r.Body.String())
		}
	}
}

func TestFileSymlinkEscape(t *testing.T) {
	parent := t.TempDir()
	outsideDir := filepath.Join(parent, "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("top-secret"), 0o644); err != nil {
		t.Fatalf("write secret.txt: %v", err)
	}

	ws := createSampleWorkspace(t)
	linkDir := filepath.Join(ws, "escape_dir")
	if err := os.Symlink(outsideDir, linkDir); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatalf("os.Symlink failed: %v", err)
		}
		cmd := exec.Command("cmd", "/c", "mklink", "/J", linkDir, outsideDir)
		out, jErr := cmd.CombinedOutput()
		if jErr != nil {
			t.Fatalf("mklink /J failed: %v (%s)", jErr, string(out))
		}
	}
	t.Cleanup(func() {
		_ = os.Remove(linkDir)
	})

	srv, err := NewServer(ws, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	h := srv.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/file?path=escape_dir/secret.txt", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("symlink escape status = %d, want 403, body = %s", rec.Code, rec.Body.String())
	}

	recMissing := httptest.NewRecorder()
	h.ServeHTTP(recMissing, httptest.NewRequest(http.MethodGet, "/api/file?path=escape_dir/missing.txt", nil))
	if recMissing.Code != http.StatusForbidden {
		t.Fatalf("symlink escape missing file status = %d, want 403, body = %s", recMissing.Code, recMissing.Body.String())
	}
}

func TestSearchAndMarkdowns(t *testing.T) {
	ws := createSampleWorkspace(t)
	srv, err := NewServer(ws, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	h := srv.Handler()

	recSearch := httptest.NewRecorder()
	h.ServeHTTP(recSearch, httptest.NewRequest(http.MethodGet, "/api/search?q=Greet&limit=50", nil))
	if recSearch.Code != http.StatusOK {
		t.Fatalf("GET /api/search status = %d, body = %s", recSearch.Code, recSearch.Body.String())
	}
	var searchBody struct {
		Query        string         `json:"query"`
		TotalMatches int            `json:"totalMatches"`
		ElapsedMs    float64        `json:"elapsedMs"`
		FileHits     map[string]int `json:"fileHits"`
		Matches      []any          `json:"matches"`
	}
	if err := json.Unmarshal(recSearch.Body.Bytes(), &searchBody); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	if searchBody.TotalMatches == 0 || len(searchBody.Matches) == 0 {
		t.Fatalf("expected matches for Greet, got %+v", searchBody)
	}

	recBadLimit := httptest.NewRecorder()
	h.ServeHTTP(recBadLimit, httptest.NewRequest(http.MethodGet, "/api/search?q=Greet&limit=abc", nil))
	if recBadLimit.Code != http.StatusBadRequest {
		t.Fatalf("bad limit status = %d, want 400", recBadLimit.Code)
	}

	recMD := httptest.NewRecorder()
	h.ServeHTTP(recMD, httptest.NewRequest(http.MethodGet, "/api/markdowns", nil))
	if recMD.Code != http.StatusOK {
		t.Fatalf("GET /api/markdowns status = %d, body = %s", recMD.Code, recMD.Body.String())
	}
	var mdResp MarkdownsResponse
	if err := json.Unmarshal(recMD.Body.Bytes(), &mdResp); err != nil {
		t.Fatalf("decode MarkdownsResponse: %v", err)
	}
	if mdResp.Count != 1 || len(mdResp.Items) != 1 || mdResp.Items[0].RelPath != "README.md" {
		t.Fatalf("unexpected MarkdownsResponse: %+v", mdResp)
	}
}

func TestOpenWorkspace(t *testing.T) {
	ws1 := createSampleWorkspace(t)
	ws2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws2, "second.go"), []byte("package second\nfunc Run() {}\n"), 0o644); err != nil {
		t.Fatalf("write second.go: %v", err)
	}

	srv, err := NewServer(ws1, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	h := srv.Handler()

	payload, _ := json.Marshal(OpenRequest{Path: ws2})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/open", bytes.NewReader(payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/open status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var openResp OpenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &openResp); err != nil {
		t.Fatalf("decode OpenResponse: %v", err)
	}
	if !openResp.Ok || openResp.Stats.TotalFiles != 1 {
		t.Fatalf("unexpected OpenResponse: %+v", openResp)
	}

	recSearch := httptest.NewRecorder()
	h.ServeHTTP(recSearch, httptest.NewRequest(http.MethodGet, "/api/search?q=Run", nil))
	if recSearch.Code != http.StatusOK || !strings.Contains(recSearch.Body.String(), "second.go") {
		t.Fatalf("expected second.go in updated search index, got %d %s", recSearch.Code, recSearch.Body.String())
	}

	recInvalidJSON := httptest.NewRecorder()
	h.ServeHTTP(recInvalidJSON, httptest.NewRequest(http.MethodPost, "/api/open", strings.NewReader("{bad")))
	if recInvalidJSON.Code != http.StatusBadRequest {
		t.Fatalf("invalid JSON status = %d, want 400", recInvalidJSON.Code)
	}

	missingPayload, _ := json.Marshal(OpenRequest{Path: filepath.Join(ws2, "nonexistent-dir")})
	recMissingDir := httptest.NewRecorder()
	h.ServeHTTP(recMissingDir, httptest.NewRequest(http.MethodPost, "/api/open", bytes.NewReader(missingPayload)))
	if recMissingDir.Code != http.StatusBadRequest {
		t.Fatalf("missing dir status = %d, want 400", recMissingDir.Code)
	}
}

func TestConcurrentAccessRace(t *testing.T) {
	ws1 := createSampleWorkspace(t)
	ws2 := createSampleWorkspace(t)
	if err := os.WriteFile(filepath.Join(ws2, "extra.go"), []byte("package main\nfunc Extra() {}\n"), 0o644); err != nil {
		t.Fatalf("write extra.go: %v", err)
	}

	srv, err := NewServer(ws1, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	h := srv.Handler()

	var wg sync.WaitGroup
	errCh := make(chan string, 64)

	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			presets := []string{"sloc", "bytes", "symbols"}
			for j := 0; j < 25; j++ {
				p := presets[(idx+j)%len(presets)]
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tree?preset="+p, nil))
				if rec.Code != http.StatusOK {
					errCh <- "tree non-200"
					return
				}
			}
		}(i)
	}

	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=Greet&limit=20", nil))
				if rec.Code != http.StatusOK {
					errCh <- "search non-200"
					return
				}
			}
		}()
	}

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			targets := []string{ws1, ws2}
			for j := 0; j < 8; j++ {
				target := targets[(idx+j)%2]
				body, _ := json.Marshal(OpenRequest{Path: target})
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/open", bytes.NewReader(body)))
				if rec.Code != http.StatusOK {
					errCh <- "open non-200"
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)
	for msg := range errCh {
		t.Fatalf("concurrent error: %s", msg)
	}
}

func TestExcludesEndpointAndInitialOptions(t *testing.T) {
	ws := createSampleWorkspace(t)
	srv, err := NewServerWithOptions(ws, "127.0.0.1:0", []string{"*.md"})
	if err != nil {
		t.Fatalf("NewServerWithOptions failed: %v", err)
	}
	h := srv.Handler()

	recTreeInit := httptest.NewRecorder()
	h.ServeHTTP(recTreeInit, httptest.NewRequest(http.MethodGet, "/api/tree", nil))
	if recTreeInit.Code != http.StatusOK {
		t.Fatalf("GET /api/tree status = %d", recTreeInit.Code)
	}
	var treeInit TreeResponse
	if err := json.Unmarshal(recTreeInit.Body.Bytes(), &treeInit); err != nil {
		t.Fatalf("decode treeInit: %v", err)
	}
	if !reflect.DeepEqual(treeInit.Excludes, []string{"*.md"}) {
		t.Fatalf("initial excludes = %v, want [*.md]", treeInit.Excludes)
	}
	if treeInit.Stats.TotalFiles != 2 {
		t.Fatalf("expected 2 files when *.md excluded, got %d", treeInit.Stats.TotalFiles)
	}

	recMDInit := httptest.NewRecorder()
	h.ServeHTTP(recMDInit, httptest.NewRequest(http.MethodGet, "/api/markdowns", nil))
	var mdInit MarkdownsResponse
	if err := json.Unmarshal(recMDInit.Body.Bytes(), &mdInit); err != nil {
		t.Fatalf("decode mdInit: %v", err)
	}
	if mdInit.Count != 0 {
		t.Fatalf("expected 0 markdowns when *.md excluded, got %d", mdInit.Count)
	}

	recMethod := httptest.NewRecorder()
	h.ServeHTTP(recMethod, httptest.NewRequest(http.MethodGet, "/api/excludes", nil))
	if recMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/excludes status = %d, want 405", recMethod.Code)
	}

	recBadJSON := httptest.NewRecorder()
	h.ServeHTTP(recBadJSON, httptest.NewRequest(http.MethodPost, "/api/excludes", strings.NewReader("{bad")))
	if recBadJSON.Code != http.StatusBadRequest {
		t.Fatalf("bad JSON status = %d, want 400", recBadJSON.Code)
	}

	addBody, _ := json.Marshal(ExcludesRequest{Add: "pkg/"})
	recAdd := httptest.NewRecorder()
	h.ServeHTTP(recAdd, httptest.NewRequest(http.MethodPost, "/api/excludes", bytes.NewReader(addBody)))
	if recAdd.Code != http.StatusOK {
		t.Fatalf("POST /api/excludes add status = %d, body = %s", recAdd.Code, recAdd.Body.String())
	}
	var addResp OpenResponse
	if err := json.Unmarshal(recAdd.Body.Bytes(), &addResp); err != nil {
		t.Fatalf("decode addResp: %v", err)
	}
	if !reflect.DeepEqual(addResp.Excludes, []string{"*.md", "pkg/"}) {
		t.Fatalf("after add excludes = %v, want [*.md pkg/]", addResp.Excludes)
	}
	if addResp.Stats.TotalFiles != 1 {
		t.Fatalf("expected 1 file after excluding *.md and pkg/, got %d", addResp.Stats.TotalFiles)
	}

	remBody, _ := json.Marshal(ExcludesRequest{Remove: "*.md"})
	recRem := httptest.NewRecorder()
	h.ServeHTTP(recRem, httptest.NewRequest(http.MethodPost, "/api/excludes", bytes.NewReader(remBody)))
	if recRem.Code != http.StatusOK {
		t.Fatalf("POST /api/excludes remove status = %d", recRem.Code)
	}
	var remResp OpenResponse
	if err := json.Unmarshal(recRem.Body.Bytes(), &remResp); err != nil {
		t.Fatalf("decode remResp: %v", err)
	}
	if !reflect.DeepEqual(remResp.Excludes, []string{"pkg/"}) {
		t.Fatalf("after remove excludes = %v, want [pkg/]", remResp.Excludes)
	}
	if remResp.Stats.TotalFiles != 2 {
		t.Fatalf("expected 2 files after removing *.md, got %d", remResp.Stats.TotalFiles)
	}

	replaceBody, _ := json.Marshal(ExcludesRequest{Excludes: []string{"main.go", "pkg/helper.go"}})
	recRep := httptest.NewRecorder()
	h.ServeHTTP(recRep, httptest.NewRequest(http.MethodPost, "/api/excludes", bytes.NewReader(replaceBody)))
	if recRep.Code != http.StatusOK {
		t.Fatalf("POST /api/excludes replace status = %d", recRep.Code)
	}
	var repResp OpenResponse
	if err := json.Unmarshal(recRep.Body.Bytes(), &repResp); err != nil {
		t.Fatalf("decode repResp: %v", err)
	}
	if !reflect.DeepEqual(repResp.Excludes, []string{"main.go", "pkg/helper.go"}) {
		t.Fatalf("after replace excludes = %v", repResp.Excludes)
	}
	if repResp.Stats.TotalFiles != 1 {
		t.Fatalf("expected 1 file remaining (README.md), got %d", repResp.Stats.TotalFiles)
	}

	recSearch := httptest.NewRecorder()
	h.ServeHTTP(recSearch, httptest.NewRequest(http.MethodGet, "/api/search?q=Greet", nil))
	if recSearch.Code != http.StatusOK {
		t.Fatalf("GET /api/search status = %d", recSearch.Code)
	}
	if strings.Contains(recSearch.Body.String(), "main.go") {
		t.Fatalf("excluded main.go should not appear in search results: %s", recSearch.Body.String())
	}

	clearBody, _ := json.Marshal(ExcludesRequest{Clear: true})
	recClear := httptest.NewRecorder()
	h.ServeHTTP(recClear, httptest.NewRequest(http.MethodPost, "/api/excludes", bytes.NewReader(clearBody)))
	if recClear.Code != http.StatusOK {
		t.Fatalf("POST /api/excludes clear status = %d", recClear.Code)
	}
	var clearResp OpenResponse
	if err := json.Unmarshal(recClear.Body.Bytes(), &clearResp); err != nil {
		t.Fatalf("decode clearResp: %v", err)
	}
	if len(clearResp.Excludes) != 0 {
		t.Fatalf("expected empty excludes after clear, got %v", clearResp.Excludes)
	}
	if clearResp.Stats.TotalFiles != 3 {
		t.Fatalf("expected all 3 files restored after clear, got %d", clearResp.Stats.TotalFiles)
	}
}

func TestFSListEndpoint(t *testing.T) {
	ws := createSampleWorkspace(t)
	srv, err := NewServer(ws, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	h := srv.Handler()

	recMethod := httptest.NewRecorder()
	h.ServeHTTP(recMethod, httptest.NewRequest(http.MethodPost, "/api/fs", nil))
	if recMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/fs status = %d, want 405", recMethod.Code)
	}

	recDefault := httptest.NewRecorder()
	h.ServeHTTP(recDefault, httptest.NewRequest(http.MethodGet, "/api/fs", nil))
	if recDefault.Code != http.StatusOK {
		t.Fatalf("GET /api/fs status = %d, body = %s", recDefault.Code, recDefault.Body.String())
	}
	var listResp FSListResponse
	if err := json.Unmarshal(recDefault.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode FSListResponse: %v", err)
	}
	if listResp.CurrentPath != ws {
		t.Fatalf("CurrentPath = %q, want %q", listResp.CurrentPath, ws)
	}
	if listResp.ParentPath == "" || listResp.ParentPath != filepath.Dir(ws) {
		t.Fatalf("ParentPath = %q, want %q", listResp.ParentPath, filepath.Dir(ws))
	}
	if len(listResp.Roots) == 0 {
		t.Fatalf("expected non-empty Roots")
	}
	if len(listResp.Entries) != 3 {
		t.Fatalf("expected 3 entries (pkg, main.go, README.md), got %d: %+v", len(listResp.Entries), listResp.Entries)
	}
	if !listResp.Entries[0].IsDir || listResp.Entries[0].Name != "pkg" || listResp.Entries[0].RelPath != "pkg" {
		t.Fatalf("expected first entry to be dir pkg with relPath pkg, got %+v", listResp.Entries[0])
	}
	if listResp.Entries[1].Name != "main.go" || listResp.Entries[2].Name != "README.md" {
		t.Fatalf("expected case-insensitive file sort (main.go then README.md), got %+v", listResp.Entries[1:])
	}

	recSub := httptest.NewRecorder()
	h.ServeHTTP(recSub, httptest.NewRequest(http.MethodGet, "/api/fs?path="+filepath.Join(ws, "pkg"), nil))
	if recSub.Code != http.StatusOK {
		t.Fatalf("GET /api/fs?path=pkg status = %d", recSub.Code)
	}
	var subResp FSListResponse
	if err := json.Unmarshal(recSub.Body.Bytes(), &subResp); err != nil {
		t.Fatalf("decode subResp: %v", err)
	}
	if len(subResp.Entries) != 1 || subResp.Entries[0].RelPath != "pkg/helper.go" {
		t.Fatalf("expected pkg/helper.go entry, got %+v", subResp.Entries)
	}

	recParent := httptest.NewRecorder()
	h.ServeHTTP(recParent, httptest.NewRequest(http.MethodGet, "/api/fs?path="+filepath.Dir(ws), nil))
	if recParent.Code != http.StatusOK {
		t.Fatalf("GET /api/fs parent status = %d", recParent.Code)
	}
	var parentResp FSListResponse
	if err := json.Unmarshal(recParent.Body.Bytes(), &parentResp); err != nil {
		t.Fatalf("decode parentResp: %v", err)
	}
	for _, entry := range parentResp.Entries {
		if entry.RelPath != "" {
			t.Fatalf("expected empty RelPath outside workspace root, got %+v", entry)
		}
	}

	recNotDir := httptest.NewRecorder()
	h.ServeHTTP(recNotDir, httptest.NewRequest(http.MethodGet, "/api/fs?path="+filepath.Join(ws, "main.go"), nil))
	if recNotDir.Code != http.StatusBadRequest {
		t.Fatalf("file path /api/fs status = %d, want 400", recNotDir.Code)
	}
}

func TestPickFolderEndpoint(t *testing.T) {
	ws := createSampleWorkspace(t)
	srv, err := NewServer(ws, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	h := srv.Handler()

	recMethod := httptest.NewRecorder()
	h.ServeHTTP(recMethod, httptest.NewRequest(http.MethodGet, "/api/pick-folder", nil))
	if recMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/pick-folder status = %d, want 405", recMethod.Code)
	}

	var seenInitial string
	srv.FolderPickerFunc = func(initialDir string) (string, bool, error) {
		seenInitial = initialDir
		return filepath.Join(ws, "pkg"), false, nil
	}
	recChosen := httptest.NewRecorder()
	h.ServeHTTP(recChosen, httptest.NewRequest(http.MethodPost, "/api/pick-folder", nil))
	if recChosen.Code != http.StatusOK {
		t.Fatalf("POST /api/pick-folder status = %d, body = %s", recChosen.Code, recChosen.Body.String())
	}
	if seenInitial != ws {
		t.Fatalf("expected initialDir=%q, got %q", ws, seenInitial)
	}
	var chosenResp PickFolderResponse
	if err := json.Unmarshal(recChosen.Body.Bytes(), &chosenResp); err != nil {
		t.Fatalf("decode chosenResp: %v", err)
	}
	if !chosenResp.Ok || chosenResp.Canceled || chosenResp.Path != filepath.Join(ws, "pkg") {
		t.Fatalf("unexpected chosenResp: %+v", chosenResp)
	}

	srv.FolderPickerFunc = func(initialDir string) (string, bool, error) {
		return "", true, nil
	}
	recCancel := httptest.NewRecorder()
	h.ServeHTTP(recCancel, httptest.NewRequest(http.MethodPost, "/api/pick-folder", nil))
	if recCancel.Code != http.StatusOK {
		t.Fatalf("POST /api/pick-folder cancel status = %d", recCancel.Code)
	}
	var cancelResp PickFolderResponse
	if err := json.Unmarshal(recCancel.Body.Bytes(), &cancelResp); err != nil {
		t.Fatalf("decode cancelResp: %v", err)
	}
	if !cancelResp.Ok || !cancelResp.Canceled || cancelResp.Path != "" {
		t.Fatalf("unexpected cancelResp: %+v", cancelResp)
	}

	srv.FolderPickerFunc = func(initialDir string) (string, bool, error) {
		return "", false, errors.New("picker unavailable")
	}
	recErr := httptest.NewRecorder()
	h.ServeHTTP(recErr, httptest.NewRequest(http.MethodPost, "/api/pick-folder", nil))
	if recErr.Code != http.StatusInternalServerError {
		t.Fatalf("POST /api/pick-folder error status = %d, want 500", recErr.Code)
	}
}

func TestMarkdownsIncludesNestedReadmesAndGitignoredPlans(t *testing.T) {
	ws := createSampleWorkspace(t)
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(ws, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	writeFile(".gitignore", "docs/\n")
	writeFile("pkg/README.md", "# Pkg Docs\nDetails.\n")
	writeFile("PLAN.md", "# Master Plan\nStep 1.\n")
	writeFile("docs/plans/git-intel.md", "# Git Intel Plan\nRoadmap.\n")

	srv, err := NewServer(ws, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	h := srv.Handler()

	recMD := httptest.NewRecorder()
	h.ServeHTTP(recMD, httptest.NewRequest(http.MethodGet, "/api/markdowns", nil))
	if recMD.Code != http.StatusOK {
		t.Fatalf("GET /api/markdowns status = %d", recMD.Code)
	}
	var resp MarkdownsResponse
	if err := json.Unmarshal(recMD.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode MarkdownsResponse: %v", err)
	}

	byPath := make(map[string]search.MarkdownEntry, len(resp.Items))
	for _, item := range resp.Items {
		byPath[item.RelPath] = item
	}
	if got, ok := byPath["README.md"]; !ok || got.Category != "readme" || !got.InMap {
		t.Fatalf("unexpected root README.md entry: %+v", got)
	}
	if got, ok := byPath["pkg/README.md"]; !ok || got.Category != "readme" || !got.InMap {
		t.Fatalf("unexpected pkg/README.md entry: %+v", got)
	}
	if got, ok := byPath["PLAN.md"]; !ok || got.Category != "plan" || !got.InMap {
		t.Fatalf("unexpected PLAN.md entry: %+v", got)
	}
	if got, ok := byPath["docs/plans/git-intel.md"]; !ok || got.Category != "plan" || got.InMap {
		t.Fatalf("expected gitignored docs/plans/git-intel.md with category=plan inMap=false, got %+v", got)
	}

	recFile := httptest.NewRecorder()
	h.ServeHTTP(recFile, httptest.NewRequest(http.MethodGet, "/api/file?path=docs/plans/git-intel.md", nil))
	if recFile.Code != http.StatusOK {
		t.Fatalf("GET /api/file for gitignored plan status = %d", recFile.Code)
	}
}

func TestMarkdownsIncludesAIAgentFiles(t *testing.T) {
	ws := createSampleWorkspace(t)
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(ws, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	writeFile(".gitignore", ".gemini/\n")
	writeFile("AGENTS.md", "# Agent Rules\n")
	writeFile(".agents/skills/review/SKILL.md", "---\nname: code-review\ndescription: x\n---\n# Review\n")
	writeFile(".agents/hooks.json", "{\"hooks\":[]}\n")
	writeFile(".gemini/settings.json", "{\"model\":\"x\"}\n")

	srv, err := NewServer(ws, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/markdowns", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/markdowns status = %d", rec.Code)
	}
	var resp MarkdownsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byPath := make(map[string]search.MarkdownEntry, len(resp.Items))
	for _, item := range resp.Items {
		byPath[item.RelPath] = item
	}

	checks := []struct {
		path  string
		kind  string
		lang  string
		inMap bool
		title string
	}{
		{"AGENTS.md", "instructions", "Markdown", true, "Agent Rules"},
		{".agents/skills/review/SKILL.md", "skill", "Markdown", true, "code-review"},
		{".agents/hooks.json", "hook", "JSON", true, "hooks.json"},
		{".gemini/settings.json", "config", "JSON", false, "settings.json"},
	}
	for _, c := range checks {
		got, ok := byPath[c.path]
		if !ok {
			t.Fatalf("missing AI entry %s in %+v", c.path, resp.Items)
		}
		if got.Category != "ai" || got.AIKind != c.kind || got.Language != c.lang || got.InMap != c.inMap || got.Title != c.title {
			t.Fatalf("unexpected AI entry for %s: %+v", c.path, got)
		}
	}
	if _, ok := byPath["main.go"]; ok {
		t.Fatalf("main.go must not appear in markdowns catalog")
	}
}
