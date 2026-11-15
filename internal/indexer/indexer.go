package indexer

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"confine/internal/analyzer"
)

type FileStats struct {
	ByteSize        int64  `json:"byteSize"`
	LineCount       int    `json:"lineCount"`
	SourceLineCount int    `json:"sourceLineCount"`
	Language        string `json:"language"`
	ChildrenCount   int    `json:"childrenCount"`
	DescendantCount int    `json:"descendantCount"`
	IsMarkdown      bool   `json:"isMarkdown"`
	SymbolCount     int    `json:"symbolCount"`
	ImportCount     int    `json:"importCount"`
}

type Node struct {
	ID       uint32                  `json:"id"`
	ParentID uint32                  `json:"parentId"`
	Name     string                  `json:"name"`
	RelPath  string                  `json:"relPath"`
	IsDir    bool                    `json:"isDir"`
	Depth    int                     `json:"depth"`
	Stats    FileStats               `json:"stats"`
	Analysis analyzer.AnalysisResult `json:"analysis"`
	Lines    []string                `json:"-"`
	Children []*Node                 `json:"children,omitempty"`
}

type TreeResult struct {
	RootPath    string           `json:"rootPath"`
	ProjectName string           `json:"projectName"`
	Root        *Node            `json:"root"`
	FlatNodes   []*Node          `json:"-"`
	FilesByPath map[string]*Node `json:"-"`
	TotalFiles  int              `json:"totalFiles"`
	TotalDirs   int              `json:"totalDirs"`
	TotalLines  int              `json:"totalLines"`
	TotalSLOC   int              `json:"totalSloc"`
	TotalBytes  int64            `json:"totalBytes"`
}

type fileTask struct {
	node     *Node
	fullPath string
}

func emptyAnalysis() analyzer.AnalysisResult {
	return analyzer.AnalysisResult{
		Symbols:  []analyzer.Symbol{},
		Imports:  []analyzer.ImportRef{},
		TopCalls: []analyzer.CallMetric{},
	}
}

type excludeEntry struct {
	pattern string
	trimmed string
	prefix  string
	rule    gitignoreRule
	hasRule bool
	negated bool
}

type excludeMatcher struct {
	entries []excludeEntry
}

func NormalizeExcludes(excludes []string) []string {
	if len(excludes) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(excludes))
	seen := make(map[string]struct{}, len(excludes))
	for _, raw := range excludes {
		p := strings.TrimSpace(raw)
		p = strings.ReplaceAll(p, "\\", "/")
		for {
			if strings.HasPrefix(p, "./") {
				p = p[2:]
				continue
			}
			if strings.HasPrefix(p, "/") {
				p = p[1:]
				continue
			}
			break
		}
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func parseGitignoreLines(lines []string) []gitignoreRule {
	rules := make([]gitignoreRule, 0, len(lines))
	for _, line := range lines {
		if rule, ok := parseGitignoreLine(line); ok {
			rules = append(rules, rule)
		}
	}
	return rules
}

func newExcludeMatcher(excludes []string) excludeMatcher {
	norm := NormalizeExcludes(excludes)
	if len(norm) == 0 {
		return excludeMatcher{}
	}
	entries := make([]excludeEntry, 0, len(norm))
	for _, pat := range norm {
		rules := parseGitignoreLines([]string{pat})
		var rule gitignoreRule
		hasRule := len(rules) > 0
		if hasRule {
			rule = rules[0]
		}
		trimmed := strings.TrimSuffix(pat, "/")
		prefix := ""
		if trimmed != "" {
			prefix = trimmed + "/"
		}
		entries = append(entries, excludeEntry{
			pattern: pat,
			trimmed: trimmed,
			prefix:  prefix,
			rule:    rule,
			hasRule: hasRule,
			negated: hasRule && rule.negated,
		})
	}
	return excludeMatcher{entries: entries}
}

func (m excludeMatcher) isExcluded(relPath string, name string, isDir bool) bool {
	if len(m.entries) == 0 {
		return false
	}
	excluded := false
	for _, entry := range m.entries {
		if entry.negated {
			if entry.hasRule && matchRule(entry.rule, relPath, isDir) {
				excluded = false
			}
			continue
		}
		if matchesExcludeEntry(entry, relPath, name, isDir) {
			excluded = true
		}
	}
	return excluded
}

func matchesExcludeEntry(entry excludeEntry, relPath string, name string, isDir bool) bool {
	pat := entry.pattern
	trimmed := entry.trimmed
	if relPath == pat || (trimmed != "" && relPath == trimmed) {
		return true
	}
	if entry.prefix != "" && strings.HasPrefix(relPath, entry.prefix) {
		return true
	}
	if name == pat || (trimmed != "" && name == trimmed) {
		return true
	}
	if ok, err := path.Match(pat, name); err == nil && ok {
		return true
	}
	if ok, err := path.Match(pat, relPath); err == nil && ok {
		return true
	}
	if entry.hasRule && matchRule(entry.rule, relPath, isDir) {
		return true
	}
	return false
}

func IndexDirectory(rootPath string) (*TreeResult, error) {
	return IndexDirectoryWithOptions(rootPath, nil)
}

func IndexDirectoryWithOptions(rootPath string, excludes []string) (*TreeResult, error) {
	if strings.TrimSpace(rootPath) == "" {
		return nil, fmt.Errorf("root path cannot be empty")
	}
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolve root path: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat root path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root path is not a directory: %s", absRoot)
	}

	projectName := filepath.Base(absRoot)
	if projectName == "" || projectName == "." || projectName == string(filepath.Separator) {
		projectName = absRoot
	}

	rootNode := &Node{
		Name:     projectName,
		RelPath:  "",
		IsDir:    true,
		Depth:    0,
		Analysis: emptyAnalysis(),
	}

	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}

	matcher := newExcludeMatcher(excludes)
	taskCh := make(chan fileTask, workers*4)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskCh {
				processFileTask(task)
			}
		}()
	}

	walkErr := walkDirTree(absRoot, rootNode, nil, matcher, taskCh, true)
	close(taskCh)
	wg.Wait()

	if walkErr != nil {
		return nil, walkErr
	}

	pruneAndAggregate(rootNode)

	result := &TreeResult{
		RootPath:    absRoot,
		ProjectName: projectName,
		Root:        rootNode,
		FlatNodes:   make([]*Node, 0),
		FilesByPath: make(map[string]*Node),
	}
	assignIDsAndBuildIndexes(rootNode, 0, result)
	return result, nil
}

func walkDirTree(dirAbsPath string, dirNode *Node, scopes gitignoreStack, matcher excludeMatcher, taskCh chan<- fileTask, isRoot bool) error {
	entries, err := os.ReadDir(dirAbsPath)
	if err != nil {
		if isRoot {
			return fmt.Errorf("read root directory: %w", err)
		}
		return nil
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	activeScopes := scopes
	for _, entry := range entries {
		if entry.Name() == ".gitignore" && !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			if scope, ok := loadGitignoreScope(dirAbsPath, dirNode.RelPath); ok {
				activeScopes = append(scopes[:len(scopes):len(scopes)], scope)
			}
			break
		}
	}

	children := make([]*Node, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		mode := entry.Type()
		if mode&os.ModeSymlink != 0 {
			continue
		}

		isDir := entry.IsDir()
		if !isDir && !mode.IsRegular() {
			continue
		}

		if isHardSkipped(name, isDir) {
			continue
		}

		relPath := name
		if dirNode.RelPath != "" {
			relPath = filepath.ToSlash(dirNode.RelPath + "/" + name)
		} else {
			relPath = filepath.ToSlash(name)
		}

		if activeScopes.isIgnored(relPath, isDir) {
			continue
		}

		if matcher.isExcluded(relPath, name, isDir) {
			continue
		}

		fullPath := filepath.Join(dirAbsPath, name)
		if isDir {
			childDir := &Node{
				Name:     name,
				RelPath:  relPath,
				IsDir:    true,
				Depth:    dirNode.Depth + 1,
				Analysis: emptyAnalysis(),
			}
			_ = walkDirTree(fullPath, childDir, activeScopes, matcher, taskCh, false)
			children = append(children, childDir)
			continue
		}

		if isKnownBinaryExtension(name) {
			continue
		}

		childFile := &Node{
			Name:     name,
			RelPath:  relPath,
			IsDir:    false,
			Depth:    dirNode.Depth + 1,
			Analysis: emptyAnalysis(),
		}
		children = append(children, childFile)
		taskCh <- fileTask{
			node:     childFile,
			fullPath: fullPath,
		}
	}

	dirNode.Children = children
	return nil
}

func processFileTask(task fileTask) {
	content, err := os.ReadFile(task.fullPath)
	if err != nil {
		return
	}

	lang, isBinary := DetectLanguage(task.node.Name, content)
	if isBinary {
		return
	}

	totalLines, sloc, lines := CountLines(lang, content)
	analysis := analyzer.Analyze(lang, string(content))

	task.node.Lines = lines
	task.node.Analysis = analysis
	task.node.Stats = FileStats{
		ByteSize:        int64(len(content)),
		LineCount:       totalLines,
		SourceLineCount: sloc,
		Language:        lang,
		IsMarkdown:      lang == "Markdown",
		SymbolCount:     len(analysis.Symbols),
		ImportCount:     len(analysis.Imports),
	}
}

func pruneAndAggregate(node *Node) bool {
	if !node.IsDir {
		return node.Stats.Language != ""
	}

	kept := make([]*Node, 0, len(node.Children))
	var byteSize int64
	var lineCount, slocCount, symbolCount, importCount, descCount int

	for _, child := range node.Children {
		if !pruneAndAggregate(child) {
			continue
		}
		kept = append(kept, child)
		byteSize += child.Stats.ByteSize
		lineCount += child.Stats.LineCount
		slocCount += child.Stats.SourceLineCount
		symbolCount += child.Stats.SymbolCount
		importCount += child.Stats.ImportCount
		descCount += 1 + child.Stats.DescendantCount
	}

	node.Children = kept
	node.Stats = FileStats{
		ByteSize:        byteSize,
		LineCount:       lineCount,
		SourceLineCount: slocCount,
		ChildrenCount:   len(kept),
		DescendantCount: descCount,
		SymbolCount:     symbolCount,
		ImportCount:     importCount,
	}

	if node.Depth > 0 && len(kept) == 0 {
		return false
	}
	return true
}

func assignIDsAndBuildIndexes(node *Node, parentID uint32, result *TreeResult) {
	node.ID = uint32(len(result.FlatNodes))
	node.ParentID = parentID
	result.FlatNodes = append(result.FlatNodes, node)

	if node.IsDir {
		result.TotalDirs++
		for _, child := range node.Children {
			assignIDsAndBuildIndexes(child, node.ID, result)
		}
		return
	}

	result.TotalFiles++
	result.FilesByPath[node.RelPath] = node
	result.TotalLines += node.Stats.LineCount
	result.TotalSLOC += node.Stats.SourceLineCount
	result.TotalBytes += node.Stats.ByteSize
}
