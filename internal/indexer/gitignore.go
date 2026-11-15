package indexer

import (
	"os"
	"path/filepath"
	"strings"
)

var hardSkipDirs = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"build":        {},
	".next":        {},
	"dist":         {},
	"target":       {},
	".gradle":      {},
	".idea":        {},
	"__pycache__":  {},
	".venv":        {},
	"vendor":       {},
	"out":          {},
	".dart_tool":   {},
	".cache":       {},
}

var hardSkipLockfiles = map[string]struct{}{
	"package-lock.json": {},
	"yarn.lock":         {},
	"pnpm-lock.yaml":    {},
	"Cargo.lock":        {},
	"go.sum":            {},
	"poetry.lock":       {},
	"Gemfile.lock":      {},
	"composer.lock":     {},
	"pubspec.lock":      {},
}

type gitignoreRule struct {
	negated  bool
	dirOnly  bool
	anchored bool
	segs     []string
}

type gitignoreScope struct {
	baseRel string
	rules   []gitignoreRule
}

type gitignoreStack []gitignoreScope

func isHardSkipped(name string, isDir bool) bool {
	if isDir {
		_, ok := hardSkipDirs[name]
		return ok
	}
	_, ok := hardSkipLockfiles[name]
	return ok
}

func loadGitignoreScope(dirAbsPath string, baseRel string) (gitignoreScope, bool) {
	data, err := os.ReadFile(filepath.Join(dirAbsPath, ".gitignore"))
	if err != nil {
		return gitignoreScope{}, false
	}
	rules := parseGitignoreContent(string(data))
	if len(rules) == 0 {
		return gitignoreScope{}, false
	}
	return gitignoreScope{
		baseRel: baseRel,
		rules:   rules,
	}, true
}

func parseGitignoreContent(content string) []gitignoreRule {
	rawLines := strings.Split(content, "\n")
	rules := make([]gitignoreRule, 0, len(rawLines))
	for _, line := range rawLines {
		rule, ok := parseGitignoreLine(line)
		if ok {
			rules = append(rules, rule)
		}
	}
	return rules
}

func parseGitignoreLine(line string) (gitignoreRule, bool) {
	line = strings.TrimSuffix(line, "\r")
	line = trimTrailingUnescapedSpaces(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return gitignoreRule{}, false
	}

	negated := false
	if strings.HasPrefix(line, "!") {
		negated = true
		line = line[1:]
	} else if strings.HasPrefix(line, "\\!") || strings.HasPrefix(line, "\\#") {
		line = line[1:]
	}

	if line == "" {
		return gitignoreRule{}, false
	}

	dirOnly := false
	if strings.HasSuffix(line, "/") {
		dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}
	if line == "" {
		return gitignoreRule{}, false
	}

	anchored := false
	if strings.HasPrefix(line, "/") {
		anchored = true
		line = strings.TrimPrefix(line, "/")
	} else if strings.Contains(line, "/") {
		anchored = true
	}
	if line == "" {
		return gitignoreRule{}, false
	}

	segs := strings.Split(line, "/")
	return gitignoreRule{
		negated:  negated,
		dirOnly:  dirOnly,
		anchored: anchored,
		segs:     segs,
	}, true
}

func trimTrailingUnescapedSpaces(s string) string {
	for len(s) > 0 && s[len(s)-1] == ' ' {
		backslashes := 0
		for i := len(s) - 2; i >= 0 && s[i] == '\\'; i-- {
			backslashes++
		}
		if backslashes%2 == 1 {
			break
		}
		s = s[:len(s)-1]
	}
	return strings.ReplaceAll(s, "\\ ", " ")
}

func (stack gitignoreStack) isIgnored(relPath string, isDir bool) bool {
	ignored := false
	for _, scope := range stack {
		subPath := relPath
		if scope.baseRel != "" {
			prefix := scope.baseRel + "/"
			if !strings.HasPrefix(relPath, prefix) {
				continue
			}
			subPath = strings.TrimPrefix(relPath, prefix)
		}
		if subPath == "" {
			continue
		}
		for _, rule := range scope.rules {
			if matchRule(rule, subPath, isDir) {
				ignored = !rule.negated
			}
		}
	}
	return ignored
}

func matchRule(rule gitignoreRule, subPath string, isDir bool) bool {
	if rule.dirOnly && !isDir {
		return false
	}
	pathSegs := strings.Split(subPath, "/")
	if !rule.anchored {
		return matchGlobSegment(rule.segs[0], pathSegs[len(pathSegs)-1])
	}
	return matchPathSegments(rule.segs, pathSegs)
}

func matchPathSegments(patSegs []string, pathSegs []string) bool {
	for len(patSegs) > 0 {
		if patSegs[0] == "**" {
			for len(patSegs) > 1 && patSegs[1] == "**" {
				patSegs = patSegs[1:]
			}
			if len(patSegs) == 1 {
				return len(pathSegs) >= 1
			}
			for i := 0; i <= len(pathSegs); i++ {
				if matchPathSegments(patSegs[1:], pathSegs[i:]) {
					return true
				}
			}
			return false
		}
		if len(pathSegs) == 0 {
			return false
		}
		if !matchGlobSegment(patSegs[0], pathSegs[0]) {
			return false
		}
		patSegs = patSegs[1:]
		pathSegs = pathSegs[1:]
	}
	return len(pathSegs) == 0
}

func matchGlobSegment(pattern string, name string) bool {
	pRunes := []rune(pattern)
	nRunes := []rune(name)
	return matchGlobRunes(pRunes, nRunes)
}

func matchGlobRunes(p []rune, n []rune) bool {
	for len(p) > 0 {
		switch p[0] {
		case '*':
			for len(p) > 1 && p[1] == '*' {
				p = p[1:]
			}
			if len(p) == 1 {
				return true
			}
			for i := 0; i <= len(n); i++ {
				if matchGlobRunes(p[1:], n[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(n) == 0 {
				return false
			}
			p = p[1:]
			n = n[1:]
		case '[':
			if len(n) == 0 {
				return false
			}
			matched, nextPatIdx, ok := matchCharClass(p, n[0])
			if !ok {
				if p[0] != n[0] {
					return false
				}
				p = p[1:]
				n = n[1:]
				continue
			}
			if !matched {
				return false
			}
			p = p[nextPatIdx:]
			n = n[1:]
		case '\\':
			if len(p) > 1 {
				if len(n) == 0 || p[1] != n[0] {
					return false
				}
				p = p[2:]
				n = n[1:]
				continue
			}
			if len(n) == 0 || n[0] != '\\' {
				return false
			}
			p = p[1:]
			n = n[1:]
		default:
			if len(n) == 0 || p[0] != n[0] {
				return false
			}
			p = p[1:]
			n = n[1:]
		}
	}
	return len(n) == 0
}

func matchCharClass(p []rune, ch rune) (bool, int, bool) {
	if len(p) < 2 || p[0] != '[' {
		return false, 0, false
	}
	i := 1
	negate := false
	if p[i] == '!' || p[i] == '^' {
		negate = true
		i++
	}
	matched := false
	hasContent := false
	for i < len(p) {
		if p[i] == ']' && hasContent {
			if negate {
				return !matched, i + 1, true
			}
			return matched, i + 1, true
		}
		hasContent = true
		lo := p[i]
		if lo == '\\' && i+1 < len(p) {
			i++
			lo = p[i]
		}
		if i+2 < len(p) && p[i+1] == '-' && p[i+2] != ']' {
			hi := p[i+2]
			i += 3
			if ch >= lo && ch <= hi {
				matched = true
			}
			continue
		}
		if ch == lo {
			matched = true
		}
		i++
	}
	return false, 0, false
}
