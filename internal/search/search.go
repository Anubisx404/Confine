package search

import (
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"confine/internal/indexer"
	"confine/internal/treemap"
)

const (
	defaultSearchLimit = 200
	maxSearchLimit     = 1000
	parallelThreshold  = 500
	maxSnippetRunes    = 160
	bloomWords         = 8
)

type Match struct {
	RelPath string       `json:"relPath"`
	Line    int          `json:"line"`
	Column  int          `json:"column"`
	Snippet string       `json:"snippet"`
	Bounds  treemap.Rect `json:"bounds"`
	Z       float64      `json:"z"`
}

type QueryResult struct {
	Query        string         `json:"query"`
	TotalMatches int            `json:"totalMatches"`
	ElapsedMs    float64        `json:"elapsedMs"`
	FileHits     map[string]int `json:"fileHits"`
	Matches      []Match        `json:"matches"`
}

type MarkdownEntry struct {
	Name      string       `json:"name"`
	RelPath   string       `json:"relPath"`
	Title     string       `json:"title"`
	ByteSize  int64        `json:"byteSize"`
	LineCount int          `json:"lineCount"`
	Bounds    treemap.Rect `json:"bounds"`
	Z         float64      `json:"z"`
}

type fileEntry struct {
	relPath      string
	rawLines     []string
	lowerLines   []string
	lowerContent string
	trigramMask  uint64
	bloom        [bloomWords]uint64
	bounds       treemap.Rect
	z            float64
}

type Index struct {
	mu        sync.RWMutex
	files     []fileEntry
	markdowns []MarkdownEntry
}

type fileHit struct {
	relPath string
	count   int
}

type chunkResult struct {
	total   int
	hits    []fileHit
	matches []Match
}

func NewIndex(tree *indexer.TreeResult, layout *treemap.LayoutResult) *Index {
	nodes := collectNodes(tree)
	layoutByPath := resolveLayoutMap(layout)

	files := make([]fileEntry, 0, len(nodes))
	markdowns := make([]MarkdownEntry, 0)

	for _, node := range nodes {
		if node == nil || node.IsDir {
			continue
		}

		var bounds treemap.Rect
		var z float64
		if ln, ok := layoutByPath[node.RelPath]; ok {
			bounds = ln.Bounds
			z = ln.Z
		}

		entry := buildFileEntry(node.RelPath, node.Lines, bounds, z)
		files = append(files, entry)

		if node.Stats.IsMarkdown {
			name := node.Name
			if name == "" {
				name = path.Base(node.RelPath)
			}
			markdowns = append(markdowns, MarkdownEntry{
				Name:      name,
				RelPath:   node.RelPath,
				Title:     extractMarkdownTitle(node.Lines, name),
				ByteSize:  node.Stats.ByteSize,
				LineCount: node.Stats.LineCount,
				Bounds:    bounds,
				Z:         z,
			})
		}
	}

	sortMarkdownEntries(markdowns)

	return &Index{
		files:     files,
		markdowns: markdowns,
	}
}

func collectNodes(tree *indexer.TreeResult) []*indexer.Node {
	if tree == nil {
		return nil
	}
	if len(tree.FlatNodes) > 0 {
		return tree.FlatNodes
	}
	if tree.Root != nil {
		var out []*indexer.Node
		var walk func(n *indexer.Node)
		walk = func(n *indexer.Node) {
			if n == nil {
				return
			}
			out = append(out, n)
			for _, child := range n.Children {
				walk(child)
			}
		}
		walk(tree.Root)
		return out
	}
	if len(tree.FilesByPath) > 0 {
		keys := make([]string, 0, len(tree.FilesByPath))
		for k := range tree.FilesByPath {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]*indexer.Node, 0, len(keys))
		for _, k := range keys {
			out = append(out, tree.FilesByPath[k])
		}
		return out
	}
	return nil
}

func resolveLayoutMap(layout *treemap.LayoutResult) map[string]treemap.LayoutNode {
	if layout == nil {
		return nil
	}
	if len(layout.ByPath) > 0 {
		return layout.ByPath
	}
	if len(layout.Nodes) == 0 {
		return nil
	}
	m := make(map[string]treemap.LayoutNode, len(layout.Nodes))
	for _, n := range layout.Nodes {
		m[n.RelPath] = n
	}
	return m
}

func buildFileEntry(relPath string, rawLines []string, bounds treemap.Rect, z float64) fileEntry {
	lowerLines := make([]string, len(rawLines))
	totalBytes := 0
	var mask uint64
	var bloom [bloomWords]uint64

	for i, line := range rawLines {
		lower := strings.ToLower(line)
		lowerLines[i] = lower
		totalBytes += len(lower)
		if i > 0 {
			totalBytes++
		}
		if len(lower) >= 3 {
			m, b := computeTrigrams(lower)
			mask |= m
			for w := 0; w < bloomWords; w++ {
				bloom[w] |= b[w]
			}
		}
	}

	var sb strings.Builder
	sb.Grow(totalBytes)
	for i, lower := range lowerLines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(lower)
	}

	return fileEntry{
		relPath:      relPath,
		rawLines:     rawLines,
		lowerLines:   lowerLines,
		lowerContent: sb.String(),
		trigramMask:  mask,
		bloom:        bloom,
		bounds:       bounds,
		z:            z,
	}
}

func computeTrigrams(s string) (uint64, [bloomWords]uint64) {
	var mask uint64
	var bloom [bloomWords]uint64
	for i := 0; i <= len(s)-3; i++ {
		h := uint64(s[i])*1315423911 ^ uint64(s[i+1])*2654435761 ^ uint64(s[i+2])*97531
		h ^= h >> 17
		mask |= 1 << (h & 63)
		bit := (h >> 6) & 511
		bloom[bit>>6] |= 1 << (bit & 63)
	}
	return mask, bloom
}

func (idx *Index) UpdateLayout(layout *treemap.LayoutResult) {
	if idx == nil {
		return
	}
	layoutByPath := resolveLayoutMap(layout)

	idx.mu.Lock()
	defer idx.mu.Unlock()

	for i := range idx.files {
		if ln, ok := layoutByPath[idx.files[i].relPath]; ok {
			idx.files[i].bounds = ln.Bounds
			idx.files[i].z = ln.Z
		} else {
			idx.files[i].bounds = treemap.Rect{}
			idx.files[i].z = 0
		}
	}

	for i := range idx.markdowns {
		if ln, ok := layoutByPath[idx.markdowns[i].RelPath]; ok {
			idx.markdowns[i].Bounds = ln.Bounds
			idx.markdowns[i].Z = ln.Z
		} else {
			idx.markdowns[i].Bounds = treemap.Rect{}
			idx.markdowns[i].Z = 0
		}
	}
}

func (idx *Index) Markdowns() []MarkdownEntry {
	if idx == nil {
		return []MarkdownEntry{}
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if len(idx.markdowns) == 0 {
		return []MarkdownEntry{}
	}
	out := make([]MarkdownEntry, len(idx.markdowns))
	copy(out, idx.markdowns)
	return out
}

func (idx *Index) Search(query string, limit int) QueryResult {
	start := time.Now()

	if limit <= 0 {
		limit = defaultSearchLimit
	} else if limit > maxSearchLimit {
		limit = maxSearchLimit
	}

	trimmed := strings.TrimSpace(query)
	if idx == nil || trimmed == "" {
		return QueryResult{
			Query:        trimmed,
			TotalMatches: 0,
			ElapsedMs:    elapsedMs(start),
			FileHits:     map[string]int{},
			Matches:      []Match{},
		}
	}

	lowerQuery := strings.ToLower(trimmed)
	if strings.ContainsRune(lowerQuery, '\n') {
		return QueryResult{
			Query:        trimmed,
			TotalMatches: 0,
			ElapsedMs:    elapsedMs(start),
			FileHits:     map[string]int{},
			Matches:      []Match{},
		}
	}

	useMask := len(lowerQuery) >= 3
	var queryMask uint64
	var queryBloom [bloomWords]uint64
	var activeWords [bloomWords]int
	activeCount := 0
	if useMask {
		queryMask, queryBloom = computeTrigrams(lowerQuery)
		for w := 0; w < bloomWords; w++ {
			if queryBloom[w] != 0 {
				activeWords[activeCount] = w
				activeCount++
			}
		}
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	n := len(idx.files)
	if n == 0 {
		return QueryResult{
			Query:        trimmed,
			TotalMatches: 0,
			ElapsedMs:    elapsedMs(start),
			FileHits:     map[string]int{},
			Matches:      []Match{},
		}
	}

	if n <= parallelThreshold {
		res := searchChunk(idx.files, lowerQuery, useMask, queryMask, queryBloom, activeWords, activeCount, limit)
		fileHits := make(map[string]int, len(res.hits))
		for _, h := range res.hits {
			fileHits[h.relPath] = h.count
		}
		matches := res.matches
		if matches == nil {
			matches = []Match{}
		}
		return QueryResult{
			Query:        trimmed,
			TotalMatches: res.total,
			ElapsedMs:    elapsedMs(start),
			FileHits:     fileHits,
			Matches:      matches,
		}
	}

	workers := runtime.NumCPU()
	if workers < 2 {
		workers = 2
	}
	if workers > n {
		workers = n
	}

	results := make([]chunkResult, workers)
	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		begin := (w * n) / workers
		end := ((w + 1) * n) / workers
		go func(slot int, slice []fileEntry) {
			defer wg.Done()
			results[slot] = searchChunk(slice, lowerQuery, useMask, queryMask, queryBloom, activeWords, activeCount, limit)
		}(w, idx.files[begin:end])
	}
	wg.Wait()

	totalMatches := 0
	totalHitFiles := 0
	for i := range results {
		totalMatches += results[i].total
		totalHitFiles += len(results[i].hits)
	}

	fileHits := make(map[string]int, totalHitFiles)
	matches := make([]Match, 0, minInt(limit, totalMatches))

	for i := range results {
		for _, h := range results[i].hits {
			fileHits[h.relPath] = h.count
		}
		if len(matches) < limit && len(results[i].matches) > 0 {
			need := limit - len(matches)
			if len(results[i].matches) <= need {
				matches = append(matches, results[i].matches...)
			} else {
				matches = append(matches, results[i].matches[:need]...)
			}
		}
	}

	return QueryResult{
		Query:        trimmed,
		TotalMatches: totalMatches,
		ElapsedMs:    elapsedMs(start),
		FileHits:     fileHits,
		Matches:      matches,
	}
}

func searchChunk(
	entries []fileEntry,
	lowerQuery string,
	useMask bool,
	queryMask uint64,
	queryBloom [bloomWords]uint64,
	activeWords [bloomWords]int,
	activeCount int,
	limit int,
) chunkResult {
	var out chunkResult
	qLen := len(lowerQuery)

	for i := range entries {
		entry := &entries[i]
		if useMask {
			if (entry.trigramMask & queryMask) != queryMask {
				continue
			}
			matchedBloom := true
			for k := 0; k < activeCount; k++ {
				w := activeWords[k]
				if (entry.bloom[w] & queryBloom[w]) != queryBloom[w] {
					matchedBloom = false
					break
				}
			}
			if !matchedBloom {
				continue
			}
		}

		if !strings.Contains(entry.lowerContent, lowerQuery) {
			continue
		}

		if len(out.matches) >= limit {
			cnt := strings.Count(entry.lowerContent, lowerQuery)
			if cnt > 0 {
				out.total += cnt
				out.hits = append(out.hits, fileHit{relPath: entry.relPath, count: cnt})
			}
			continue
		}

		fileCount := 0
		for lineIdx, lowerLine := range entry.lowerLines {
			if !strings.Contains(lowerLine, lowerQuery) {
				continue
			}
			pos := 0
			for {
				relIdx := strings.Index(lowerLine[pos:], lowerQuery)
				if relIdx < 0 {
					break
				}
				matchByte := pos + relIdx
				fileCount++
				if len(out.matches) < limit {
					col := utf8.RuneCountInString(lowerLine[:matchByte]) + 1
					rawLine := entry.rawLines[lineIdx]
					rawStart, rawEnd := mapMatchByteRange(rawLine, lowerLine, matchByte, matchByte+qLen)
					snippet := buildSnippet(rawLine, rawStart, rawEnd)
					out.matches = append(out.matches, Match{
						RelPath: entry.relPath,
						Line:    lineIdx + 1,
						Column:  col,
						Snippet: snippet,
						Bounds:  entry.bounds,
						Z:       entry.z,
					})
				}
				pos = matchByte + qLen
			}
		}

		if fileCount > 0 {
			out.total += fileCount
			out.hits = append(out.hits, fileHit{relPath: entry.relPath, count: fileCount})
		}
	}

	return out
}

func mapMatchByteRange(rawLine, lowerLine string, lowerStart, lowerEnd int) (int, int) {
	if len(rawLine) == len(lowerLine) {
		return lowerStart, lowerEnd
	}
	targetStartRune := utf8.RuneCountInString(lowerLine[:lowerStart])
	targetEndRune := targetStartRune + utf8.RuneCountInString(lowerLine[lowerStart:lowerEnd])

	runeIdx := 0
	rawStart := len(rawLine)
	rawEnd := len(rawLine)
	for byteIdx := range rawLine {
		if runeIdx == targetStartRune {
			rawStart = byteIdx
		}
		if runeIdx == targetEndRune {
			rawEnd = byteIdx
			break
		}
		runeIdx++
	}
	if rawEnd < rawStart {
		rawEnd = rawStart
	}
	return rawStart, rawEnd
}

func buildSnippet(rawLine string, matchStart, matchEnd int) string {
	if len(rawLine) <= maxSnippetRunes {
		return strings.TrimSpace(rawLine)
	}

	if matchStart < 0 {
		matchStart = 0
	}
	if matchStart > len(rawLine) {
		matchStart = len(rawLine)
	}
	if matchEnd < matchStart {
		matchEnd = matchStart
	}
	if matchEnd > len(rawLine) {
		matchEnd = len(rawLine)
	}

	winByteStart := matchStart - 400
	if winByteStart < 0 {
		winByteStart = 0
	}
	for winByteStart > 0 && winByteStart < len(rawLine) && !utf8.RuneStart(rawLine[winByteStart]) {
		winByteStart++
	}

	winByteEnd := matchEnd + 400
	if winByteEnd > len(rawLine) {
		winByteEnd = len(rawLine)
	}
	for winByteEnd < len(rawLine) && !utf8.RuneStart(rawLine[winByteEnd]) {
		winByteEnd++
	}

	clippedLeft := winByteStart > 0
	clippedRight := winByteEnd < len(rawLine)

	sub := rawLine[winByteStart:winByteEnd]
	if !clippedLeft && !clippedRight {
		trimmed := strings.TrimSpace(sub)
		if utf8.RuneCountInString(trimmed) <= maxSnippetRunes {
			return trimmed
		}
	}

	runes := []rune(sub)
	matchRuneStart := utf8.RuneCountInString(rawLine[winByteStart:matchStart])
	matchRuneLen := utf8.RuneCountInString(rawLine[matchStart:matchEnd])
	matchRuneEnd := matchRuneStart + matchRuneLen

	center := (matchRuneStart + matchRuneEnd) / 2
	start := center - maxSnippetRunes/2
	if start < 0 {
		start = 0
	}
	end := start + maxSnippetRunes
	if end > len(runes) {
		end = len(runes)
		start = end - maxSnippetRunes
		if start < 0 {
			start = 0
		}
	}
	if matchRuneStart < start {
		start = matchRuneStart
		end = start + maxSnippetRunes
		if end > len(runes) {
			end = len(runes)
		}
	}

	needLeft := clippedLeft || start > 0
	needRight := clippedRight || end < len(runes)

	if needLeft && end-start > 6 && start+3 <= matchRuneStart {
		start += 3
	}
	if needRight && end-start > 6 {
		end -= 3
	}

	piece := strings.TrimSpace(string(runes[start:end]))
	if needLeft {
		piece = "..." + piece
	}
	if needRight {
		piece = piece + "..."
	}
	return piece
}

func extractMarkdownTitle(lines []string, fallback string) string {
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			title := strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			if title != "" {
				return title
			}
		}
	}
	return fallback
}

func sortMarkdownEntries(entries []MarkdownEntry) {
	sort.Slice(entries, func(i, j int) bool {
		ri := canonicalDocRank(entries[i].RelPath)
		rj := canonicalDocRank(entries[j].RelPath)
		if ri != rj {
			return ri < rj
		}
		return entries[i].RelPath < entries[j].RelPath
	})
}

func canonicalDocRank(relPath string) int {
	if strings.Contains(relPath, "/") {
		return 6
	}
	switch {
	case strings.EqualFold(relPath, "README.md"):
		return 0
	case strings.EqualFold(relPath, "PLAN.md"):
		return 1
	case strings.EqualFold(relPath, "SPEC.md"):
		return 2
	case strings.EqualFold(relPath, "ARCHITECTURE.md"):
		return 3
	case strings.EqualFold(relPath, "CHANGELOG.md"):
		return 4
	case strings.EqualFold(relPath, "CONTRIBUTING.md"):
		return 5
	default:
		return 6
	}
}

func elapsedMs(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000.0
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
