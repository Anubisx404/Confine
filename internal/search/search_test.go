package search

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"confine/internal/indexer"
	"confine/internal/treemap"
)

func makeTestNode(id uint32, relPath string, name string, isDir bool, isMarkdown bool, lines []string) *indexer.Node {
	var totalBytes int64
	for _, l := range lines {
		totalBytes += int64(len(l) + 1)
	}
	return &indexer.Node{
		ID:      id,
		Name:    name,
		RelPath: relPath,
		IsDir:   isDir,
		Stats: indexer.FileStats{
			ByteSize:        totalBytes,
			LineCount:       len(lines),
			SourceLineCount: len(lines),
			IsMarkdown:      isMarkdown,
		},
		Lines: lines,
	}
}

func makeTestLayout(nodes []*indexer.Node, boundsOffset float64) *treemap.LayoutResult {
	layoutNodes := make([]treemap.LayoutNode, 0, len(nodes))
	byPath := make(map[string]treemap.LayoutNode, len(nodes))
	for i, n := range nodes {
		if n.IsDir {
			continue
		}
		rect := treemap.Rect{
			X: boundsOffset + float64(i)*10,
			Y: boundsOffset + float64(i)*5,
			W: 50,
			H: 30,
		}
		z := float64(i+1) * 1.4
		ln := treemap.LayoutNode{
			ID:      n.ID,
			Name:    n.Name,
			RelPath: n.RelPath,
			Bounds:  rect,
			Z:       z,
		}
		layoutNodes = append(layoutNodes, ln)
		byPath[n.RelPath] = ln
	}
	return &treemap.LayoutResult{
		Preset:     "sloc",
		CanvasRect: treemap.Rect{X: 0, Y: 0, W: 1000, H: 650},
		Nodes:      layoutNodes,
		ByPath:     byPath,
	}
}

func TestSearchBasicCaseInsensitiveAndCoordinates(t *testing.T) {
	nodes := []*indexer.Node{
		makeTestNode(0, "", "root", true, false, nil),
		makeTestNode(1, "pkg/alpha.go", "alpha.go", false, false, []string{
			"package pkg",
			"  func ComputeValue(input string) string {",
			"    return input + ComputeValueHelper(input)",
			"  }",
		}),
		makeTestNode(2, "pkg/utf8.go", "utf8.go", false, false, []string{
			"αβγ valueTarget one valueTarget two",
		}),
	}
	tree := &indexer.TreeResult{
		FlatNodes: nodes,
	}
	layout := makeTestLayout(nodes, 0)
	idx := NewIndex(tree, layout)

	res := idx.Search("computevalue", 50)
	if res.TotalMatches != 2 {
		t.Fatalf("expected 2 matches, got %d", res.TotalMatches)
	}
	if len(res.Matches) != 2 {
		t.Fatalf("expected 2 match items, got %d", len(res.Matches))
	}
	if res.FileHits["pkg/alpha.go"] != 2 {
		t.Fatalf("expected FileHits[pkg/alpha.go] == 2, got %d", res.FileHits["pkg/alpha.go"])
	}

	m0 := res.Matches[0]
	if m0.RelPath != "pkg/alpha.go" || m0.Line != 2 || m0.Column != 8 {
		t.Fatalf("unexpected first match: %+v", m0)
	}
	if m0.Snippet != "func ComputeValue(input string) string {" {
		t.Fatalf("unexpected snippet: %q", m0.Snippet)
	}
	if m0.Bounds.X != 10 || m0.Z != 2.8 {
		t.Fatalf("unexpected bounds/z: %+v z=%v", m0.Bounds, m0.Z)
	}

	m1 := res.Matches[1]
	if m1.Line != 3 || m1.Column != 20 {
		t.Fatalf("unexpected second match: %+v", m1)
	}

	resUTF := idx.Search("VALUETARGET", 50)
	if resUTF.TotalMatches != 2 {
		t.Fatalf("expected 2 UTF-8 matches, got %d", resUTF.TotalMatches)
	}
	if resUTF.Matches[0].Line != 1 || resUTF.Matches[0].Column != 5 {
		t.Fatalf("expected 1-based rune column 5, got %+v", resUTF.Matches[0])
	}
	if resUTF.Matches[1].Line != 1 || resUTF.Matches[1].Column != 21 {
		t.Fatalf("expected 1-based rune column 21, got %+v", resUTF.Matches[1])
	}
}

func TestSearchEmptyAndShortQueries(t *testing.T) {
	nodes := []*indexer.Node{
		makeTestNode(1, "main.go", "main.go", false, false, []string{
			"x := 1",
			"xy := 2",
		}),
	}
	idx := NewIndex(&indexer.TreeResult{FlatNodes: nodes}, nil)

	emptyRes := idx.Search("   \t  ", 10)
	if emptyRes.TotalMatches != 0 || emptyRes.Matches == nil || len(emptyRes.Matches) != 0 || emptyRes.FileHits == nil || len(emptyRes.FileHits) != 0 {
		t.Fatalf("unexpected empty query result: %+v", emptyRes)
	}

	short1 := idx.Search("X", 10)
	if short1.TotalMatches != 2 {
		t.Fatalf("expected 2 matches for 1-char query, got %d", short1.TotalMatches)
	}

	short2 := idx.Search("XY", 10)
	if short2.TotalMatches != 1 {
		t.Fatalf("expected 1 match for 2-char query, got %d", short2.TotalMatches)
	}
}

func TestSearchSnippetTruncation(t *testing.T) {
	prefix := strings.Repeat("a", 240)
	suffix := strings.Repeat("b", 240)
	longLine := "   " + prefix + "NeedleToken" + suffix + "   "

	nodes := []*indexer.Node{
		makeTestNode(1, "long.txt", "long.txt", false, false, []string{longLine}),
	}
	idx := NewIndex(&indexer.TreeResult{FlatNodes: nodes}, nil)

	res := idx.Search("needletoken", 10)
	if res.TotalMatches != 1 || len(res.Matches) != 1 {
		t.Fatalf("expected 1 match, got total=%d len=%d", res.TotalMatches, len(res.Matches))
	}
	snippet := res.Matches[0].Snippet
	if !strings.Contains(snippet, "NeedleToken") {
		t.Fatalf("snippet missing matched token: %q", snippet)
	}
	if runeLen := utf8.RuneCountInString(snippet); runeLen > 165 {
		t.Fatalf("snippet too long: %d runes (%q)", runeLen, snippet)
	}
}

func TestSearchLimitAndCompleteFileHits(t *testing.T) {
	var nodes []*indexer.Node
	for i := 0; i < 650; i++ {
		lines := []string{
			"alpha line one targetHit",
			"alpha line two targetHit",
			"alpha line three targetHit",
		}
		nodes = append(nodes, makeTestNode(uint32(i+1), fmt.Sprintf("dir/file_%04d.go", i), fmt.Sprintf("file_%04d.go", i), false, false, lines))
	}

	idx := NewIndex(&indexer.TreeResult{FlatNodes: nodes}, nil)

	resSmall := idx.Search("targethit", 5)
	if len(resSmall.Matches) != 5 {
		t.Fatalf("expected 5 matches capped by limit, got %d", len(resSmall.Matches))
	}
	if resSmall.TotalMatches != 650*3 {
		t.Fatalf("expected TotalMatches == %d, got %d", 650*3, resSmall.TotalMatches)
	}
	if len(resSmall.FileHits) != 650 {
		t.Fatalf("expected 650 files in FileHits, got %d", len(resSmall.FileHits))
	}
	for _, n := range nodes {
		if resSmall.FileHits[n.RelPath] != 3 {
			t.Fatalf("expected 3 hits for %s, got %d", n.RelPath, resSmall.FileHits[n.RelPath])
		}
	}
	if resSmall.Matches[0].RelPath != "dir/file_0000.go" || resSmall.Matches[3].RelPath != "dir/file_0001.go" {
		t.Fatalf("matches not in deterministic order: %+v", resSmall.Matches)
	}

	resDefault := idx.Search("targethit", 0)
	if len(resDefault.Matches) != 200 {
		t.Fatalf("expected default limit 200, got %d", len(resDefault.Matches))
	}

	resCap := idx.Search("targethit", 5000)
	if len(resCap.Matches) != 1000 {
		t.Fatalf("expected capped limit 1000, got %d", len(resCap.Matches))
	}
}

func TestUpdateLayoutAndConcurrency(t *testing.T) {
	nodes := []*indexer.Node{
		makeTestNode(1, "a.go", "a.go", false, false, []string{"func Hello() {}"}),
		makeTestNode(2, "README.md", "README.md", false, true, []string{"# Hello Doc"}),
	}
	layout1 := makeTestLayout(nodes, 0)
	layout2 := makeTestLayout(nodes, 500)

	idx := NewIndex(&indexer.TreeResult{FlatNodes: nodes}, layout1)
	res1 := idx.Search("Hello", 10)
	if res1.Matches[0].Bounds.X != 0 {
		t.Fatalf("expected initial X=0, got %v", res1.Matches[0].Bounds.X)
	}

	idx.UpdateLayout(layout2)
	res2 := idx.Search("Hello", 10)
	if res2.Matches[0].Bounds.X != 500 {
		t.Fatalf("expected updated X=500, got %v", res2.Matches[0].Bounds.X)
	}
	mds := idx.Markdowns()
	if len(mds) != 1 || mds[0].Bounds.X != 510 {
		t.Fatalf("expected markdown bounds updated to X=510, got %+v", mds)
	}

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for k := 0; k < 50; k++ {
				if id%2 == 0 {
					_ = idx.Search("hello", 10)
					_ = idx.Markdowns()
				} else if k%2 == 0 {
					idx.UpdateLayout(layout1)
				} else {
					idx.UpdateLayout(layout2)
				}
			}
		}(g)
	}
	wg.Wait()
}

func TestMarkdownsCatalogSortingAndTitles(t *testing.T) {
	nodes := []*indexer.Node{
		makeTestNode(1, "docs/z.md", "z.md", false, true, []string{"## Only H2", "body text"}),
		makeTestNode(2, "docs/README.md", "README.md", false, true, []string{"# Nested Readme"}),
		makeTestNode(3, "CONTRIBUTING.md", "CONTRIBUTING.md", false, true, []string{"# Contributing Guide"}),
		makeTestNode(4, "PLAN.md", "PLAN.md", false, true, []string{"```bash", "# not a heading", "```", "# Execution Plan"}),
		makeTestNode(5, "README.md", "README.md", false, true, []string{"# Confine"}),
		makeTestNode(6, "ARCHITECTURE.md", "ARCHITECTURE.md", false, true, []string{"# Architecture Overview"}),
		makeTestNode(7, "CHANGELOG.md", "CHANGELOG.md", false, true, []string{"# Changelog"}),
		makeTestNode(8, "SPEC.md", "SPEC.md", false, true, []string{"# Specification"}),
		makeTestNode(9, "alpha.md", "alpha.md", false, true, []string{"# Alpha Doc"}),
		makeTestNode(10, "code.go", "code.go", false, false, []string{"package main"}),
	}

	idx := NewIndex(&indexer.TreeResult{FlatNodes: nodes}, makeTestLayout(nodes, 0))
	mds := idx.Markdowns()

	expectedOrder := []string{
		"README.md",
		"PLAN.md",
		"SPEC.md",
		"ARCHITECTURE.md",
		"CHANGELOG.md",
		"CONTRIBUTING.md",
		"alpha.md",
		"docs/README.md",
		"docs/z.md",
	}

	if len(mds) != len(expectedOrder) {
		t.Fatalf("expected %d markdown entries, got %d", len(expectedOrder), len(mds))
	}
	for i, want := range expectedOrder {
		if mds[i].RelPath != want {
			t.Fatalf("index %d: expected %q, got %q", i, want, mds[i].RelPath)
		}
	}

	if mds[1].Title != "Execution Plan" {
		t.Fatalf("expected fenced code block ignored for PLAN.md title, got %q", mds[1].Title)
	}
	if mds[8].Title != "z.md" {
		t.Fatalf("expected fallback filename title z.md, got %q", mds[8].Title)
	}

	mds[0].Title = "mutated"
	mdsAgain := idx.Markdowns()
	if mdsAgain[0].Title != "Confine" {
		t.Fatalf("expected safe slice copy, got %q", mdsAgain[0].Title)
	}
}

func buildSyntheticIndex(fileCount int) *Index {
	nodes := make([]*indexer.Node, 0, fileCount)
	for i := 0; i < fileCount; i++ {
		lines := make([]string, 80)
		for l := 0; l < 80; l++ {
			if i%50 == 0 && l == 40 {
				lines[l] = fmt.Sprintf("func HandleSpecialQuery_%d(ctx Context) error { return nil }", i)
			} else {
				lines[l] = fmt.Sprintf("const item_%d_%d = computeMetric(%d, %d)", i, l, i, l)
			}
		}
		nodes = append(nodes, makeTestNode(
			uint32(i+1),
			fmt.Sprintf("module_%02d/file_%04d.go", i%20, i),
			fmt.Sprintf("file_%04d.go", i),
			false,
			false,
			lines,
		))
	}
	return NewIndex(&indexer.TreeResult{FlatNodes: nodes}, makeTestLayout(nodes, 0))
}

func TestSearch2000FilesSub10ms(t *testing.T) {
	idx := buildSyntheticIndex(2000)
	_ = idx.Search("HandleSpecialQuery", 50)
	res := idx.Search("HandleSpecialQuery", 50)
	if res.TotalMatches != 40 {
		t.Fatalf("expected 40 matches across 2000 files, got %d", res.TotalMatches)
	}
	if res.ElapsedMs >= 10.0 {
		t.Fatalf("expected sub-10ms search, got %.3f ms", res.ElapsedMs)
	}
}

func BenchmarkSearch(b *testing.B) {
	idx := buildSyntheticIndex(2000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := idx.Search("HandleSpecialQuery", 200)
		if res.TotalMatches != 40 {
			b.Fatalf("unexpected match count: %d", res.TotalMatches)
		}
	}
}
