package indexer

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var extraSkipDocDirs = map[string]struct{}{
	".svn": {},
	".hg":  {},
	"venv": {},
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

func isDocTextExtension(ext string) bool {
	switch ext {
	case "", ".md", ".mdx", ".markdown", ".txt", ".rst", ".adoc":
		return true
	default:
		return false
	}
}

func ScanProjectReadmesAndPlans(rootPath string, excludes []string) ([]*Node, error) {
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

		name := d.Name()
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
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

		if !IsReadmePath(relSlash) && !IsPlanPath(relSlash) {
			return nil
		}
		if isKnownBinaryExtension(name) {
			return nil
		}

		content, readErr := os.ReadFile(fullPath)
		if readErr != nil || hasNulByte(content) {
			return nil
		}

		totalLines, sloc, lines := CountLines("Markdown", content)
		docs = append(docs, &Node{
			Name:    name,
			RelPath: relSlash,
			IsDir:   false,
			Stats: FileStats{
				ByteSize:        int64(len(content)),
				LineCount:       totalLines,
				SourceLineCount: sloc,
				Language:        "Markdown",
				IsMarkdown:      true,
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
