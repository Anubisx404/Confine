package treemap

import (
	"confine/internal/analyzer"
	"confine/internal/indexer"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func makeTestFile(id uint32, parentID uint32, name string, relPath string, depth int, sloc int, bytes int64, syms int, imports int, lines []string) *indexer.Node {
	var symList []analyzer.Symbol
	for i := 0; i < syms; i++ {
		symList = append(symList, analyzer.Symbol{
			Name:    fmt.Sprintf("Sym%d", i),
			Kind:    "func",
			Line:    i + 1,
			EndLine: i + 2,
		})
	}
	return &indexer.Node{
		ID:       id,
		ParentID: parentID,
		Name:     name,
		RelPath:  relPath,
		IsDir:    false,
		Depth:    depth,
		Stats: indexer.FileStats{
			ByteSize:        bytes,
			LineCount:       len(lines),
			SourceLineCount: sloc,
			Language:        "Go",
			SymbolCount:     syms,
			ImportCount:     imports,
		},
		Analysis: analyzer.AnalysisResult{
			Symbols: symList,
		},
		Lines: lines,
	}
}

func makeTestDir(id uint32, parentID uint32, name string, relPath string, depth int, children ...*indexer.Node) *indexer.Node {
	return &indexer.Node{
		ID:       id,
		ParentID: parentID,
		Name:     name,
		RelPath:  relPath,
		IsDir:    true,
		Depth:    depth,
		Children: children,
	}
}

func buildComplexTree() *indexer.TreeResult {
	longLine := "func VeryLongFunctionSignatureWithManyParametersAndLongIdentifierNames(alpha int, beta string, gamma float64, delta bool) error {"
	lines120 := make([]string, 120)
	for i := range lines120 {
		if i%3 == 0 {
			lines120[i] = longLine
		} else if i%5 == 0 {
			lines120[i] = ""
		} else {
			lines120[i] = "    return nil"
		}
	}

	deepFileA := makeTestFile(7, 6, "alpha.go", "internal/core/sub/alpha.go", 3, 250, 8200, 12, 4, lines120)
	deepFileB := makeTestFile(8, 6, "beta.go", "internal/core/sub/beta.go", 3, 80, 2100, 3, 1, []string{"package sub", "func b() {}"})
	subDir := makeTestDir(6, 5, "sub", "internal/core/sub", 2, deepFileA, deepFileB)
	coreFile := makeTestFile(9, 5, "engine.go", "internal/core/engine.go", 2, 420, 14500, 20, 6, lines120[:40])
	coreDir := makeTestDir(5, 1, "core", "internal/core", 1, subDir, coreFile)

	tinyFile := makeTestFile(10, 1, "tiny.go", "internal/tiny.go", 2, 1, 12, 0, 0, []string{"package internal"})
	internalDir := makeTestDir(1, 0, "internal", "internal", 1, coreDir, tinyFile)

	webApp := makeTestFile(11, 2, "app.js", "web/app.js", 2, 600, 19000, 18, 2, lines120[:80])
	webStyle := makeTestFile(12, 2, "styles.css", "web/styles.css", 2, 150, 4200, 0, 0, []string{"body { margin: 0; }"})
	webDir := makeTestDir(2, 0, "web", "web", 1, webApp, webStyle)

	mainFile := makeTestFile(13, 3, "main.go", "cmd/main.go", 2, 95, 3100, 2, 3, []string{"package main", "func main() {}"})
	cmdDir := makeTestDir(3, 0, "cmd", "cmd", 1, mainFile)

	readme := makeTestFile(14, 0, "README.md", "README.md", 1, 45, 1800, 0, 0, []string{"# Project", "Details"})

	root := makeTestDir(0, 0, "myproj", "", 0, internalDir, webDir, cmdDir, readme)

	return &indexer.TreeResult{
		RootPath:    "/tmp/myproj",
		ProjectName: "myproj",
		Root:        root,
	}
}

func assertInvariants(t *testing.T, res *LayoutResult) {
	t.Helper()
	const eps = 1e-6

	if res.CanvasRect != (Rect{X: 0, Y: 0, W: 1000, H: 650}) {
		t.Fatalf("unexpected CanvasRect: %+v", res.CanvasRect)
	}
	if len(res.Nodes) == 0 {
		t.Fatalf("expected non-empty Nodes")
	}

	childrenByParent := make(map[string][]LayoutNode)
	for _, n := range res.Nodes {
		if n.Bounds.W <= 0 || n.Bounds.H <= 0 {
			t.Fatalf("node %q has non-positive Bounds: %+v", n.RelPath, n.Bounds)
		}
		if n.ContentRect.W <= 0 || n.ContentRect.H <= 0 {
			t.Fatalf("node %q has non-positive ContentRect: %+v", n.RelPath, n.ContentRect)
		}
		if n.ContentRect.X < n.Bounds.X-eps ||
			n.ContentRect.Y < n.Bounds.Y-eps ||
			n.ContentRect.X+n.ContentRect.W > n.Bounds.X+n.Bounds.W+eps ||
			n.ContentRect.Y+n.ContentRect.H > n.Bounds.Y+n.Bounds.H+eps {
			t.Fatalf("node %q ContentRect %+v escapes Bounds %+v", n.RelPath, n.ContentRect, n.Bounds)
		}
		if len(n.Color) != 7 || n.Color[0] != '#' {
			t.Fatalf("node %q invalid Color %q", n.RelPath, n.Color)
		}
		expectedHex := fmt.Sprintf("#%02x%02x%02x", n.ColorRGBA[0], n.ColorRGBA[1], n.ColorRGBA[2])
		if n.Color != expectedHex {
			t.Fatalf("node %q Color %q mismatches ColorRGBA %v", n.RelPath, n.Color, n.ColorRGBA)
		}
		if n.ColorRGBA[3] != 255 {
			t.Fatalf("node %q alpha expected 255, got %d", n.RelPath, n.ColorRGBA[3])
		}
		if _, ok := res.ByPath[n.RelPath]; !ok {
			t.Fatalf("node %q missing from ByPath", n.RelPath)
		}
		if n.Depth > 0 {
			parentPath := parentRelPath(n.RelPath)
			childrenByParent[parentPath] = append(childrenByParent[parentPath], n)
		}
	}

	for parentPath, children := range childrenByParent {
		parent, ok := res.ByPath[parentPath]
		if !ok {
			t.Fatalf("parent %q not found in ByPath", parentPath)
		}
		pr := parent.ContentRect
		var areaSum float64
		for i, c := range children {
			cb := c.Bounds
			if cb.X < pr.X-eps || cb.Y < pr.Y-eps ||
				cb.X+cb.W > pr.X+pr.W+eps ||
				cb.Y+cb.H > pr.Y+pr.H+eps {
				t.Fatalf("child %q bounds %+v escape parent %q contentRect %+v", c.RelPath, cb, parentPath, pr)
			}
			areaSum += cb.W * cb.H
			for j := i + 1; j < len(children); j++ {
				ob := children[j].Bounds
				ix := math.Min(cb.X+cb.W, ob.X+ob.W) - math.Max(cb.X, ob.X)
				iy := math.Min(cb.Y+cb.H, ob.Y+ob.H) - math.Max(cb.Y, ob.Y)
				if ix > eps && iy > eps {
					t.Fatalf("siblings %q (%+v) and %q (%+v) overlap by (%g, %g)", c.RelPath, cb, children[j].RelPath, ob, ix, iy)
				}
			}
		}
		parentArea := pr.W * pr.H
		if math.Abs(areaSum-parentArea) > 1e-3 {
			t.Fatalf("parent %q area mismatch: sum=%g vs contentRect=%g", parentPath, areaSum, parentArea)
		}
	}
}

func parentRelPath(rel string) string {
	for i := len(rel) - 1; i >= 0; i-- {
		if rel[i] == '/' {
			return rel[:i]
		}
	}
	return ""
}

func TestComputeLayoutInvariants(t *testing.T) {
	tree := buildComplexTree()
	for _, preset := range []string{"sloc", "bytes", "symbols", "", "unknown"} {
		res := ComputeLayout(tree, preset)
		assertInvariants(t, res)
	}
}

func TestPresetWeightsAndSwitching(t *testing.T) {
	fSmallSlocManySyms := makeTestFile(1, 0, "syms.go", "syms.go", 1, 10, 100, 50, 1, []string{"package p"})
	fLargeBytes := makeTestFile(2, 0, "blob.json", "blob.json", 1, 5, 50000, 0, 0, []string{"{}"})
	fZero := makeTestFile(3, 0, "empty.txt", "empty.txt", 1, 0, 10, 0, 0, nil)
	root := makeTestDir(0, 0, "proj", "", 0, fSmallSlocManySyms, fLargeBytes, fZero)
	tree := &indexer.TreeResult{Root: root}

	slocRes := ComputeLayout(tree, "unknown")
	if slocRes.Preset != "sloc" {
		t.Fatalf("expected unknown preset to normalize to sloc, got %q", slocRes.Preset)
	}
	if slocRes.ByPath["syms.go"].Weight != 10 {
		t.Fatalf("expected syms.go sloc weight 10, got %g", slocRes.ByPath["syms.go"].Weight)
	}
	if slocRes.ByPath["empty.txt"].Weight != 1 {
		t.Fatalf("expected empty.txt sloc weight clamped to 1, got %g", slocRes.ByPath["empty.txt"].Weight)
	}
	if slocRes.ByPath[""].Weight != 16 {
		t.Fatalf("expected root sloc weight 16, got %g", slocRes.ByPath[""].Weight)
	}

	bytesRes := ComputeLayout(tree, "bytes")
	if bytesRes.Preset != "bytes" {
		t.Fatalf("expected bytes preset, got %q", bytesRes.Preset)
	}
	if bytesRes.ByPath["blob.json"].Weight != 50000 {
		t.Fatalf("expected blob.json bytes weight 50000, got %g", bytesRes.ByPath["blob.json"].Weight)
	}
	if bytesRes.ByPath["empty.txt"].Weight != 64 {
		t.Fatalf("expected empty.txt bytes weight clamped to 64, got %g", bytesRes.ByPath["empty.txt"].Weight)
	}

	symRes := ComputeLayout(tree, "symbols")
	if symRes.Preset != "symbols" {
		t.Fatalf("expected symbols preset, got %q", symRes.Preset)
	}
	if symRes.ByPath["syms.go"].Weight != 510 {
		t.Fatalf("expected syms.go symbols weight 510, got %g", symRes.ByPath["syms.go"].Weight)
	}
	if symRes.ByPath["empty.txt"].Weight != 1 {
		t.Fatalf("expected empty.txt symbols weight 1, got %g", symRes.ByPath["empty.txt"].Weight)
	}
}

func TestDeterministicSortOrderAndPreOrder(t *testing.T) {
	fZ := makeTestFile(1, 0, "z_file.go", "z_file.go", 1, 50, 500, 0, 0, []string{"a"})
	fA := makeTestFile(2, 0, "a_file.go", "a_file.go", 1, 50, 500, 0, 0, []string{"b"})
	dB := makeTestDir(3, 0, "b_dir", "b_dir", 1, makeTestFile(4, 3, "inner.go", "b_dir/inner.go", 2, 50, 500, 0, 0, []string{"c"}))
	dA := makeTestDir(5, 0, "a_dir", "a_dir", 1, makeTestFile(6, 5, "inner.go", "a_dir/inner.go", 2, 50, 500, 0, 0, []string{"d"}))
	fBig := makeTestFile(7, 0, "big.go", "big.go", 1, 200, 2000, 0, 0, []string{"e"})

	root := makeTestDir(0, 0, "root", "", 0, fZ, fA, dB, dA, fBig)
	tree := &indexer.TreeResult{Root: root}

	res1 := ComputeLayout(tree, "sloc")
	res2 := ComputeLayout(tree, "sloc")
	if !reflect.DeepEqual(res1, res2) {
		t.Fatalf("ComputeLayout is not deterministic across repeated calls")
	}

	var order []string
	for _, n := range res1.Nodes {
		order = append(order, n.RelPath)
	}
	want := []string{
		"",
		"big.go",
		"a_dir",
		"a_dir/inner.go",
		"b_dir",
		"b_dir/inner.go",
		"a_file.go",
		"z_file.go",
	}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("unexpected pre-order sorted traversal:\n got: %v\nwant: %v", order, want)
	}
}

func TestEdgeCasesNilEmptySingleAndTinyNested(t *testing.T) {
	nilRes := ComputeLayout(nil, "sloc")
	if len(nilRes.Nodes) != 0 || nilRes.ByPath == nil {
		t.Fatalf("expected empty Nodes and non-nil ByPath for nil tree, got %+v", nilRes)
	}

	emptyRoot := makeTestDir(0, 0, "empty", "", 0)
	emptyRes := ComputeLayout(&indexer.TreeResult{Root: emptyRoot}, "sloc")
	assertInvariants(t, emptyRes)
	if len(emptyRes.Nodes) != 1 || emptyRes.Nodes[0].Weight != 1 {
		t.Fatalf("expected single empty root with weight 1, got %+v", emptyRes.Nodes)
	}

	singleFile := makeTestFile(1, 0, "main.go", "main.go", 1, 30, 600, 1, 0, []string{"package main"})
	singleRes := ComputeLayout(&indexer.TreeResult{Root: makeTestDir(0, 0, "one", "", 0, singleFile)}, "sloc")
	assertInvariants(t, singleRes)
	if singleRes.ByPath["main.go"].Bounds != singleRes.ByPath[""].ContentRect {
		t.Fatalf("single child bounds %+v should fill parent contentRect %+v", singleRes.ByPath["main.go"].Bounds, singleRes.ByPath[""].ContentRect)
	}

	huge := makeTestFile(1, 0, "huge.go", "huge.go", 1, 50000, 1000000, 100, 20, []string{"package huge"})
	n5 := makeTestFile(6, 5, "leaf.go", "d1/d2/d3/d4/leaf.go", 5, 1, 10, 0, 0, []string{"x"})
	n4 := makeTestDir(5, 4, "d4", "d1/d2/d3/d4", 4, n5)
	n3 := makeTestDir(4, 3, "d3", "d1/d2/d3", 3, n4)
	n2 := makeTestDir(3, 2, "d2", "d1/d2", 2, n3)
	n1 := makeTestDir(2, 0, "d1", "d1", 1, n2)
	skewedTree := &indexer.TreeResult{Root: makeTestDir(0, 0, "skew", "", 0, huge, n1)}
	skewRes := ComputeLayout(skewedTree, "sloc")
	assertInvariants(t, skewRes)
}

func TestExtrusionModuleColorsAndPreviewLines(t *testing.T) {
	var modules []*indexer.Node
	var id uint32 = 1
	for i := 0; i < 20; i++ {
		modName := fmt.Sprintf("mod%02d", i)
		f := makeTestFile(id+1, id, "file.go", modName+"/file.go", 2, 15+i*5, 400, 2, 1, []string{"package p", "func f() {}"})
		d := makeTestDir(id, 0, modName, modName, 1, f)
		modules = append(modules, d)
		id += 2
	}
	root := makeTestDir(0, 0, "workspace", "", 0, modules...)
	res := ComputeLayout(&indexer.TreeResult{Root: root}, "sloc")
	assertInvariants(t, res)

	if res.ByPath[""].ModuleName != "workspace" {
		t.Fatalf("expected root ModuleName 'workspace', got %q", res.ByPath[""].ModuleName)
	}
	if res.ByPath[""].Z != 0 || res.ByPath[""].Height != 1.2 {
		t.Fatalf("unexpected root Z/Height: %g / %g", res.ByPath[""].Z, res.ByPath[""].Height)
	}

	seenColors := make(map[string]bool)
	for i := 0; i < 20; i++ {
		modName := fmt.Sprintf("mod%02d", i)
		modNode := res.ByPath[modName]
		if modNode.ModuleName != modName {
			t.Fatalf("expected ModuleName %q, got %q", modName, modNode.ModuleName)
		}
		if math.Abs(modNode.Z-1.4) > 1e-9 || modNode.Height != 1.2 {
			t.Fatalf("unexpected depth-1 dir Z/Height for %s: %g / %g", modName, modNode.Z, modNode.Height)
		}
		seenColors[modNode.Color] = true

		childNode := res.ByPath[modName+"/file.go"]
		if childNode.ModuleName != modName {
			t.Fatalf("expected child ModuleName %q, got %q", modName, childNode.ModuleName)
		}
		expectedZ := 2.8
		expectedH := 1.6 + 1.8*math.Log2(float64(15+i*5)+1.0)
		if math.Abs(childNode.Z-expectedZ) > 1e-9 || math.Abs(childNode.Height-expectedH) > 1e-9 {
			t.Fatalf("unexpected leaf Z/Height: got (%g, %g), want (%g, %g)", childNode.Z, childNode.Height, expectedZ, expectedH)
		}
	}
	if len(seenColors) < 16 {
		t.Fatalf("expected distinct palette colors across 20 modules, got %d unique", len(seenColors))
	}

	longLines := make([]string, 130)
	for i := range longLines {
		if i == 0 {
			longLines[i] = "short"
		} else if i == 129 {
			longLines[i] = "01234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789"
		} else {
			longLines[i] = "    x := 1"
		}
	}
	fileNode := makeTestFile(1, 0, "sample.go", "sample.go", 1, 130, 2000, 1, 0, longLines)
	pRes := ComputeLayout(&indexer.TreeResult{Root: makeTestDir(0, 0, "p", "", 0, fileNode)}, "sloc")
	if len(pRes.ByPath[""].PreviewLines) != 0 {
		t.Fatalf("expected directory PreviewLines to be empty")
	}
	pl := pRes.ByPath["sample.go"].PreviewLines
	if len(pl) != 64 {
		t.Fatalf("expected 64 sampled preview lines, got %d", len(pl))
	}
	if pl[0] != 5 || pl[63] != 100 {
		t.Fatalf("unexpected sampled endpoint lengths: first=%d, last=%d", pl[0], pl[63])
	}
}

func TestComputeLayoutFromIndexedDirectory(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	mustWrite("cmd/app/main.go", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n")
	mustWrite("internal/engine/calc.go", "package engine\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n\nfunc Sub(a, b int) int {\n\treturn a - b\n}\n")
	mustWrite("web/app.js", "function render() {\n  return 42;\n}\n")
	mustWrite("README.md", "# Demo\n\nOverview text\n")

	tree, err := indexer.IndexDirectory(dir)
	if err != nil {
		t.Fatalf("IndexDirectory failed: %v", err)
	}

	for _, preset := range []string{"sloc", "bytes", "symbols"} {
		res := ComputeLayout(tree, preset)
		assertInvariants(t, res)
		calcNode, ok := res.ByPath["internal/engine/calc.go"]
		if !ok {
			t.Fatalf("expected internal/engine/calc.go in ByPath")
		}
		if calcNode.ModuleName != "internal" {
			t.Fatalf("expected ModuleName internal, got %q", calcNode.ModuleName)
		}
		if len(calcNode.Symbols) < 2 {
			t.Fatalf("expected >= 2 symbols on calc.go, got %d", len(calcNode.Symbols))
		}
		if len(calcNode.PreviewLines) == 0 {
			t.Fatalf("expected PreviewLines populated on calc.go")
		}
	}
}

