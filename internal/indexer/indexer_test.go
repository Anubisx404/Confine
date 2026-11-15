package indexer

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, root string, rel string, content []byte) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}
	if err := os.WriteFile(full, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

func TestIndexDirectoryValidationAndEmptyRoot(t *testing.T) {
	if _, err := IndexDirectory(""); err == nil {
		t.Fatalf("expected error for empty rootPath")
	}
	if _, err := IndexDirectory(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatalf("expected error for nonexistent rootPath")
	}

	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "notadir.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := IndexDirectory(filePath); err == nil {
		t.Fatalf("expected error when rootPath is a file")
	}

	emptyRoot := t.TempDir()
	res, err := IndexDirectory(emptyRoot)
	if err != nil {
		t.Fatalf("IndexDirectory empty root: %v", err)
	}
	if res.Root == nil {
		t.Fatalf("expected non-nil root")
	}
	if res.Root.ID != 0 || res.Root.ParentID != 0 || res.Root.RelPath != "" || !res.Root.IsDir {
		t.Fatalf("unexpected empty root metadata: %+v", res.Root)
	}
	if res.TotalDirs != 1 || res.TotalFiles != 0 || len(res.FlatNodes) != 1 {
		t.Fatalf("unexpected empty totals: dirs=%d files=%d flat=%d", res.TotalDirs, res.TotalFiles, len(res.FlatNodes))
	}
}

func TestDetectLanguageAndBinaryDetection(t *testing.T) {
	cases := []struct {
		fileName string
		content  []byte
		wantLang string
		wantBin  bool
	}{
		{"main.go", []byte("package main\n"), "Go", false},
		{"lib.rs", []byte("fn main() {}\n"), "Rust", false},
		{"util.c", []byte("int x = 0;\n"), "C", false},
		{"util.h", []byte("int x;\n"), "C", false},
		{"engine.cpp", []byte("class A {};\n"), "C++", false},
		{"Program.cs", []byte("class Program {}\n"), "C#", false},
		{"App.java", []byte("class App {}\n"), "Java", false},
		{"widget.dart", []byte("void main() {}\n"), "Dart", false},
		{"index.ts", []byte("export const a = 1;\n"), "TypeScript", false},
		{"app.tsx", []byte("export const a = 1;\n"), "TypeScript", false},
		{"script.js", []byte("const a = 1;\n"), "JavaScript", false},
		{"run.py", []byte("def f():\n    pass\n"), "Python", false},
		{"index.html", []byte("<html></html>\n"), "HTML", false},
		{"styles.css", []byte("body { color: #000; }\n"), "CSS", false},
		{"data.json", []byte("{}\n"), "JSON", false},
		{"config.yaml", []byte("key: val\n"), "YAML", false},
		{"README.md", []byte("# Title\n"), "Markdown", false},
		{"schema.sql", []byte("SELECT 1;\n"), "SQL", false},
		{"build.sh", []byte("echo hi\n"), "Shell", false},
		{"Cargo.toml", []byte("[package]\n"), "TOML", false},
		{"manifest.xml", []byte("<root/>\n"), "XML", false},
		{"notes.txt", []byte("plain text\n"), "Text", false},
		{"Dockerfile", []byte("FROM scratch\n"), "Dockerfile", false},
		{"Dockerfile.dev", []byte("FROM alpine\n"), "Dockerfile", false},
		{"Makefile", []byte("all:\n\techo ok\n"), "Makefile", false},
		{"photo.png", []byte("not really png"), "", true},
		{"archive.zip", []byte("PK"), "", true},
		{"app.wasm", []byte("wasm"), "", true},
		{"bin.exe", []byte("MZ"), "", true},
		{"payload.dat", []byte{'a', 'b', 0x00, 'c'}, "", true},
	}

	for _, tc := range cases {
		lang, isBin := DetectLanguage(tc.fileName, tc.content)
		if isBin != tc.wantBin || lang != tc.wantLang {
			t.Fatalf("DetectLanguage(%q): got (%q, %v), want (%q, %v)", tc.fileName, lang, isBin, tc.wantLang, tc.wantBin)
		}
	}

	largePrefix := bytes.Repeat([]byte("a"), 8192)
	largeWithLateNul := append(largePrefix, 0x00)
	lang, isBin := DetectLanguage("late.txt", largeWithLateNul)
	if isBin || lang != "Text" {
		t.Fatalf("expected NUL after 8192 bytes not to trigger binary check, got (%q, %v)", lang, isBin)
	}
}

func TestCountLinesAndSLOC(t *testing.T) {
	lineSlash := "/" + "/"
	blockStart := "/" + "*"
	blockEnd := "*" + "/"
	htmlStart := "<!" + "--"
	htmlEnd := "--" + ">"

	total, sloc, lines := CountLines("Go", []byte{})
	if total != 0 || sloc != 0 || len(lines) != 0 {
		t.Fatalf("empty file: got (%d, %d, %v)", total, sloc, lines)
	}

	goSrc := strings.Join([]string{
		"package main",
		"",
		lineSlash + " single line comment",
		blockStart + " single line block " + blockEnd,
		blockStart + " multi",
		"   line block",
		blockEnd,
		blockStart + " inline " + blockEnd + " func f() {",
		"\ts := \"" + blockStart + " not a comment\"",
		"\tx := 1 " + lineSlash + " trailing comment",
		"}",
	}, "\r\n") + "\r\n"

	total, sloc, lines = CountLines("Go", []byte(goSrc))
	if total != 11 {
		t.Fatalf("Go total lines: got %d, want 11", total)
	}
	if sloc != 5 {
		t.Fatalf("Go SLOC: got %d, want 5", sloc)
	}
	if len(lines) != 11 || lines[0] != "package main" || lines[10] != "}" {
		t.Fatalf("unexpected split lines: %v", lines)
	}

	rustSrc := strings.Join([]string{
		"fn borrow<'a>(s: &'a str) -> &'a str {",
		"    " + lineSlash + " comment after lifetime",
		"    let c = '/';",
		"    s",
		"}",
	}, "\n")
	total, sloc, _ = CountLines("Rust", []byte(rustSrc))
	if total != 5 || sloc != 4 {
		t.Fatalf("Rust lines: got (%d, %d), want (5, 4)", total, sloc)
	}

	pySrc := "# header\n\ndef add(a, b):\n    # body\n    return a + b\n"
	total, sloc, _ = CountLines("Python", []byte(pySrc))
	if total != 5 || sloc != 2 {
		t.Fatalf("Python lines: got (%d, %d), want (5, 2)", total, sloc)
	}

	sqlSrc := "-- comment\n" + blockStart + " block " + blockEnd + "\nSELECT 1;\n"
	total, sloc, _ = CountLines("SQL", []byte(sqlSrc))
	if total != 3 || sloc != 1 {
		t.Fatalf("SQL lines: got (%d, %d), want (3, 1)", total, sloc)
	}

	htmlSrc := htmlStart + "\nmulti\n" + htmlEnd + "\n<p>It's fine</p>\n"
	total, sloc, _ = CountLines("HTML", []byte(htmlSrc))
	if total != 4 || sloc != 1 {
		t.Fatalf("HTML lines: got (%d, %d), want (4, 1)", total, sloc)
	}
}

func TestGitignoreHierarchicalRules(t *testing.T) {
	root := t.TempDir()

	gitignoreRoot := strings.Join([]string{
		"# comment",
		"",
		"*.log",
		"!important.log",
		"/rootonly.txt",
		"dirmatch/",
		"file?.txt",
		"**" + "/gen/" + "*.go",
		"docs/" + "**" + "/draft.md",
	}, "\n")
	writeTestFile(t, root, ".gitignore", []byte(gitignoreRoot))
	writeTestFile(t, root, "sub/.gitignore", []byte("!subkeep.log\nlocalonly.txt\n"))

	writeTestFile(t, root, "app.log", []byte("ignored"))
	writeTestFile(t, root, "important.log", []byte("kept"))
	writeTestFile(t, root, "rootonly.txt", []byte("ignored"))
	writeTestFile(t, root, "sub/rootonly.txt", []byte("kept because rooted at /"))
	writeTestFile(t, root, "dirmatch/inside.txt", []byte("ignored dir"))
	writeTestFile(t, root, "sub/dirmatch", []byte("kept because file, not dir"))
	writeTestFile(t, root, "fileA.txt", []byte("ignored by ?"))
	writeTestFile(t, root, "fileAB.txt", []byte("kept"))
	writeTestFile(t, root, "a/b/gen/code.go", []byte("package gen\n"))
	writeTestFile(t, root, "docs/draft.md", []byte("# draft"))
	writeTestFile(t, root, "docs/v1/api/draft.md", []byte("# draft nested"))
	writeTestFile(t, root, "docs/v1/guide.md", []byte("# guide"))
	writeTestFile(t, root, "sub/drop.log", []byte("ignored by root *.log"))
	writeTestFile(t, root, "sub/subkeep.log", []byte("re-included by child .gitignore"))
	writeTestFile(t, root, "sub/localonly.txt", []byte("ignored in sub"))
	writeTestFile(t, root, "localonly.txt", []byte("kept at root"))

	res, err := IndexDirectory(root)
	if err != nil {
		t.Fatalf("IndexDirectory: %v", err)
	}

	mustExist := []string{
		".gitignore",
		"important.log",
		"sub/rootonly.txt",
		"sub/dirmatch",
		"fileAB.txt",
		"docs/v1/guide.md",
		"sub/subkeep.log",
		"localonly.txt",
	}
	for _, p := range mustExist {
		if _, ok := res.FilesByPath[p]; !ok {
			t.Fatalf("expected %q to be indexed, FilesByPath=%v", p, keysOf(res.FilesByPath))
		}
	}

	mustSkip := []string{
		"app.log",
		"rootonly.txt",
		"dirmatch/inside.txt",
		"fileA.txt",
		"a/b/gen/code.go",
		"docs/draft.md",
		"docs/v1/api/draft.md",
		"sub/drop.log",
		"sub/localonly.txt",
	}
	for _, p := range mustSkip {
		if _, ok := res.FilesByPath[p]; ok {
			t.Fatalf("expected %q to be ignored", p)
		}
	}
}

func TestHardSkipsPruningAggregationAndDeterminism(t *testing.T) {
	root := t.TempDir()

	writeTestFile(t, root, ".gitignore", []byte("!.git\n!node_modules\n!go.sum\n"))
	writeTestFile(t, root, ".git/HEAD", []byte("ref: refs/heads/main\n"))
	writeTestFile(t, root, "node_modules/pkg/index.js", []byte("module.exports = 1;\n"))
	writeTestFile(t, root, "build/output.txt", []byte("artifact"))
	writeTestFile(t, root, ".dart_tool/package_config.json", []byte("{}"))
	writeTestFile(t, root, "package-lock.json", []byte("{}"))
	writeTestFile(t, root, "go.sum", []byte("mod v1\n"))
	writeTestFile(t, root, "Cargo.lock", []byte("lock\n"))
	writeTestFile(t, root, "pubspec.lock", []byte("lock\n"))

	writeTestFile(t, root, "onlybin/logo.png", []byte("png"))
	writeTestFile(t, root, "onlybin/nul.txt", []byte{'x', 0x00, 'y'})

	writeTestFile(t, root, "README.md", []byte("# Confine\n\nOverview.\n"))
	writeTestFile(t, root, "pkg/alpha.go", []byte("package pkg\n\nimport \"fmt\"\n\nfunc Alpha() {\n\tfmt.Println(\"hi\")\n}\n"))
	writeTestFile(t, root, "pkg/sub/beta.go", []byte("package sub\n\nfunc Beta() int {\n\treturn 42\n}\n"))

	res1, err := IndexDirectory(root)
	if err != nil {
		t.Fatalf("IndexDirectory: %v", err)
	}

	if _, ok := res1.FilesByPath[".git/HEAD"]; ok {
		t.Fatalf("hard-skip dir .git was indexed")
	}
	if _, ok := res1.FilesByPath["node_modules/pkg/index.js"]; ok {
		t.Fatalf("hard-skip dir node_modules was indexed")
	}
	if _, ok := res1.FilesByPath["go.sum"]; ok {
		t.Fatalf("hard-skip lockfile go.sum was indexed")
	}
	if _, ok := res1.FilesByPath["onlybin/logo.png"]; ok {
		t.Fatalf("binary extension was indexed")
	}
	if _, ok := res1.FilesByPath["onlybin/nul.txt"]; ok {
		t.Fatalf("binary NUL-byte file was indexed")
	}
	for _, n := range res1.FlatNodes {
		if n.RelPath == "onlybin" {
			t.Fatalf("empty directory onlybin should have been pruned")
		}
	}

	readme, ok := res1.FilesByPath["README.md"]
	if !ok || !readme.Stats.IsMarkdown {
		t.Fatalf("expected README.md with IsMarkdown=true, got %+v", readme)
	}

	alpha, ok := res1.FilesByPath["pkg/alpha.go"]
	if !ok {
		t.Fatalf("expected pkg/alpha.go")
	}
	if alpha.Stats.SymbolCount < 1 || alpha.Stats.ImportCount < 1 {
		t.Fatalf("expected analyzer stats on pkg/alpha.go, got %+v", alpha.Stats)
	}

	var pkgNode *Node
	for _, n := range res1.FlatNodes {
		if n.RelPath == "pkg" {
			pkgNode = n
			break
		}
	}
	if pkgNode == nil || !pkgNode.IsDir {
		t.Fatalf("expected pkg directory node")
	}
	if pkgNode.Stats.ChildrenCount != 2 {
		t.Fatalf("pkg ChildrenCount: got %d, want 2", pkgNode.Stats.ChildrenCount)
	}
	if pkgNode.Stats.DescendantCount != 3 {
		t.Fatalf("pkg DescendantCount: got %d, want 3 (alpha.go, sub, beta.go)", pkgNode.Stats.DescendantCount)
	}
	if pkgNode.Stats.ByteSize != alpha.Stats.ByteSize+res1.FilesByPath["pkg/sub/beta.go"].Stats.ByteSize {
		t.Fatalf("pkg ByteSize mismatch: %d", pkgNode.Stats.ByteSize)
	}

	for i, n := range res1.FlatNodes {
		if n.ID != uint32(i) {
			t.Fatalf("expected pre-order ID %d, got %d", i, n.ID)
		}
		if strings.Contains(n.RelPath, "\\") {
			t.Fatalf("RelPath not forward-slash normalized: %q", n.RelPath)
		}
	}
	if res1.Root.ID != 0 || res1.Root.ParentID != 0 {
		t.Fatalf("expected root ID=0, ParentID=0, got %d/%d", res1.Root.ID, res1.Root.ParentID)
	}
	if res1.TotalDirs+res1.TotalFiles != len(res1.FlatNodes) {
		t.Fatalf("TotalDirs(%d)+TotalFiles(%d) != len(FlatNodes)(%d)", res1.TotalDirs, res1.TotalFiles, len(res1.FlatNodes))
	}

	res2, err := IndexDirectory(root)
	if err != nil {
		t.Fatalf("IndexDirectory second run: %v", err)
	}
	if len(res1.FlatNodes) != len(res2.FlatNodes) {
		t.Fatalf("determinism failed: len %d vs %d", len(res1.FlatNodes), len(res2.FlatNodes))
	}
	for i := range res1.FlatNodes {
		if res1.FlatNodes[i].RelPath != res2.FlatNodes[i].RelPath ||
			res1.FlatNodes[i].ID != res2.FlatNodes[i].ID ||
			res1.FlatNodes[i].ParentID != res2.FlatNodes[i].ParentID ||
			res1.FlatNodes[i].Stats != res2.FlatNodes[i].Stats {
			t.Fatalf("determinism mismatch at node %d: %+v vs %+v", i, res1.FlatNodes[i], res2.FlatNodes[i])
		}
	}
}

func keysOf(m map[string]*Node) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestIndexDirectoryParallelDeterminism(t *testing.T) {
	root := t.TempDir()
	for d := 0; d < 6; d++ {
		for f := 0; f < 20; f++ {
			rel := filepath.Join("mod_"+string(rune('a'+d)), "file_"+string(rune('a'+f))+".go")
			src := "package mod\n\nfunc Fn() int {\n\treturn 1\n}\n"
			writeTestFile(t, root, filepath.ToSlash(rel), []byte(src))
		}
	}

	first, err := IndexDirectory(root)
	if err != nil {
		t.Fatalf("first IndexDirectory: %v", err)
	}
	if first.TotalFiles != 120 || first.TotalDirs != 7 {
		t.Fatalf("unexpected counts: files=%d dirs=%d", first.TotalFiles, first.TotalDirs)
	}
	if first.TotalSLOC != 480 || first.TotalLines != 600 {
		t.Fatalf("unexpected lines/sloc: lines=%d sloc=%d", first.TotalLines, first.TotalSLOC)
	}

	second, err := IndexDirectory(root)
	if err != nil {
		t.Fatalf("second IndexDirectory: %v", err)
	}
	if len(first.FlatNodes) != len(second.FlatNodes) {
		t.Fatalf("flat node count mismatch: %d vs %d", len(first.FlatNodes), len(second.FlatNodes))
	}
	for i := range first.FlatNodes {
		a := first.FlatNodes[i]
		b := second.FlatNodes[i]
		if a.ID != b.ID || a.ParentID != b.ParentID || a.RelPath != b.RelPath || a.Stats != b.Stats {
			t.Fatalf("mismatch at %d: %+v vs %+v", i, a, b)
		}
	}
}

func TestNormalizeExcludes(t *testing.T) {
	got := NormalizeExcludes([]string{
		"",
		"   ",
		"/",
		"./",
		"  .\\pkg\\sub\\  ",
		"./pkg/sub/",
		"/README.md",
		"  *_test.go ",
		"*_test.go",
		"docs" + string('/') + "guide.md",
	})
	want := []string{
		"pkg/sub/",
		"README.md",
		"*_test.go",
		"docs/guide.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeExcludes = %v, want %v", got, want)
	}

	empty := NormalizeExcludes(nil)
	if len(empty) != 0 {
		t.Fatalf("expected empty slice for nil input, got %v", empty)
	}
}

func TestIndexDirectoryWithOptionsExcludes(t *testing.T) {
	root := t.TempDir()

	writeTestFile(t, root, ".git/HEAD", []byte("ref: refs/heads/main\n"))
	writeTestFile(t, root, "go.sum", []byte("mod v1\n"))
	writeTestFile(t, root, "main.go", []byte("package main\nfunc main() {}\n"))
	writeTestFile(t, root, "main_test.go", []byte("package main\nfunc TestMain() {}\n"))
	writeTestFile(t, root, "README.md", []byte("# Root\n"))
	writeTestFile(t, root, "docs/guide.md", []byte("# Guide\n"))
	writeTestFile(t, root, "docs/spec.txt", []byte("spec\n"))
	writeTestFile(t, root, "pkg/alpha.go", []byte("package pkg\nfunc A() {}\n"))
	writeTestFile(t, root, "pkg/alpha_test.go", []byte("package pkg\nfunc TestA() {}\n"))
	writeTestFile(t, root, "pkg/sub/beta.go", []byte("package sub\nfunc B() {}\n"))
	writeTestFile(t, root, "deep/a/sub/gamma.go", []byte("package sub\nfunc G() {}\n"))
	writeTestFile(t, root, "deep/a/keep.go", []byte("package a\nfunc K() {}\n"))
	writeTestFile(t, root, "exact/drop.go", []byte("package exact\nfunc Drop() {}\n"))
	writeTestFile(t, root, "exact/stay.go", []byte("package exact\nfunc Stay() {}\n"))

	resExact, err := IndexDirectoryWithOptions(root, []string{"exact/drop.go"})
	if err != nil {
		t.Fatalf("exact file exclude error: %v", err)
	}
	if _, ok := resExact.FilesByPath["exact/drop.go"]; ok {
		t.Fatalf("expected exact/drop.go to be excluded")
	}
	if _, ok := resExact.FilesByPath["exact/stay.go"]; !ok {
		t.Fatalf("expected exact/stay.go to be kept")
	}

	resDir, err := IndexDirectoryWithOptions(root, []string{"docs/"})
	if err != nil {
		t.Fatalf("directory exclude error: %v", err)
	}
	if _, ok := resDir.FilesByPath["docs/guide.md"]; ok {
		t.Fatalf("expected docs/guide.md to be excluded")
	}
	if _, ok := resDir.FilesByPath["docs/spec.txt"]; ok {
		t.Fatalf("expected docs/spec.txt to be excluded")
	}
	if _, ok := resDir.FilesByPath["main.go"]; !ok {
		t.Fatalf("expected main.go to be kept")
	}

	resGlob, err := IndexDirectoryWithOptions(root, []string{"*_test.go", "*.md"})
	if err != nil {
		t.Fatalf("glob exclude error: %v", err)
	}
	for _, excluded := range []string{"main_test.go", "pkg/alpha_test.go", "README.md", "docs/guide.md"} {
		if _, ok := resGlob.FilesByPath[excluded]; ok {
			t.Fatalf("expected %s to be excluded by glob", excluded)
		}
	}
	for _, kept := range []string{"main.go", "pkg/alpha.go", "docs/spec.txt"} {
		if _, ok := resGlob.FilesByPath[kept]; !ok {
			t.Fatalf("expected %s to be kept", kept)
		}
	}

	subPattern := "**" + "/sub/" + "**"
	resRecursive, err := IndexDirectoryWithOptions(root, []string{subPattern})
	if err != nil {
		t.Fatalf("recursive exclude error: %v", err)
	}
	if _, ok := resRecursive.FilesByPath["pkg/sub/beta.go"]; ok {
		t.Fatalf("expected pkg/sub/beta.go to be excluded by %s", subPattern)
	}
	if _, ok := resRecursive.FilesByPath["deep/a/sub/gamma.go"]; ok {
		t.Fatalf("expected deep/a/sub/gamma.go to be excluded by %s", subPattern)
	}
	if _, ok := resRecursive.FilesByPath["deep/a/keep.go"]; !ok {
		t.Fatalf("expected deep/a/keep.go to be kept")
	}

	resNeg, err := IndexDirectoryWithOptions(root, []string{"*.md", "!README.md", "!.git", "!go.sum"})
	if err != nil {
		t.Fatalf("negation exclude error: %v", err)
	}
	if _, ok := resNeg.FilesByPath["README.md"]; !ok {
		t.Fatalf("expected !README.md to re-include README.md")
	}
	if _, ok := resNeg.FilesByPath["docs/guide.md"]; ok {
		t.Fatalf("expected docs/guide.md to remain excluded")
	}
	if _, ok := resNeg.FilesByPath[".git/HEAD"]; ok {
		t.Fatalf("negation must never un-exclude hard-skipped .git")
	}
	if _, ok := resNeg.FilesByPath["go.sum"]; ok {
		t.Fatalf("negation must never un-exclude hard-skipped go.sum")
	}
}
