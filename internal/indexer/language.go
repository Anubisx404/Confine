package indexer

import (
	"bytes"
	"path/filepath"
	"strings"
)

var binaryExtensions = map[string]struct{}{
	".png":   {},
	".jpg":   {},
	".jpeg":  {},
	".gif":   {},
	".webp":  {},
	".ico":   {},
	".pdf":   {},
	".zip":   {},
	".tar":   {},
	".gz":    {},
	".ipa":   {},
	".apk":   {},
	".exe":   {},
	".dll":   {},
	".so":    {},
	".dylib": {},
	".wasm":  {},
	".ttf":   {},
	".otf":   {},
	".woff":  {},
	".woff2": {},
	".mp3":   {},
	".mp4":   {},
	".7z":    {},
	".bin":   {},
	".o":     {},
	".a":     {},
}

var extensionLanguageMap = map[string]string{
	".go":       "Go",
	".rs":       "Rust",
	".c":        "C",
	".h":        "C",
	".cpp":      "C++",
	".cc":       "C++",
	".cxx":      "C++",
	".hpp":      "C++",
	".hh":       "C++",
	".hxx":      "C++",
	".cs":       "C#",
	".java":     "Java",
	".dart":     "Dart",
	".ts":       "TypeScript",
	".tsx":      "TypeScript",
	".mts":      "TypeScript",
	".cts":      "TypeScript",
	".js":       "JavaScript",
	".jsx":      "JavaScript",
	".mjs":      "JavaScript",
	".cjs":      "JavaScript",
	".py":       "Python",
	".pyw":      "Python",
	".pyi":      "Python",
	".html":     "HTML",
	".htm":      "HTML",
	".css":      "CSS",
	".scss":     "CSS",
	".sass":     "CSS",
	".less":     "CSS",
	".json":     "JSON",
	".jsonc":    "JSON",
	".arb":      "JSON",
	".yaml":     "YAML",
	".yml":      "YAML",
	".md":       "Markdown",
	".mdx":      "Markdown",
	".markdown": "Markdown",
	".sql":      "SQL",
	".sh":       "Shell",
	".bash":     "Shell",
	".zsh":      "Shell",
	".fish":     "Shell",
	".ps1":      "Shell",
	".toml":     "TOML",
	".xml":      "XML",
	".svg":      "XML",
	".plist":    "XML",
	".csproj":   "XML",
	".xaml":     "XML",
}

func isKnownBinaryExtension(fileName string) bool {
	ext := strings.ToLower(filepath.Ext(fileName))
	_, ok := binaryExtensions[ext]
	return ok
}

func hasNulByte(content []byte) bool {
	limit := len(content)
	if limit > 8192 {
		limit = 8192
	}
	return bytes.IndexByte(content[:limit], 0) >= 0
}

func DetectLanguage(fileName string, content []byte) (string, bool) {
	if isKnownBinaryExtension(fileName) || hasNulByte(content) {
		return "", true
	}

	base := strings.ToLower(filepath.Base(fileName))
	if base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") {
		return "Dockerfile", false
	}
	if base == "makefile" || base == "gnumakefile" {
		return "Makefile", false
	}

	ext := strings.ToLower(filepath.Ext(fileName))
	if lang, ok := extensionLanguageMap[ext]; ok {
		return lang, false
	}
	return "Text", false
}

func CountLines(language string, content []byte) (int, int, []string) {
	if len(content) == 0 {
		return 0, 0, []string{}
	}

	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	lines := strings.Split(text, "\n")
	totalLines := len(lines)
	sloc := countSourceLines(language, lines)
	return totalLines, sloc, lines
}

func countSourceLines(language string, lines []string) int {
	lineSlash := "/" + "/"
	blockStart := "/" + "*"
	blockEnd := "*" + "/"
	htmlStart := "<!" + "--"
	htmlEnd := "--" + ">"

	switch language {
	case "Go", "Rust", "C", "C++", "C#", "Java", "Dart", "TypeScript", "JavaScript", "CSS":
		return scanCommentLines(lines, language, lineSlash, blockStart, blockEnd, true)
	case "Python", "YAML", "Shell", "TOML", "Dockerfile", "Makefile":
		return scanHashCommentLines(lines)
	case "SQL":
		return scanCommentLines(lines, language, "--", blockStart, blockEnd, true)
	case "HTML", "XML":
		return scanCommentLines(lines, language, "", htmlStart, htmlEnd, false)
	default:
		count := 0
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				count++
			}
		}
		return count
	}
}

func scanHashCommentLines(lines []string) int {
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		count++
	}
	return count
}

func scanCommentLines(lines []string, language string, linePrefix string, blockStart string, blockEnd string, trackQuotes bool) int {
	count := 0
	inBlockComment := false
	isRust := language == "Rust"

	for _, line := range lines {
		inDoubleQuote := false
		inSingleQuote := false
		inBacktick := false
		hasCode := false

		for i := 0; i < len(line); i++ {
			if inBlockComment {
				if blockEnd != "" && strings.HasPrefix(line[i:], blockEnd) {
					inBlockComment = false
					i += len(blockEnd) - 1
				}
				continue
			}
			if inDoubleQuote {
				hasCode = true
				if line[i] == '\\' {
					i++
					continue
				}
				if line[i] == '"' {
					inDoubleQuote = false
				}
				continue
			}
			if inSingleQuote {
				hasCode = true
				if line[i] == '\\' {
					i++
					continue
				}
				if line[i] == '\'' {
					inSingleQuote = false
				}
				continue
			}
			if inBacktick {
				hasCode = true
				if language != "Go" && line[i] == '\\' {
					i++
					continue
				}
				if line[i] == '`' {
					inBacktick = false
				}
				continue
			}

			ch := line[i]
			if ch == ' ' || ch == '\t' || ch == '\r' {
				continue
			}
			if linePrefix != "" && strings.HasPrefix(line[i:], linePrefix) {
				break
			}
			if blockStart != "" && strings.HasPrefix(line[i:], blockStart) {
				inBlockComment = true
				i += len(blockStart) - 1
				continue
			}

			hasCode = true
			if trackQuotes {
				if ch == '"' {
					inDoubleQuote = true
				} else if ch == '\'' {
					if !isRust || (i+2 < len(line) && line[i+2] == '\'') || (i+1 < len(line) && line[i+1] == '\\') {
						inSingleQuote = true
					}
				} else if ch == '`' {
					inBacktick = true
				}
			}
		}

		if hasCode {
			count++
		}
	}

	return count
}
