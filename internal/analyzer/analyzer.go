package analyzer

import (
	"sort"
	"strings"
	"unicode"
)

type Symbol struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Line    int    `json:"line"`
	EndLine int    `json:"endLine"`
}

type ImportRef struct {
	Target string `json:"target"`
	Line   int    `json:"line"`
}

type CallMetric struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type AnalysisResult struct {
	Symbols  []Symbol     `json:"symbols"`
	Imports  []ImportRef  `json:"imports"`
	TopCalls []CallMetric `json:"topCalls"`
}

type langKind uint8

const (
	langUnknown langKind = iota
	langGo
	langRust
	langTS
	langJS
	langDart
	langJava
	langCSharp
	langPython
	langC
	langCPP
	langSQL
)

type lexState struct {
	inBlockComment  bool
	inBacktick      bool
	inTripleQuote   byte
	inImportGroup   bool
	inTypeGroup     bool
	inImportStmt    bool
	inDollarQuote   bool
	dollarTag       string
	inTypedefStruct bool
	typedefLine     int
	typedefDepth    int
}

type openSymbol struct {
	index        int
	baseDepth    int
	sawOpenBrace bool
	indent       int
	beginDepth   int
}

type analyzerState struct {
	lang          langKind
	symbols       []Symbol
	imports       []ImportRef
	callCounts    map[string]int
	open          []openSymbol
	braceDepth    int
	parenDepth    int
	sqlBeginDepth int
	lastCodeLine  int
	lex           lexState
}

func Analyze(language string, content string) AnalysisResult {
	result := AnalysisResult{
		Symbols:  []Symbol{},
		Imports:  []ImportRef{},
		TopCalls: []CallMetric{},
	}
	lang := normalizeLanguage(language)
	if lang == langUnknown || content == "" {
		return result
	}

	st := &analyzerState{
		lang:       lang,
		symbols:    result.Symbols,
		imports:    result.Imports,
		callCounts: make(map[string]int),
	}

	lines := splitLines(content)
	for idx, rawLine := range lines {
		lineNum := idx + 1
		st.processLine(lineNum, rawLine)
	}

	totalLines := len(lines)
	st.closeRemaining(totalLines)

	result.Symbols = st.symbols
	result.Imports = st.imports
	result.TopCalls = buildTopCalls(st.callCounts)
	return result
}

func normalizeLanguage(language string) langKind {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "go", "golang":
		return langGo
	case "rust", "rs":
		return langRust
	case "typescript", "ts", "tsx", "mts", "cts":
		return langTS
	case "javascript", "js", "jsx", "mjs", "cjs":
		return langJS
	case "dart":
		return langDart
	case "java":
		return langJava
	case "c#", "csharp", "cs":
		return langCSharp
	case "python", "py", "pyw":
		return langPython
	case "c":
		return langC
	case "c++", "cpp", "cc", "cxx", "h", "hpp", "hh":
		return langCPP
	case "sql":
		return langSQL
	default:
		return langUnknown
	}
}

func splitLines(content string) []string {
	raw := strings.Split(content, "\n")
	if len(raw) > 0 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	for i := range raw {
		raw[i] = strings.TrimSuffix(raw[i], "\r")
	}
	return raw
}

func (st *analyzerState) processLine(lineNum int, rawLine string) {
	codeOnly, codeNoStr := stripLine(rawLine, st.lang, &st.lex)
	trimmedCode := strings.TrimSpace(codeOnly)
	trimmedNoStr := strings.TrimSpace(codeNoStr)

	if trimmedCode == "" {
		return
	}

	if st.lang == langPython {
		st.processPythonLine(lineNum, rawLine, trimmedCode, trimmedNoStr)
		return
	}

	if st.lang == langSQL {
		st.processSQLLine(lineNum, trimmedCode, trimmedNoStr)
		return
	}

	st.processBraceLine(lineNum, trimmedCode, trimmedNoStr)
}

func (st *analyzerState) processBraceLine(lineNum int, trimmedCode string, trimmedNoStr string) {
	st.closePendingNoBrace(lineNum, trimmedNoStr)

	prevDepth := st.braceDepth
	isDeclOrImport := st.scanBraceDeclaration(lineNum, trimmedCode, trimmedNoStr, prevDepth)

	hadBraceOnLine := false
	for i := 0; i < len(trimmedNoStr); i++ {
		switch trimmedNoStr[i] {
		case '{':
			hadBraceOnLine = true
			st.braceDepth++
			for j := range st.open {
				if st.braceDepth > st.open[j].baseDepth {
					st.open[j].sawOpenBrace = true
				}
			}
		case '}':
			if st.braceDepth > 0 {
				st.braceDepth--
			}
			st.popClosedByBrace(lineNum)
		}
	}

	st.finalizeBraceDeclOnLine(lineNum, trimmedNoStr, hadBraceOnLine)

	if !isDeclOrImport {
		collectCalls(trimmedNoStr, st.lang, st.callCounts)
	}
	st.lastCodeLine = lineNum
}

func (st *analyzerState) closePendingNoBrace(lineNum int, trimmedNoStr string) {
	if len(st.open) == 0 {
		return
	}
	top := st.open[len(st.open)-1]
	if top.sawOpenBrace {
		return
	}
	if strings.HasPrefix(trimmedNoStr, "{") {
		return
	}
	st.symbols[top.index].EndLine = st.symbols[top.index].Line
	st.open = st.open[:len(st.open)-1]
	_ = lineNum
}

func (st *analyzerState) popClosedByBrace(lineNum int) {
	for len(st.open) > 0 {
		top := st.open[len(st.open)-1]
		if top.sawOpenBrace && st.braceDepth <= top.baseDepth {
			st.symbols[top.index].EndLine = lineNum
			st.open = st.open[:len(st.open)-1]
			continue
		}
		if !top.sawOpenBrace && st.braceDepth < top.baseDepth {
			st.symbols[top.index].EndLine = st.symbols[top.index].Line
			st.open = st.open[:len(st.open)-1]
			continue
		}
		break
	}
}

func (st *analyzerState) finalizeBraceDeclOnLine(lineNum int, trimmedNoStr string, hadBraceOnLine bool) {
	for len(st.open) > 0 {
		top := st.open[len(st.open)-1]
		if top.sawOpenBrace {
			break
		}
		if hadBraceOnLine && st.braceDepth <= top.baseDepth {
			st.symbols[top.index].EndLine = lineNum
			st.open = st.open[:len(st.open)-1]
			continue
		}
		if strings.HasSuffix(trimmedNoStr, ";") {
			st.symbols[top.index].EndLine = lineNum
			st.open = st.open[:len(st.open)-1]
			continue
		}
		if st.symbols[top.index].Kind == "type" && !strings.Contains(trimmedNoStr, "{") {
			st.symbols[top.index].EndLine = lineNum
			st.open = st.open[:len(st.open)-1]
			continue
		}
		break
	}
}

func (st *analyzerState) addSymbol(name string, kind string, lineNum int, baseDepth int, indent int) {
	if name == "" {
		return
	}
	idx := len(st.symbols)
	st.symbols = append(st.symbols, Symbol{
		Name:    name,
		Kind:    kind,
		Line:    lineNum,
		EndLine: lineNum,
	})
	st.open = append(st.open, openSymbol{
		index:      idx,
		baseDepth:  baseDepth,
		indent:     indent,
		beginDepth: st.sqlBeginDepth,
	})
}

func (st *analyzerState) addImport(target string, lineNum int) {
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}
	st.imports = append(st.imports, ImportRef{
		Target: target,
		Line:   lineNum,
	})
}

func (st *analyzerState) closeRemaining(totalLines int) {
	for len(st.open) > 0 {
		top := st.open[len(st.open)-1]
		st.open = st.open[:len(st.open)-1]
		switch {
		case st.lang == langPython:
			end := st.lastCodeLine
			if end < st.symbols[top.index].Line {
				end = st.symbols[top.index].Line
			}
			st.symbols[top.index].EndLine = end
		case st.lang == langSQL:
			end := st.lastCodeLine
			if end < st.symbols[top.index].Line {
				end = st.symbols[top.index].Line
			}
			st.symbols[top.index].EndLine = end
		case top.sawOpenBrace:
			end := totalLines
			if end < st.symbols[top.index].Line {
				end = st.symbols[top.index].Line
			}
			st.symbols[top.index].EndLine = end
		default:
			st.symbols[top.index].EndLine = st.symbols[top.index].Line
		}
	}
}

func (st *analyzerState) scanBraceDeclaration(lineNum int, code string, noStr string, prevDepth int) bool {
	switch st.lang {
	case langGo:
		return st.analyzeGoLine(lineNum, code, noStr, prevDepth)
	case langRust:
		return st.analyzeRustLine(lineNum, code, noStr, prevDepth)
	case langTS, langJS:
		return st.analyzeTSJSLine(lineNum, code, noStr, prevDepth)
	case langDart, langJava, langCSharp:
		return st.analyzeCDJLine(lineNum, code, noStr, prevDepth)
	case langC, langCPP:
		return st.analyzeCCppLine(lineNum, code, noStr, prevDepth)
	default:
		return false
	}
}

func (st *analyzerState) analyzeGoLine(lineNum int, code string, noStr string, prevDepth int) bool {
	if st.lex.inImportGroup {
		if strings.HasPrefix(noStr, ")") {
			st.lex.inImportGroup = false
			return true
		}
		if target := extractQuotedString(code); target != "" {
			st.addImport(target, lineNum)
		}
		return true
	}

	if st.lex.inTypeGroup {
		if strings.HasPrefix(noStr, ")") {
			st.lex.inTypeGroup = false
			return true
		}
		return st.parseGoTypeSpec(lineNum, noStr, prevDepth)
	}

	if hasKeywordPrefix(noStr, "import") {
		rest := strings.TrimSpace(strings.TrimPrefix(noStr, "import"))
		if strings.HasPrefix(rest, "(") {
			if !strings.Contains(rest, ")") {
				st.lex.inImportGroup = true
			}
			if target := extractQuotedString(code); target != "" {
				st.addImport(target, lineNum)
			}
			return true
		}
		if target := extractQuotedString(code); target != "" {
			st.addImport(target, lineNum)
		}
		return true
	}

	if hasKeywordPrefix(noStr, "func") {
		name, kind := parseGoFuncDecl(noStr)
		if name != "" {
			st.addSymbol(name, kind, lineNum, prevDepth, 0)
			return true
		}
		return false
	}

	if hasKeywordPrefix(noStr, "type") {
		rest := strings.TrimSpace(strings.TrimPrefix(noStr, "type"))
		if strings.HasPrefix(rest, "(") {
			if !strings.Contains(rest, ")") {
				st.lex.inTypeGroup = true
				return true
			}
			inner := strings.TrimSuffix(strings.TrimPrefix(rest, "("), ")")
			return st.parseGoTypeSpec(lineNum, strings.TrimSpace(inner), prevDepth)
		}
		return st.parseGoTypeSpec(lineNum, rest, prevDepth)
	}

	if prevDepth > 0 && len(st.open) > 0 && st.symbols[st.open[len(st.open)-1].index].Kind == "interface" {
		if name := parseInterfaceMethodName(noStr); name != "" {
			st.addSymbol(name, "method", lineNum, prevDepth, 0)
			return true
		}
	}

	return false
}

func (st *analyzerState) parseGoTypeSpec(lineNum int, spec string, prevDepth int) bool {
	name, rest := readIdent(spec)
	if name == "" {
		return false
	}
	rest = skipBracketGroup(strings.TrimSpace(rest), '[', ']')
	rest = strings.TrimSpace(rest)
	kind := "type"
	if hasKeywordPrefix(rest, "struct") {
		kind = "struct"
	} else if hasKeywordPrefix(rest, "interface") {
		kind = "interface"
	}
	st.addSymbol(name, kind, lineNum, prevDepth, 0)
	return true
}

func parseGoFuncDecl(line string) (string, string) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "func"))
	if rest == "" {
		return "", ""
	}
	kind := "function"
	if strings.HasPrefix(rest, "(") {
		afterRecv := skipBalanced(rest, '(', ')')
		if afterRecv == rest {
			return "", ""
		}
		rest = strings.TrimSpace(afterRecv)
		kind = "method"
	}
	name, afterName := readIdent(rest)
	if name == "" {
		return "", ""
	}
	afterName = strings.TrimSpace(afterName)
	if strings.HasPrefix(afterName, "[") {
		afterName = strings.TrimSpace(skipBalanced(afterName, '[', ']'))
	}
	if !strings.HasPrefix(afterName, "(") {
		return "", ""
	}
	return name, kind
}

func parseInterfaceMethodName(line string) string {
	name, rest := readIdent(line)
	if name == "" || isKeywordForLang(name, langGo) {
		return ""
	}
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "[") {
		rest = strings.TrimSpace(skipBalanced(rest, '[', ']'))
	}
	if strings.HasPrefix(rest, "(") {
		return name
	}
	return ""
}

func (st *analyzerState) analyzeRustLine(lineNum int, code string, noStr string, prevDepth int) bool {
	if strings.HasPrefix(noStr, "#[") || strings.HasPrefix(noStr, "#![") {
		return false
	}
	trimmed := stripRustModifiers(noStr)
	if hasKeywordPrefix(trimmed, "use") {
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "use"))
		rest = strings.TrimSuffix(rest, ";")
		st.addImport(strings.TrimSpace(rest), lineNum)
		return true
	}
	if hasKeywordPrefix(trimmed, "fn") {
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "fn"))
		name, after := readIdent(rest)
		after = strings.TrimSpace(after)
		if name != "" && (strings.HasPrefix(after, "(") || strings.HasPrefix(after, "<")) {
			kind := "function"
			if prevDepth > 0 && st.hasEnclosingKind("impl", "trait") {
				kind = "method"
			}
			st.addSymbol(name, kind, lineNum, prevDepth, 0)
			return true
		}
		return false
	}
	if hasKeywordPrefix(trimmed, "struct") {
		name, _ := readIdent(strings.TrimSpace(strings.TrimPrefix(trimmed, "struct")))
		if name != "" {
			st.addSymbol(name, "struct", lineNum, prevDepth, 0)
			return true
		}
	}
	if hasKeywordPrefix(trimmed, "enum") {
		name, _ := readIdent(strings.TrimSpace(strings.TrimPrefix(trimmed, "enum")))
		if name != "" {
			st.addSymbol(name, "enum", lineNum, prevDepth, 0)
			return true
		}
	}
	if hasKeywordPrefix(trimmed, "trait") {
		name, _ := readIdent(strings.TrimSpace(strings.TrimPrefix(trimmed, "trait")))
		if name != "" {
			st.addSymbol(name, "trait", lineNum, prevDepth, 0)
			return true
		}
	}
	if hasKeywordPrefix(trimmed, "impl") {
		name := parseRustImplName(strings.TrimSpace(strings.TrimPrefix(trimmed, "impl")))
		if name != "" {
			st.addSymbol(name, "impl", lineNum, prevDepth, 0)
			return true
		}
	}
	if hasKeywordPrefix(trimmed, "type") {
		name, rest := readIdent(strings.TrimSpace(strings.TrimPrefix(trimmed, "type")))
		if name != "" && strings.Contains(rest, "=") {
			st.addSymbol(name, "type", lineNum, prevDepth, 0)
			return true
		}
	}
	return false
}

func stripRustModifiers(s string) string {
	for {
		s = strings.TrimSpace(s)
		switch {
		case hasKeywordPrefix(s, "pub"):
			s = strings.TrimSpace(strings.TrimPrefix(s, "pub"))
			if strings.HasPrefix(s, "(") {
				s = skipBalanced(s, '(', ')')
			}
		case hasKeywordPrefix(s, "async"):
			s = strings.TrimPrefix(s, "async")
		case hasKeywordPrefix(s, "unsafe"):
			s = strings.TrimPrefix(s, "unsafe")
		case hasKeywordPrefix(s, "const"):
			s = strings.TrimPrefix(s, "const")
		case hasKeywordPrefix(s, "extern"):
			s = strings.TrimSpace(strings.TrimPrefix(s, "extern"))
			if strings.HasPrefix(s, `"`) {
				if idx := strings.IndexByte(s[1:], '"'); idx >= 0 {
					s = s[idx+2:]
				}
			}
		default:
			return s
		}
	}
}

func parseRustImplName(rest string) string {
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "<") {
		rest = strings.TrimSpace(skipBalanced(rest, '<', '>'))
	}
	if idx := strings.IndexByte(rest, '{'); idx >= 0 {
		rest = strings.TrimSpace(rest[:idx])
	}
	if idx := strings.Index(rest, " where "); idx >= 0 {
		rest = strings.TrimSpace(rest[:idx])
	}
	rest = stripAngleGenerics(rest)
	return strings.TrimSpace(rest)
}

func (st *analyzerState) analyzeTSJSLine(lineNum int, code string, noStr string, prevDepth int) bool {
	if st.lex.inImportStmt {
		if target := extractAfterFrom(code); target != "" {
			st.addImport(target, lineNum)
			st.lex.inImportStmt = false
		} else if strings.Contains(noStr, ";") {
			st.lex.inImportStmt = false
		}
		return true
	}

	if hasKeywordPrefix(noStr, "import") {
		if target := extractAfterFrom(code); target != "" {
			st.addImport(target, lineNum)
			return true
		}
		rest := strings.TrimSpace(strings.TrimPrefix(code, "import"))
		if strings.HasPrefix(rest, `"`) || strings.HasPrefix(rest, "'") {
			if target := extractQuotedString(rest); target != "" {
				st.addImport(target, lineNum)
			}
			return true
		}
		if !strings.Contains(noStr, "=") && !strings.Contains(noStr, "(") {
			st.lex.inImportStmt = true
			return true
		}
	}

	if hasKeywordPrefix(noStr, "export") {
		if target := extractAfterFrom(code); target != "" {
			st.addImport(target, lineNum)
			return true
		}
	}

	foundRequire := st.extractRequireCalls(code, lineNum)

	trimmed := stripTSModifiers(noStr)

	if hasKeywordPrefix(trimmed, "class") {
		name, _ := readIdent(strings.TrimSpace(strings.TrimPrefix(trimmed, "class")))
		if name != "" {
			st.addSymbol(name, "class", lineNum, prevDepth, 0)
			return true
		}
	}
	if hasKeywordPrefix(trimmed, "interface") {
		name, _ := readIdent(strings.TrimSpace(strings.TrimPrefix(trimmed, "interface")))
		if name != "" {
			st.addSymbol(name, "interface", lineNum, prevDepth, 0)
			return true
		}
	}
	if hasKeywordPrefix(trimmed, "enum") {
		name, _ := readIdent(strings.TrimSpace(strings.TrimPrefix(trimmed, "enum")))
		if name != "" {
			st.addSymbol(name, "enum", lineNum, prevDepth, 0)
			return true
		}
	}
	if hasKeywordPrefix(trimmed, "type") {
		name, rest := readIdent(strings.TrimSpace(strings.TrimPrefix(trimmed, "type")))
		rest = strings.TrimSpace(skipBalanced(rest, '<', '>'))
		if name != "" && strings.HasPrefix(strings.TrimSpace(rest), "=") {
			st.addSymbol(name, "type", lineNum, prevDepth, 0)
			return true
		}
	}
	if hasKeywordPrefix(trimmed, "function") {
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "function"))
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "*"))
		name, after := readIdent(rest)
		after = strings.TrimSpace(after)
		if name != "" && (strings.HasPrefix(after, "(") || strings.HasPrefix(after, "<")) {
			st.addSymbol(name, "function", lineNum, prevDepth, 0)
			return true
		}
	}

	if name := parseArrowFunctionDecl(trimmed); name != "" {
		st.addSymbol(name, "function", lineNum, prevDepth, 0)
		return true
	}

	if prevDepth > 0 && st.hasEnclosingKind("class", "interface") {
		if name := parseTSMethodDecl(trimmed); name != "" {
			st.addSymbol(name, "method", lineNum, prevDepth, 0)
			return true
		}
	}

	return foundRequire
}

func (st *analyzerState) extractRequireCalls(code string, lineNum int) bool {
	found := false
	search := code
	for {
		idx := strings.Index(search, "require")
		if idx < 0 {
			break
		}
		if idx > 0 && isIdentChar(search[idx-1]) {
			search = search[idx+7:]
			continue
		}
		rest := strings.TrimSpace(search[idx+7:])
		if strings.HasPrefix(rest, "(") {
			inside := strings.TrimSpace(rest[1:])
			if target := extractQuotedString(inside); target != "" {
				st.addImport(target, lineNum)
				found = true
			}
		}
		search = search[idx+7:]
	}
	return found
}

func stripTSModifiers(s string) string {
	for {
		s = strings.TrimSpace(s)
		switch {
		case hasKeywordPrefix(s, "export"):
			s = strings.TrimPrefix(s, "export")
		case hasKeywordPrefix(s, "default"):
			s = strings.TrimPrefix(s, "default")
		case hasKeywordPrefix(s, "declare"):
			s = strings.TrimPrefix(s, "declare")
		case hasKeywordPrefix(s, "abstract"):
			s = strings.TrimPrefix(s, "abstract")
		case hasKeywordPrefix(s, "async"):
			s = strings.TrimPrefix(s, "async")
		case hasKeywordPrefix(s, "public"):
			s = strings.TrimPrefix(s, "public")
		case hasKeywordPrefix(s, "private"):
			s = strings.TrimPrefix(s, "private")
		case hasKeywordPrefix(s, "protected"):
			s = strings.TrimPrefix(s, "protected")
		case hasKeywordPrefix(s, "static"):
			s = strings.TrimPrefix(s, "static")
		case hasKeywordPrefix(s, "override"):
			s = strings.TrimPrefix(s, "override")
		case hasKeywordPrefix(s, "readonly"):
			s = strings.TrimPrefix(s, "readonly")
		default:
			return s
		}
	}
}

func parseArrowFunctionDecl(s string) string {
	var rest string
	switch {
	case hasKeywordPrefix(s, "const"):
		rest = strings.TrimPrefix(s, "const")
	case hasKeywordPrefix(s, "let"):
		rest = strings.TrimPrefix(s, "let")
	case hasKeywordPrefix(s, "var"):
		rest = strings.TrimPrefix(s, "var")
	default:
		return ""
	}
	name, after := readIdent(strings.TrimSpace(rest))
	if name == "" {
		return ""
	}
	eqIdx := findAssignEqual(after)
	if eqIdx < 0 {
		return ""
	}
	rhs := strings.TrimSpace(after[eqIdx+1:])
	if strings.HasPrefix(rhs, ">") {
		return ""
	}
	if hasKeywordPrefix(rhs, "async") {
		rhs = strings.TrimSpace(strings.TrimPrefix(rhs, "async"))
	}
	if strings.HasPrefix(rhs, "<") {
		rhs = strings.TrimSpace(skipBalanced(rhs, '<', '>'))
	}
	if strings.HasPrefix(rhs, "(") {
		afterParen := strings.TrimSpace(skipBalanced(rhs, '(', ')'))
		if afterParen == rhs {
			return ""
		}
		if strings.HasPrefix(afterParen, ":") {
			arrowIdx := strings.Index(afterParen, "=>")
			if arrowIdx >= 0 {
				return name
			}
			return ""
		}
		if strings.HasPrefix(afterParen, "=>") {
			return name
		}
		return ""
	}
	arg, afterArg := readIdent(rhs)
	if arg != "" && strings.HasPrefix(strings.TrimSpace(afterArg), "=>") {
		return name
	}
	return ""
}

func findAssignEqual(s string) int {
	parenDepth := 0
	angleDepth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '<':
			angleDepth++
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case '=':
			if parenDepth == 0 && angleDepth == 0 {
				if i+1 < len(s) && (s[i+1] == '>' || s[i+1] == '=') {
					i++
					continue
				}
				return i
			}
		}
	}
	return -1
}

func parseTSMethodDecl(s string) string {
	if hasKeywordPrefix(s, "get") || hasKeywordPrefix(s, "set") {
		cand := strings.TrimSpace(s[3:])
		if !strings.HasPrefix(cand, "(") {
			s = cand
		}
	}
	name, rest := readIdent(s)
	if name == "" || isTSJSKeyword(name) {
		return ""
	}
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "<") {
		rest = strings.TrimSpace(skipBalanced(rest, '<', '>'))
	}
	if !strings.HasPrefix(rest, "(") {
		return ""
	}
	afterParen := strings.TrimSpace(skipBalanced(rest, '(', ')'))
	if afterParen == rest {
		return ""
	}
	if strings.HasSuffix(s, "{") || strings.Contains(afterParen, "{") {
		return name
	}
	return ""
}

func (st *analyzerState) analyzeCDJLine(lineNum int, code string, noStr string, prevDepth int) bool {
	if st.lang == langDart && hasKeywordPrefix(noStr, "import") {
		if target := extractQuotedString(code); target != "" {
			st.addImport(target, lineNum)
		}
		return true
	}
	if st.lang == langJava && hasKeywordPrefix(noStr, "import") {
		rest := strings.TrimSpace(strings.TrimPrefix(noStr, "import"))
		if hasKeywordPrefix(rest, "static") {
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "static"))
		}
		rest = strings.TrimSuffix(strings.TrimSpace(rest), ";")
		st.addImport(rest, lineNum)
		return true
	}
	if st.lang == langCSharp && prevDepth == 0 {
		cand := noStr
		if hasKeywordPrefix(cand, "global") {
			cand = strings.TrimSpace(strings.TrimPrefix(cand, "global"))
		}
		if hasKeywordPrefix(cand, "using") {
			rest := strings.TrimSpace(strings.TrimPrefix(cand, "using"))
			if !strings.HasPrefix(rest, "(") && !hasKeywordPrefix(rest, "var") {
				if hasKeywordPrefix(rest, "static") {
					rest = strings.TrimSpace(strings.TrimPrefix(rest, "static"))
				}
				if eqIdx := strings.IndexByte(rest, '='); eqIdx >= 0 {
					rest = strings.TrimSpace(rest[eqIdx+1:])
				}
				rest = strings.TrimSuffix(strings.TrimSpace(rest), ";")
				st.addImport(rest, lineNum)
				return true
			}
		}
	}

	if strings.HasPrefix(noStr, "@") || strings.HasPrefix(noStr, "[") {
		return false
	}

	trimmed := stripCDJModifiers(noStr)

	if hasKeywordPrefix(trimmed, "class") {
		name, _ := readIdent(strings.TrimSpace(strings.TrimPrefix(trimmed, "class")))
		if name != "" {
			st.addSymbol(name, "class", lineNum, prevDepth, 0)
			return true
		}
	}
	if hasKeywordPrefix(trimmed, "enum") {
		name, _ := readIdent(strings.TrimSpace(strings.TrimPrefix(trimmed, "enum")))
		if name != "" {
			st.addSymbol(name, "enum", lineNum, prevDepth, 0)
			return true
		}
	}
	if hasKeywordPrefix(trimmed, "interface") {
		name, _ := readIdent(strings.TrimSpace(strings.TrimPrefix(trimmed, "interface")))
		if name != "" {
			st.addSymbol(name, "interface", lineNum, prevDepth, 0)
			return true
		}
	}
	if st.lang == langCSharp && hasKeywordPrefix(trimmed, "struct") {
		name, _ := readIdent(strings.TrimSpace(strings.TrimPrefix(trimmed, "struct")))
		if name != "" {
			st.addSymbol(name, "struct", lineNum, prevDepth, 0)
			return true
		}
	}
	if (st.lang == langJava || st.lang == langCSharp) && hasKeywordPrefix(trimmed, "record") {
		name, _ := readIdent(strings.TrimSpace(strings.TrimPrefix(trimmed, "record")))
		if name != "" {
			st.addSymbol(name, "class", lineNum, prevDepth, 0)
			return true
		}
	}
	if st.lang == langDart && hasKeywordPrefix(trimmed, "mixin") {
		name, _ := readIdent(strings.TrimSpace(strings.TrimPrefix(trimmed, "mixin")))
		if name != "" {
			st.addSymbol(name, "mixin", lineNum, prevDepth, 0)
			return true
		}
	}
	if st.lang == langDart && hasKeywordPrefix(trimmed, "extension") {
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "extension"))
		name, after := readIdent(rest)
		if name == "on" {
			target, _ := readIdent(strings.TrimSpace(after))
			if target != "" {
				name = "on " + target
			}
		}
		if name != "" {
			st.addSymbol(name, "extension", lineNum, prevDepth, 0)
			return true
		}
	}

	if name := parseMethodOrFuncDecl(trimmed, st.lang); name != "" {
		kind := "function"
		if prevDepth > 0 || st.lang != langDart {
			kind = "method"
		}
		st.addSymbol(name, kind, lineNum, prevDepth, 0)
		return true
	}

	return false
}

func stripCDJModifiers(s string) string {
	for {
		s = strings.TrimSpace(s)
		switch {
		case hasKeywordPrefix(s, "public"):
			s = strings.TrimPrefix(s, "public")
		case hasKeywordPrefix(s, "private"):
			s = strings.TrimPrefix(s, "private")
		case hasKeywordPrefix(s, "protected"):
			s = strings.TrimPrefix(s, "protected")
		case hasKeywordPrefix(s, "internal"):
			s = strings.TrimPrefix(s, "internal")
		case hasKeywordPrefix(s, "static"):
			s = strings.TrimPrefix(s, "static")
		case hasKeywordPrefix(s, "final"):
			s = strings.TrimPrefix(s, "final")
		case hasKeywordPrefix(s, "abstract"):
			s = strings.TrimPrefix(s, "abstract")
		case hasKeywordPrefix(s, "sealed"):
			s = strings.TrimPrefix(s, "sealed")
		case hasKeywordPrefix(s, "partial"):
			s = strings.TrimPrefix(s, "partial")
		case hasKeywordPrefix(s, "virtual"):
			s = strings.TrimPrefix(s, "virtual")
		case hasKeywordPrefix(s, "override"):
			s = strings.TrimPrefix(s, "override")
		case hasKeywordPrefix(s, "async"):
			s = strings.TrimPrefix(s, "async")
		case hasKeywordPrefix(s, "synchronized"):
			s = strings.TrimPrefix(s, "synchronized")
		case hasKeywordPrefix(s, "extern"):
			s = strings.TrimPrefix(s, "extern")
		case hasKeywordPrefix(s, "readonly"):
			s = strings.TrimPrefix(s, "readonly")
		case hasKeywordPrefix(s, "base"):
			s = strings.TrimPrefix(s, "base")
		default:
			return s
		}
	}
}

func parseMethodOrFuncDecl(line string, lang langKind) string {
	if line == "" {
		return ""
	}
	first, _ := readIdent(line)
	if isDeclarationExcludeStarter(first) {
		return ""
	}
	parenIdx := findDeclOpenParen(line)
	if parenIdx < 0 {
		return ""
	}
	beforeParen := strings.TrimSpace(line[:parenIdx])
	if beforeParen == "" || strings.ContainsAny(beforeParen, "=;.") {
		if lang == langDart && strings.Contains(beforeParen, ".") && !strings.ContainsAny(beforeParen, "=;") {
			dotIdx := strings.LastIndexByte(beforeParen, '.')
			cls, _ := readIdent(beforeParen[:dotIdx])
			ctor, _ := readIdent(beforeParen[dotIdx+1:])
			if cls != "" && ctor != "" && !isDeclarationExcludeStarter(cls) {
				if isDeclTerminator(line[parenIdx:]) {
					return cls + "." + ctor
				}
			}
		}
		return ""
	}

	tokens := splitHeaderTokens(beforeParen)
	if len(tokens) == 0 {
		return ""
	}
	name := tokens[len(tokens)-1]
	if name == "" || isKeywordForLang(name, lang) {
		return ""
	}
	if len(tokens) == 1 && lang == langDart && !unicode.IsUpper(rune(name[0])) {
		return ""
	}
	if len(tokens) == 1 && (lang == langJava || lang == langCSharp) && !unicode.IsUpper(rune(name[0])) {
		return ""
	}
	if !isDeclTerminator(line[parenIdx:]) {
		return ""
	}
	return name
}

func isDeclTerminator(fromParen string) bool {
	afterParen := strings.TrimSpace(skipBalanced(fromParen, '(', ')'))
	if afterParen == fromParen {
		return false
	}
	if strings.Contains(afterParen, "{") || strings.HasPrefix(afterParen, "=>") {
		return true
	}
	if strings.HasSuffix(strings.TrimSpace(fromParen), ";") {
		return false
	}
	if afterParen == "" || hasKeywordPrefix(afterParen, "async") || hasKeywordPrefix(afterParen, "throws") || hasKeywordPrefix(afterParen, "where") || strings.HasPrefix(afterParen, ":") {
		return true
	}
	return false
}

func (st *analyzerState) analyzeCCppLine(lineNum int, code string, noStr string, prevDepth int) bool {
	if strings.HasPrefix(noStr, "#") {
		directive := strings.TrimSpace(noStr[1:])
		if hasKeywordPrefix(directive, "include") {
			codeDir := strings.TrimSpace(strings.TrimPrefix(code, "#"))
			rest := strings.TrimSpace(strings.TrimPrefix(codeDir, "include"))
			if target := extractIncludeTarget(rest); target != "" {
				st.addImport(target, lineNum)
			}
			return true
		}
		return true
	}

	if hasKeywordPrefix(noStr, "typedef") {
		rest := strings.TrimSpace(strings.TrimPrefix(noStr, "typedef"))
		if hasKeywordPrefix(rest, "struct") {
			if name := parseTypedefName(rest); name != "" {
				st.addSymbol(name, "struct", lineNum, prevDepth, 0)
				return true
			}
			afterStruct := strings.TrimSpace(strings.TrimPrefix(rest, "struct"))
			if afterStruct == "" || strings.HasPrefix(afterStruct, "{") {
				st.lex.inTypedefStruct = true
				st.lex.typedefLine = lineNum
				st.lex.typedefDepth = prevDepth
				return true
			}
		}
	}

	if st.lex.inTypedefStruct && prevDepth == st.lex.typedefDepth+1 && strings.HasPrefix(noStr, "}") {
		st.lex.inTypedefStruct = false
		if name, _ := readIdent(strings.TrimSpace(noStr[1:])); name != "" {
			st.symbols = append(st.symbols, Symbol{
				Name:    name,
				Kind:    "struct",
				Line:    st.lex.typedefLine,
				EndLine: lineNum,
			})
			return true
		}
	}

	if hasKeywordPrefix(noStr, "enum") {
		rest := strings.TrimSpace(strings.TrimPrefix(noStr, "enum"))
		if hasKeywordPrefix(rest, "class") {
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "class"))
		} else if hasKeywordPrefix(rest, "struct") {
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "struct"))
		}
		name, after := readIdent(rest)
		if name != "" && !strings.HasPrefix(strings.TrimSpace(after), ";") {
			st.addSymbol(name, "enum", lineNum, prevDepth, 0)
			return true
		}
	}

	if hasKeywordPrefix(noStr, "struct") {
		rest := strings.TrimSpace(strings.TrimPrefix(noStr, "struct"))
		name, after := readIdent(rest)
		after = strings.TrimSpace(after)
		if name != "" && (strings.HasPrefix(after, "{") || strings.HasPrefix(after, ":") || after == "") {
			st.addSymbol(name, "struct", lineNum, prevDepth, 0)
			return true
		}
	}

	if hasKeywordPrefix(noStr, "class") {
		rest := strings.TrimSpace(strings.TrimPrefix(noStr, "class"))
		name, after := readIdent(rest)
		after = strings.TrimSpace(after)
		if name != "" && (strings.HasPrefix(after, "{") || strings.HasPrefix(after, ":") || hasKeywordPrefix(after, "final") || after == "") {
			st.addSymbol(name, "class", lineNum, prevDepth, 0)
			return true
		}
	}

	if name := parseCCppFuncDecl(noStr); name != "" {
		kind := "function"
		if strings.Contains(name, "::") || (prevDepth > 0 && st.hasEnclosingKind("class", "struct")) {
			kind = "method"
		}
		st.addSymbol(name, kind, lineNum, prevDepth, 0)
		return true
	}

	return false
}

func parseTypedefName(rest string) string {
	afterStruct := strings.TrimSpace(strings.TrimPrefix(rest, "struct"))
	if name, _ := readIdent(afterStruct); name != "" {
		return name
	}
	if braceClose := strings.LastIndexByte(afterStruct, '}'); braceClose >= 0 {
		tail := strings.TrimSpace(afterStruct[braceClose+1:])
		if name, _ := readIdent(tail); name != "" {
			return name
		}
	}
	return ""
}

func parseCCppFuncDecl(line string) string {
	first, _ := readIdent(line)
	if isDeclarationExcludeStarter(first) || hasKeywordPrefix(line, "template") {
		return ""
	}
	parenIdx := findDeclOpenParen(line)
	if parenIdx < 0 {
		return ""
	}
	beforeParen := strings.TrimSpace(line[:parenIdx])
	if beforeParen == "" || strings.ContainsAny(beforeParen, "=;.") {
		return ""
	}
	tokens := splitHeaderTokens(beforeParen)
	if len(tokens) == 0 {
		return ""
	}
	name := tokens[len(tokens)-1]
	if len(tokens) == 1 && !strings.Contains(name, "::") && !strings.HasPrefix(name, "~") {
		return ""
	}
	if name == "" || isKeywordForLang(name, langCPP) {
		return ""
	}
	if strings.HasSuffix(line, ";") {
		return ""
	}
	afterParen := strings.TrimSpace(skipBalanced(line[parenIdx:], '(', ')'))
	if afterParen == line[parenIdx:] {
		return ""
	}
	if strings.Contains(afterParen, "{") || afterParen == "" || hasKeywordPrefix(afterParen, "const") || hasKeywordPrefix(afterParen, "override") || hasKeywordPrefix(afterParen, "noexcept") {
		return name
	}
	return ""
}

func (st *analyzerState) processPythonLine(lineNum int, rawLine string, trimmedCode string, trimmedNoStr string) {
	indent := measureIndent(rawLine)
	if st.parenDepth == 0 {
		st.popClosedPythonByIndent(indent)
	}

	isDeclOrImport := st.analyzePythonLine(lineNum, trimmedCode, trimmedNoStr, indent)

	for i := 0; i < len(trimmedNoStr); i++ {
		switch trimmedNoStr[i] {
		case '(', '[', '{':
			st.parenDepth++
		case ')', ']', '}':
			if st.parenDepth > 0 {
				st.parenDepth--
			}
		}
	}

	if !isDeclOrImport {
		collectCalls(trimmedNoStr, langPython, st.callCounts)
	}
	st.lastCodeLine = lineNum
}

func (st *analyzerState) popClosedPythonByIndent(indent int) {
	for len(st.open) > 0 {
		top := st.open[len(st.open)-1]
		if indent <= top.indent {
			end := st.lastCodeLine
			if end < st.symbols[top.index].Line {
				end = st.symbols[top.index].Line
			}
			st.symbols[top.index].EndLine = end
			st.open = st.open[:len(st.open)-1]
			continue
		}
		break
	}
}

func (st *analyzerState) analyzePythonLine(lineNum int, code string, noStr string, indent int) bool {
	if strings.HasPrefix(noStr, "@") {
		return true
	}
	if hasKeywordPrefix(noStr, "import") {
		rest := strings.TrimSpace(strings.TrimPrefix(noStr, "import"))
		for _, part := range strings.Split(rest, ",") {
			part = strings.TrimSpace(part)
			if asIdx := strings.Index(part, " as "); asIdx >= 0 {
				part = strings.TrimSpace(part[:asIdx])
			}
			if part != "" {
				st.addImport(part, lineNum)
			}
		}
		return true
	}
	if hasKeywordPrefix(noStr, "from") {
		rest := strings.TrimSpace(strings.TrimPrefix(noStr, "from"))
		if impIdx := strings.Index(rest, " import "); impIdx >= 0 {
			target := strings.TrimSpace(rest[:impIdx])
			st.addImport(target, lineNum)
			return true
		}
	}
	if hasKeywordPrefix(noStr, "class") {
		name, _ := readIdent(strings.TrimSpace(strings.TrimPrefix(noStr, "class")))
		if name != "" {
			st.addSymbol(name, "class", lineNum, 0, indent)
			return true
		}
	}
	cand := noStr
	if hasKeywordPrefix(cand, "async") {
		cand = strings.TrimSpace(strings.TrimPrefix(cand, "async"))
	}
	if hasKeywordPrefix(cand, "def") {
		name, after := readIdent(strings.TrimSpace(strings.TrimPrefix(cand, "def")))
		if name != "" && strings.HasPrefix(strings.TrimSpace(after), "(") {
			kind := "function"
			if st.hasEnclosingKind("class") {
				kind = "method"
			}
			st.addSymbol(name, kind, lineNum, 0, indent)
			return true
		}
	}
	_ = code
	return false
}

func (st *analyzerState) processSQLLine(lineNum int, trimmedCode string, trimmedNoStr string) {
	upper := strings.ToUpper(trimmedNoStr)
	isDecl := false

	if !st.lex.inDollarQuote && strings.HasPrefix(upper, "CREATE") {
		if name, kind := parseSQLCreate(trimmedNoStr, upper); name != "" {
			st.addSymbol(name, kind, lineNum, 0, 0)
			isDecl = true
		}
	}

	st.scanSQLBlockTokens(trimmedNoStr, lineNum)

	if !isDecl && !st.lex.inDollarQuote {
		collectCalls(trimmedNoStr, langSQL, st.callCounts)
	}
	st.lastCodeLine = lineNum
}

func parseSQLCreate(orig string, upper string) (string, string) {
	restOrig := strings.TrimSpace(orig[len("CREATE"):])
	restUpper := strings.TrimSpace(upper[len("CREATE"):])

	for {
		switch {
		case strings.HasPrefix(restUpper, "OR REPLACE "):
			restOrig = strings.TrimSpace(restOrig[len("OR REPLACE "):])
			restUpper = strings.TrimSpace(restUpper[len("OR REPLACE "):])
		case strings.HasPrefix(restUpper, "TEMPORARY "):
			restOrig = strings.TrimSpace(restOrig[len("TEMPORARY "):])
			restUpper = strings.TrimSpace(restUpper[len("TEMPORARY "):])
		case strings.HasPrefix(restUpper, "TEMP "):
			restOrig = strings.TrimSpace(restOrig[len("TEMP "):])
			restUpper = strings.TrimSpace(restUpper[len("TEMP "):])
		case strings.HasPrefix(restUpper, "UNLOGGED "):
			restOrig = strings.TrimSpace(restOrig[len("UNLOGGED "):])
			restUpper = strings.TrimSpace(restUpper[len("UNLOGGED "):])
		case strings.HasPrefix(restUpper, "MATERIALIZED "):
			restOrig = strings.TrimSpace(restOrig[len("MATERIALIZED "):])
			restUpper = strings.TrimSpace(restUpper[len("MATERIALIZED "):])
		default:
			goto doneMods
		}
	}
doneMods:

	kind := ""
	kwLen := 0
	switch {
	case hasWordUpper(restUpper, "TABLE"):
		kind = "table"
		kwLen = len("TABLE")
	case hasWordUpper(restUpper, "VIEW"):
		kind = "view"
		kwLen = len("VIEW")
	case hasWordUpper(restUpper, "FUNCTION"):
		kind = "function"
		kwLen = len("FUNCTION")
	case hasWordUpper(restUpper, "POLICY"):
		kind = "policy"
		kwLen = len("POLICY")
	default:
		return "", ""
	}

	restOrig = strings.TrimSpace(restOrig[kwLen:])
	restUpper = strings.TrimSpace(restUpper[kwLen:])
	if strings.HasPrefix(restUpper, "IF NOT EXISTS") {
		after := restUpper[len("IF NOT EXISTS"):]
		if after == "" || !isIdentChar(after[0]) {
			restOrig = strings.TrimSpace(restOrig[len("IF NOT EXISTS"):])
		}
	}

	name := readSQLObjectName(restOrig)
	return name, kind
}

func (st *analyzerState) scanSQLBlockTokens(line string, lineNum int) {
	i := 0
	n := len(line)
	for i < n {
		if line[i] == '$' {
			if tag, tagLen, ok := readDollarTag(line[i:]); ok {
				if !st.lex.inDollarQuote {
					st.lex.inDollarQuote = true
					st.lex.dollarTag = tag
				} else if st.lex.dollarTag == tag {
					st.lex.inDollarQuote = false
					st.lex.dollarTag = ""
				}
				i += tagLen
				continue
			}
		}
		if st.lex.inDollarQuote {
			i++
			continue
		}
		switch line[i] {
		case '(':
			st.parenDepth++
		case ')':
			if st.parenDepth > 0 {
				st.parenDepth--
			}
		case ';':
			if st.parenDepth == 0 && st.sqlBeginDepth == 0 {
				st.closeSQLSymbols(lineNum)
			}
		}
		if isIdentStart(line[i]) {
			word, _ := readIdent(line[i:])
			up := strings.ToUpper(word)
			if up == "BEGIN" {
				st.sqlBeginDepth++
			} else if up == "END" && st.sqlBeginDepth > 0 {
				st.sqlBeginDepth--
			}
			i += len(word)
			continue
		}
		i++
	}
}

func (st *analyzerState) closeSQLSymbols(lineNum int) {
	for len(st.open) > 0 {
		top := st.open[len(st.open)-1]
		st.symbols[top.index].EndLine = lineNum
		st.open = st.open[:len(st.open)-1]
	}
}

func (st *analyzerState) hasEnclosingKind(kinds ...string) bool {
	for _, item := range st.open {
		k := st.symbols[item.index].Kind
		for _, target := range kinds {
			if k == target {
				return true
			}
		}
	}
	return false
}

func stripLine(line string, lang langKind, state *lexState) (string, string) {
	n := len(line)
	var codeOnly strings.Builder
	var codeNoStr strings.Builder
	codeOnly.Grow(n)
	codeNoStr.Grow(n)

	i := 0
	for i < n {
		if state.inBlockComment {
			if line[i] == '*' && i+1 < n && line[i+1] == '/' {
				state.inBlockComment = false
				codeOnly.WriteString("  ")
				codeNoStr.WriteString("  ")
				i += 2
				continue
			}
			codeOnly.WriteByte(' ')
			codeNoStr.WriteByte(' ')
			i++
			continue
		}

		if state.inTripleQuote != 0 {
			q := state.inTripleQuote
			if line[i] == '\\' && i+1 < n {
				codeOnly.WriteString(line[i : i+2])
				codeNoStr.WriteString("  ")
				i += 2
				continue
			}
			if line[i] == q && i+2 < n && line[i+1] == q && line[i+2] == q {
				state.inTripleQuote = 0
				codeOnly.WriteString(line[i : i+3])
				codeNoStr.WriteString("   ")
				i += 3
				continue
			}
			codeOnly.WriteByte(line[i])
			codeNoStr.WriteByte(' ')
			i++
			continue
		}

		if state.inBacktick {
			if lang != langGo && line[i] == '\\' && i+1 < n {
				codeOnly.WriteString(line[i : i+2])
				codeNoStr.WriteString("  ")
				i += 2
				continue
			}
			if line[i] == '`' {
				state.inBacktick = false
				codeOnly.WriteByte('`')
				codeNoStr.WriteByte(' ')
				i++
				continue
			}
			codeOnly.WriteByte(line[i])
			codeNoStr.WriteByte(' ')
			i++
			continue
		}

		ch := line[i]

		if lang == langPython && ch == '#' {
			break
		}
		if lang == langSQL && ch == '-' && i+1 < n && line[i+1] == '-' {
			break
		}
		if lang != langPython && lang != langSQL && ch == '/' && i+1 < n && line[i+1] == '/' {
			break
		}
		if lang != langPython && ch == '/' && i+1 < n && line[i+1] == '*' {
			state.inBlockComment = true
			codeOnly.WriteString("  ")
			codeNoStr.WriteString("  ")
			i += 2
			continue
		}

		if lang == langPython && (ch == '"' || ch == '\'') && i+2 < n && line[i+1] == ch && line[i+2] == ch {
			state.inTripleQuote = ch
			codeOnly.WriteString(line[i : i+3])
			codeNoStr.WriteString("   ")
			i += 3
			continue
		}

		if (lang == langGo || lang == langTS || lang == langJS || lang == langSQL) && ch == '`' {
			state.inBacktick = true
			codeOnly.WriteByte('`')
			codeNoStr.WriteByte(' ')
			i++
			continue
		}

		if ch == '"' && lang != langSQL {
			end := scanQuotedLiteral(line, i, '"', true)
			codeOnly.WriteString(line[i:end])
			writeSpaces(&codeNoStr, end-i)
			i = end
			continue
		}

		if ch == '\'' {
			if lang == langRust && !isRustCharLiteral(line, i) {
				codeOnly.WriteByte('\'')
				codeNoStr.WriteByte('\'')
				i++
				continue
			}
			allowBackslash := lang != langSQL
			end := scanQuotedLiteral(line, i, '\'', allowBackslash)
			codeOnly.WriteString(line[i:end])
			writeSpaces(&codeNoStr, end-i)
			i = end
			continue
		}

		codeOnly.WriteByte(ch)
		codeNoStr.WriteByte(ch)
		i++
	}

	return codeOnly.String(), codeNoStr.String()
}

func scanQuotedLiteral(line string, start int, quote byte, allowBackslash bool) int {
	n := len(line)
	i := start + 1
	for i < n {
		if allowBackslash && line[i] == '\\' && i+1 < n {
			i += 2
			continue
		}
		if line[i] == quote {
			if !allowBackslash && i+1 < n && line[i+1] == quote {
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return n
}

func isRustCharLiteral(line string, idx int) bool {
	n := len(line)
	if idx+2 < n && line[idx+1] != '\\' && line[idx+1] != '\'' && line[idx+2] == '\'' {
		return true
	}
	if idx+1 < n && line[idx+1] == '\\' {
		limit := idx + 12
		if limit > n {
			limit = n
		}
		for j := idx + 2; j < limit; j++ {
			if line[j] == '\'' {
				return true
			}
		}
	}
	return false
}

func writeSpaces(b *strings.Builder, count int) {
	for i := 0; i < count; i++ {
		b.WriteByte(' ')
	}
}

func collectCalls(codeNoStr string, lang langKind, counts map[string]int) {
	n := len(codeNoStr)
	i := 0
	for i < n {
		if !isIdentStart(codeNoStr[i]) {
			i++
			continue
		}
		start := i
		i++
		for i < n && isIdentChar(codeNoStr[i]) {
			i++
		}
		ident := codeNoStr[start:i]
		j := i
		for j < n && (codeNoStr[j] == ' ' || codeNoStr[j] == '\t') {
			j++
		}
		if j < n && codeNoStr[j] == '(' {
			if !isKeywordForLang(ident, lang) {
				counts[ident]++
			}
		}
	}
}

func buildTopCalls(counts map[string]int) []CallMetric {
	if len(counts) == 0 {
		return []CallMetric{}
	}
	metrics := make([]CallMetric, 0, len(counts))
	for name, count := range counts {
		metrics = append(metrics, CallMetric{Name: name, Count: count})
	}
	sort.Slice(metrics, func(i, j int) bool {
		if metrics[i].Count != metrics[j].Count {
			return metrics[i].Count > metrics[j].Count
		}
		return metrics[i].Name < metrics[j].Name
	})
	if len(metrics) > 25 {
		metrics = metrics[:25]
	}
	return metrics
}

func extractQuotedString(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '"' || s[i] == '\'' || s[i] == '`' {
			q := s[i]
			end := scanQuotedLiteral(s, i, q, q != '`')
			if end > i+1 && end <= len(s) && s[end-1] == q {
				return s[i+1 : end-1]
			}
			return ""
		}
	}
	return ""
}

func extractAfterFrom(code string) string {
	idx := findWordIndex(code, "from")
	if idx < 0 {
		return ""
	}
	return extractQuotedString(code[idx+4:])
}

func findWordIndex(s string, word string) int {
	offset := 0
	for {
		idx := strings.Index(s[offset:], word)
		if idx < 0 {
			return -1
		}
		pos := offset + idx
		leftOK := pos == 0 || !isIdentChar(s[pos-1])
		rightPos := pos + len(word)
		rightOK := rightPos >= len(s) || !isIdentChar(s[rightPos])
		if leftOK && rightOK {
			return pos
		}
		offset = pos + len(word)
	}
}

func extractIncludeTarget(rest string) string {
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "<") {
		if end := strings.IndexByte(rest[1:], '>'); end >= 0 {
			return strings.TrimSpace(rest[1 : end+1])
		}
	}
	return extractQuotedString(rest)
}

func hasKeywordPrefix(s string, kw string) bool {
	if !strings.HasPrefix(s, kw) {
		return false
	}
	if len(s) == len(kw) {
		return true
	}
	return !isIdentChar(s[len(kw)])
}

func hasWordUpper(s string, kw string) bool {
	if !strings.HasPrefix(s, kw) {
		return false
	}
	if len(s) == len(kw) {
		return true
	}
	return !isIdentChar(s[len(kw)])
}

func readIdent(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" || !isIdentStart(s[0]) {
		return "", s
	}
	i := 1
	for i < len(s) && isIdentChar(s[i]) {
		i++
	}
	return s[:i], s[i:]
}

func readSQLObjectName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	i := 0
	for i < len(s) {
		ch := s[i]
		if ch == '"' || ch == '`' || ch == '[' {
			closeCh := ch
			if ch == '[' {
				closeCh = ']'
			}
			i++
			for i < len(s) && s[i] != closeCh {
				b.WriteByte(s[i])
				i++
			}
			if i < len(s) && s[i] == closeCh {
				i++
			}
			continue
		}
		if isIdentChar(ch) || ch == '.' {
			b.WriteByte(ch)
			i++
			continue
		}
		break
	}
	return b.String()
}

func readDollarTag(s string) (string, int, bool) {
	if len(s) < 2 || s[0] != '$' {
		return "", 0, false
	}
	if s[1] == '$' {
		return "$$", 2, true
	}
	if !isIdentStart(s[1]) {
		return "", 0, false
	}
	i := 2
	for i < len(s) && isIdentChar(s[i]) {
		i++
	}
	if i < len(s) && s[i] == '$' {
		return s[:i+1], i + 1, true
	}
	return "", 0, false
}

func skipBalanced(s string, openCh byte, closeCh byte) string {
	if s == "" || s[0] != openCh {
		return s
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		if s[i] == openCh {
			depth++
		} else if s[i] == closeCh {
			depth--
			if depth == 0 {
				return s[i+1:]
			}
		}
	}
	return s
}

func skipBracketGroup(s string, openCh byte, closeCh byte) string {
	if strings.HasPrefix(s, string(openCh)) {
		return skipBalanced(s, openCh, closeCh)
	}
	return s
}

func stripAngleGenerics(s string) string {
	var b strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteByte(s[i])
			}
		}
	}
	return b.String()
}

func findDeclOpenParen(line string) int {
	angleDepth := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '<':
			angleDepth++
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case '(':
			if angleDepth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitHeaderTokens(s string) []string {
	s = stripAngleGenerics(s)
	var tokens []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if isIdentChar(ch) || ch == ':' || ch == '~' {
			cur.WriteByte(ch)
			continue
		}
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

func measureIndent(rawLine string) int {
	indent := 0
	for i := 0; i < len(rawLine); i++ {
		switch rawLine[i] {
		case ' ':
			indent++
		case '\t':
			indent += 4
		default:
			return indent
		}
	}
	return indent
}

func isIdentStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func isIdentChar(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9')
}

func isDeclarationExcludeStarter(word string) bool {
	switch word {
	case "return", "throw", "await", "yield", "new", "delete", "case", "else",
		"if", "for", "foreach", "while", "switch", "catch", "using", "lock",
		"synchronized", "typedef", "namespace", "package", "import", "export":
		return true
	default:
		return false
	}
}

func isTSJSKeyword(word string) bool {
	switch word {
	case "if", "else", "for", "while", "do", "switch", "case", "default",
		"try", "catch", "finally", "return", "throw", "new", "delete",
		"typeof", "instanceof", "void", "await", "async", "yield", "import",
		"export", "from", "class", "interface", "type", "enum", "function",
		"const", "let", "var", "super", "this", "with", "break", "continue":
		return true
	default:
		return false
	}
}

func isKeywordForLang(word string, lang langKind) bool {
	if lang == langSQL {
		switch strings.ToUpper(word) {
		case "SELECT", "INSERT", "INTO", "VALUES", "UPDATE", "SET", "DELETE",
			"FROM", "WHERE", "AND", "OR", "NOT", "IN", "EXISTS", "AS", "ON",
			"USING", "JOIN", "INNER", "LEFT", "RIGHT", "FULL", "OUTER", "CROSS",
			"GROUP", "BY", "HAVING", "ORDER", "LIMIT", "OFFSET", "CREATE",
			"TABLE", "VIEW", "FUNCTION", "POLICY", "RETURNS", "CHECK",
			"REFERENCES", "PRIMARY", "KEY", "UNIQUE", "FOREIGN", "CONSTRAINT",
			"CASE", "WHEN", "THEN", "ELSE", "END", "IF", "FOR", "ALL",
			"VARCHAR", "CHAR", "INT", "INTEGER", "BIGINT", "SMALLINT",
			"NUMERIC", "DECIMAL", "TEXT", "BOOLEAN", "TIMESTAMP", "DATE",
			"ARRAY", "ENUM", "DEFAULT", "NULL", "TRUE", "FALSE":
			return true
		default:
			return false
		}
	}

	switch word {
	case "if", "else", "elif", "for", "foreach", "while", "do", "switch",
		"case", "default", "match", "select", "try", "catch", "except",
		"finally", "with", "using", "lock", "synchronized", "return",
		"yield", "throw", "raise", "break", "continue", "goto", "defer",
		"go", "await", "async", "func", "function", "fn", "def", "class",
		"struct", "interface", "trait", "impl", "enum", "type", "mixin",
		"extension", "namespace", "package", "module", "mod", "import",
		"export", "from", "use", "include", "require", "pub", "public",
		"private", "protected", "internal", "static", "const", "let", "var",
		"val", "final", "mut", "ref", "out", "in", "is", "as", "of", "new",
		"delete", "sizeof", "typeof", "alignof", "decltype", "offsetof",
		"instanceof", "lambda", "pass", "assert", "and", "or", "not", "del",
		"global", "nonlocal", "super", "this", "self", "Self", "crate",
		"loop", "move", "dyn", "where", "when", "void", "true", "false",
		"null", "nullptr", "None", "True", "False", "undefined", "range",
		"chan", "map", "throws", "extends", "implements", "abstract",
		"override", "virtual", "explicit", "friend", "operator", "template",
		"typename", "constexpr", "noexcept", "static_assert", "typedef",
		"union", "extern", "volatile", "register", "inline", "restrict",
		"auto", "fallthrough":
		return true
	}

	if lang == langC || lang == langCPP {
		switch word {
		case "int", "char", "short", "long", "float", "double", "signed", "unsigned", "bool":
			return true
		}
	}
	return false
}
