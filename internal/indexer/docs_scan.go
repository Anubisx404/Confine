package indexer

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxSupplementalDocBytes = 512 * 1024
	maxSupplementalDocs     = 4000
)

var extraSkipDocDirs = map[string]struct{}{
	".svn": {},
	".hg":  {},
	"venv": {},
}

var aiToolDirs = map[string]struct{}{
	".agents":   {},
	".agent":    {},
	".claude":   {},
	".cursor":   {},
	".gemini":   {},
	".windsurf": {},
	".codex":    {},
	".kiro":     {},
	".roo":      {},
	".cline":    {},
	".continue": {},
	".aider":    {},
	".copilot":  {},
	".junie":    {},
	".augment":  {},
	".amazonq":  {},
	".opencode": {},
	".goose":    {},
	".omx":      {},
	".specify":  {},
}

var aiGithubSubdirs = map[string]struct{}{
	"instructions": {},
	"prompts":      {},
	"chatmodes":    {},
	"agents":       {},
}

var aiInstructionFiles = map[string]struct{}{
	"agents.md":               {},
	"agent.md":                {},
	"claude.md":               {},
	"claude.local.md":         {},
	"gemini.md":               {},
	"copilot-instructions.md": {},
	"llms.txt":                {},
	"llms-full.txt":           {},
	".goosehints":             {},
}

var aiRuleFiles = map[string]struct{}{
	".cursorrules":   {},
	".windsurfrules": {},
	".clinerules":    {},
	".roorules":      {},
}

var aiConfigFiles = map[string]struct{}{
	".mcp.json":       {},
	".aider.conf.yml": {},
	".aiderignore":    {},
	".cursorignore":   {},
	".geminiignore":   {},
	".codeiumignore":  {},
	".impeccable.md":  {},
}

var aiTextExtensions = map[string]struct{}{
	"":          {},
	".md":       {},
	".mdc":      {},
	".mdx":      {},
	".markdown": {},
	".txt":      {},
	".json":     {},
	".jsonc":    {},
	".yaml":     {},
	".yml":      {},
	".toml":     {},
	".mjs":      {},
	".cjs":      {},
	".js":       {},
	".ts":       {},
	".py":       {},
	".sh":       {},
	".ps1":      {},
	".xml":      {},
}

func IsReadmePath(relPath string) bool {
	base := strings.ToLower(path.Base(filepath.ToSlash(relPath)))
	if base == "" || base == "." {
		return false
	}
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem != "readme" && !strings.HasPrefix(stem, "readme.") {
		return false
	}
	return isDocTextExtension(ext)
}

func IsPlanPath(relPath string) bool {
	slash := strings.ToLower(filepath.ToSlash(relPath))
	base := path.Base(slash)
	if base == "" || base == "." {
		return false
	}
	ext := path.Ext(base)
	if !isDocTextExtension(ext) {
		return false
	}
	stem := strings.TrimSuffix(base, ext)
	if stem == "plan" || stem == "plans" || stem == "roadmap" {
		return true
	}
	if strings.HasSuffix(stem, "_plan") || strings.HasSuffix(stem, "-plan") ||
		strings.HasPrefix(stem, "plan_") || strings.HasPrefix(stem, "plan-") ||
		strings.HasSuffix(stem, "_roadmap") || strings.HasSuffix(stem, "-roadmap") {
		return true
	}
	dir := path.Dir(slash)
	if dir != "." {
		for _, seg := range strings.Split(dir, "/") {
			if seg == "plans" || seg == ".plans" || seg == ".planning" || seg == "roadmap" {
				return true
			}
		}
	}
	return false
}

func IsAIPath(relPath string) bool {
	slash := strings.ToLower(filepath.ToSlash(relPath))
	base := path.Base(slash)
	if base == "" || base == "." {
		return false
	}
	if _, ok := aiInstructionFiles[base]; ok {
		return true
	}
	if _, ok := aiRuleFiles[base]; ok {
		return true
	}
	if _, ok := aiConfigFiles[base]; ok {
		return true
	}
	if base == "skill.md" || strings.HasSuffix(base, ".instructions.md") || strings.HasSuffix(base, ".prompt.md") || strings.HasSuffix(base, ".chatmode.md") {
		return true
	}
	if _, ok := aiTextExtensions[path.Ext(base)]; !ok {
		return false
	}
	segs := strings.Split(path.Dir(slash), "/")
	for i, seg := range segs {
		if _, ok := aiToolDirs[seg]; ok {
			return true
		}
		if seg == ".github" && i+1 < len(segs) {
			if _, ok := aiGithubSubdirs[segs[i+1]]; ok {
				return true
			}
		}
	}
	return false
}

func ClassifyAIKind(relPath string) string {
	slash := strings.ToLower(filepath.ToSlash(relPath))
	base := path.Base(slash)
	segs := strings.Split(path.Dir(slash), "/")
	hasSeg := func(names ...string) bool {
		for _, seg := range segs {
			for _, n := range names {
				if seg == n {
					return true
				}
			}
		}
		return false
	}

	switch {
	case base == "skill.md" || hasSeg("skills"):
		return "skill"
	case strings.Contains(base, "mcp"):
		return "mcp"
	case base == "hooks.json" || hasSeg("hooks"):
		return "hook"
	case hasSeg("workflows"):
		return "workflow"
	case hasSeg("commands"):
		return "command"
	case strings.HasSuffix(base, ".prompt.md") || strings.HasSuffix(base, ".chatmode.md") || hasSeg("prompts", "chatmodes"):
		return "prompt"
	case hasSeg("agents", "subagents"):
		return "agent"
	case path.Ext(base) == ".mdc" || hasSeg("rules"):
		return "rule"
	}
	if _, ok := aiRuleFiles[base]; ok {
		return "rule"
	}
	if _, ok := aiInstructionFiles[base]; ok || strings.HasSuffix(base, ".instructions.md") || hasSeg("instructions") {
		return "instructions"
	}
	return "config"
}

func isDocTextExtension(ext string) bool {
	switch ext {
	case "", ".md", ".mdx", ".markdown", ".txt", ".rst", ".adoc":
		return true
	default:
		return false
	}
}

func ScanProjectDocs(rootPath string, excludes []string) ([]*Node, error) {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}
	matcher := newExcludeMatcher(excludes)
	var docs []*Node

	walkErr := filepath.WalkDir(absRoot, func(fullPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if fullPath == absRoot {
			return nil
		}
		if len(docs) >= maxSupplementalDocs {
			return filepath.SkipAll
		}

		name := d.Name()
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		isDir := d.IsDir()
		if isHardSkipped(name, isDir) {
			if isDir {
				return filepath.SkipDir
			}
			return nil
		}
		if isDir {
			if _, skip := extraSkipDocDirs[name]; skip {
				return filepath.SkipDir
			}
		}

		relOS, relErr := filepath.Rel(absRoot, fullPath)
		if relErr != nil {
			return nil
		}
		relSlash := filepath.ToSlash(relOS)

		if matcher.isExcluded(relSlash, name, isDir) {
			if isDir {
				return filepath.SkipDir
			}
			return nil
		}

		if isDir {
			return nil
		}

		isAI := IsAIPath(relSlash)
		if !isAI && !IsReadmePath(relSlash) && !IsPlanPath(relSlash) {
			return nil
		}
		if isKnownBinaryExtension(name) {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil || info.Size() > maxSupplementalDocBytes {
			return nil
		}

		content, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			return nil
		}
		lang, isBinary := DetectLanguage(name, content)
		if isBinary {
			return nil
		}
		if lang == "Text" && !isAI {
			lang = "Markdown"
		}

		totalLines, sloc, lines := CountLines(lang, content)
		docs = append(docs, &Node{
			Name:    name,
			RelPath: relSlash,
			IsDir:   false,
			Stats: FileStats{
				ByteSize:        int64(len(content)),
				LineCount:       totalLines,
				SourceLineCount: sloc,
				Language:        lang,
				IsMarkdown:      lang == "Markdown",
			},
			Analysis: emptyAnalysis(),
			Lines:    lines,
		})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].RelPath < docs[j].RelPath
	})
	return docs, nil
}
