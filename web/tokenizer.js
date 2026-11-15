(function (global) {
  "use strict";

  const SLASH_SLASH = String.fromCharCode(47, 47);
  const SLASH_STAR = String.fromCharCode(47, 42);
  const STAR_SLASH = String.fromCharCode(42, 47);
  const HTML_COMM_START = String.fromCharCode(60, 33, 45, 45);
  const HTML_COMM_END = String.fromCharCode(45, 45, 62);

  const PALETTE = {
    keyword: "#e07a5f",
    type: "#e5a93c",
    func: "#8ab4c9",
    string: "#9ab87a",
    number: "#d49b6a",
    comment: "#827f77",
    operator: "#c9c3b8",
    punct: "#9b968c",
    plain: "#e8e4dc"
  };

  const LANG_ALIASES = {
    go: "go",
    golang: "go",
    rust: "rust",
    rs: "rust",
    typescript: "typescript",
    ts: "typescript",
    tsx: "typescript",
    mts: "typescript",
    javascript: "javascript",
    js: "javascript",
    jsx: "javascript",
    mjs: "javascript",
    cjs: "javascript",
    dart: "dart",
    python: "python",
    py: "python",
    c: "c",
    "c++": "cpp",
    cpp: "cpp",
    cc: "cpp",
    cxx: "cpp",
    "c#": "csharp",
    csharp: "csharp",
    cs: "csharp",
    java: "java",
    html: "html",
    htm: "html",
    xml: "html",
    svg: "html",
    css: "css",
    scss: "css",
    sass: "css",
    json: "json",
    arb: "json",
    yaml: "yaml",
    yml: "yaml",
    markdown: "markdown",
    md: "markdown",
    mdx: "markdown",
    sql: "sql",
    shell: "shell",
    sh: "shell",
    bash: "shell",
    toml: "yaml"
  };

  const KEYWORDS = {
    go: new Set([
      "break", "case", "chan", "const", "continue", "default", "defer", "else",
      "fallthrough", "for", "func", "go", "goto", "if", "import", "interface",
      "map", "package", "range", "return", "select", "struct", "switch", "type", "var"
    ]),
    rust: new Set([
      "as", "async", "await", "break", "const", "continue", "crate", "dyn", "else",
      "enum", "extern", "fn", "for", "if", "impl", "in", "let", "loop", "match",
      "mod", "move", "mut", "pub", "ref", "return", "self", "Self", "static",
      "struct", "super", "trait", "type", "unsafe", "use", "where", "while", "macro_rules"
    ]),
    typescript: new Set([
      "abstract", "any", "as", "async", "await", "break", "case", "catch", "class",
      "const", "continue", "debugger", "declare", "default", "delete", "do", "else",
      "enum", "export", "extends", "finally", "for", "from", "function", "get", "if",
      "implements", "import", "in", "infer", "instanceof", "interface", "is", "keyof",
      "let", "module", "namespace", "new", "of", "override", "package", "private",
      "protected", "public", "readonly", "return", "satisfies", "set", "static",
      "super", "switch", "this", "throw", "try", "type", "typeof", "var", "void",
      "while", "with", "yield"
    ]),
    javascript: new Set([
      "async", "await", "break", "case", "catch", "class", "const", "continue",
      "debugger", "default", "delete", "do", "else", "export", "extends", "finally",
      "for", "from", "function", "get", "if", "import", "in", "instanceof", "let",
      "new", "of", "return", "set", "static", "super", "switch", "this", "throw",
      "try", "typeof", "var", "void", "while", "with", "yield"
    ]),
    dart: new Set([
      "abstract", "as", "assert", "async", "await", "base", "break", "case", "catch",
      "class", "const", "continue", "covariant", "default", "deferred", "do", "else",
      "enum", "export", "extends", "extension", "external", "factory", "final",
      "finally", "for", "get", "hide", "if", "implements", "import", "in", "interface",
      "is", "late", "library", "mixin", "new", "on", "operator", "part", "required",
      "rethrow", "return", "sealed", "set", "show", "static", "super", "switch",
      "sync", "this", "throw", "try", "typedef", "var", "void", "when", "while",
      "with", "yield"
    ]),
    python: new Set([
      "and", "as", "assert", "async", "await", "break", "class", "continue", "def",
      "del", "elif", "else", "except", "finally", "for", "from", "global", "if",
      "import", "in", "is", "lambda", "match", "case", "nonlocal", "not", "or",
      "pass", "raise", "return", "try", "while", "with", "yield"
    ]),
    c: new Set([
      "auto", "break", "case", "const", "continue", "default", "do", "else", "enum",
      "extern", "for", "goto", "if", "inline", "register", "restrict", "return",
      "sizeof", "static", "struct", "switch", "typedef", "union", "volatile", "while",
      "_Alignas", "_Alignof", "_Atomic", "_Bool", "_Complex", "_Generic", "_Imaginary",
      "_Noreturn", "_Static_assert", "_Thread_local"
    ]),
    cpp: new Set([
      "alignas", "alignof", "and", "asm", "auto", "break", "case", "catch", "class",
      "concept", "const", "consteval", "constexpr", "constinit", "const_cast",
      "continue", "co_await", "co_return", "co_yield", "decltype", "default", "delete",
      "do", "dynamic_cast", "else", "enum", "explicit", "export", "extern", "for",
      "friend", "goto", "if", "inline", "mutable", "namespace", "new", "noexcept",
      "not", "operator", "or", "private", "protected", "public", "register",
      "reinterpret_cast", "requires", "return", "sizeof", "static", "static_assert",
      "static_cast", "struct", "switch", "template", "this", "thread_local", "throw",
      "try", "typedef", "typeid", "typename", "union", "using", "virtual", "volatile",
      "while"
    ]),
    csharp: new Set([
      "abstract", "as", "async", "await", "base", "break", "case", "catch", "checked",
      "class", "const", "continue", "default", "delegate", "do", "else", "enum",
      "event", "explicit", "extern", "finally", "fixed", "for", "foreach", "get",
      "goto", "if", "implicit", "in", "init", "interface", "internal", "is", "lock",
      "namespace", "new", "operator", "out", "override", "params", "private",
      "protected", "public", "readonly", "record", "ref", "required", "return",
      "sealed", "set", "sizeof", "stackalloc", "static", "struct", "switch", "this",
      "throw", "try", "typeof", "unchecked", "unsafe", "using", "var", "virtual",
      "void", "volatile", "where", "while", "yield"
    ]),
    java: new Set([
      "abstract", "assert", "break", "case", "catch", "class", "const", "continue",
      "default", "do", "else", "enum", "extends", "final", "finally", "for", "goto",
      "if", "implements", "import", "instanceof", "interface", "native", "new",
      "non-sealed", "package", "permits", "private", "protected", "public", "record",
      "return", "sealed", "static", "strictfp", "super", "switch", "synchronized",
      "this", "throw", "throws", "transient", "try", "var", "void", "volatile",
      "while", "yield"
    ]),
    sql: new Set([
      "select", "from", "where", "insert", "into", "values", "update", "set", "delete",
      "create", "alter", "drop", "table", "view", "index", "function", "procedure",
      "trigger", "policy", "database", "schema", "if", "not", "exists", "primary",
      "key", "foreign", "references", "unique", "check", "default", "constraint",
      "join", "inner", "left", "right", "full", "outer", "cross", "on", "as", "and",
      "or", "in", "is", "like", "ilike", "between", "order", "by", "group", "having",
      "limit", "offset", "union", "all", "distinct", "case", "when", "then", "else",
      "end", "asc", "desc", "returning", "with", "begin", "commit", "rollback",
      "grant", "revoke", "using", "for", "cascade", "replace", "temp", "temporary"
    ]),
    shell: new Set([
      "if", "then", "else", "elif", "fi", "case", "esac", "for", "while", "until",
      "do", "done", "in", "function", "return", "exit", "export", "local", "readonly",
      "unset", "shift", "source", "trap", "break", "continue"
    ])
  };

  const TYPES = {
    go: new Set([
      "any", "bool", "byte", "comparable", "complex64", "complex128", "error",
      "float32", "float64", "int", "int8", "int16", "int32", "int64", "rune",
      "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr"
    ]),
    rust: new Set([
      "bool", "char", "f32", "f64", "i8", "i16", "i32", "i64", "i128", "isize",
      "str", "u8", "u16", "u32", "u64", "u128", "usize", "String", "Vec", "Option",
      "Result", "Box", "Rc", "Arc", "HashMap", "HashSet", "BTreeMap", "BTreeSet"
    ]),
    typescript: new Set([
      "Array", "bigint", "boolean", "Date", "Error", "Function", "Map", "never",
      "number", "object", "Promise", "Record", "RegExp", "Set", "string", "symbol",
      "unknown", "void", "Partial", "Required", "Readonly", "Pick", "Omit"
    ]),
    javascript: new Set([
      "Array", "BigInt", "Boolean", "Date", "Error", "Function", "Map", "Math",
      "Number", "Object", "Promise", "Proxy", "Reflect", "RegExp", "Set", "String",
      "Symbol", "WeakMap", "WeakSet", "Uint8Array", "Float32Array", "Float64Array"
    ]),
    dart: new Set([
      "bool", "double", "dynamic", "Future", "int", "Iterable", "List", "Map",
      "Never", "num", "Object", "Set", "Stream", "String", "Symbol", "Type", "void"
    ]),
    python: new Set([
      "bool", "bytes", "bytearray", "complex", "dict", "float", "frozenset", "int",
      "list", "memoryview", "object", "range", "set", "slice", "str", "tuple", "type",
      "Any", "Dict", "List", "Optional", "Sequence", "Set", "Tuple", "Union"
    ]),
    c: new Set([
      "char", "double", "float", "int", "long", "short", "signed", "unsigned", "void",
      "size_t", "ssize_t", "int8_t", "int16_t", "int32_t", "int64_t", "uint8_t",
      "uint16_t", "uint32_t", "uint64_t", "bool", "FILE"
    ]),
    cpp: new Set([
      "bool", "char", "char8_t", "char16_t", "char32_t", "double", "float", "int",
      "long", "short", "signed", "unsigned", "void", "wchar_t", "size_t", "int8_t",
      "int16_t", "int32_t", "int64_t", "uint8_t", "uint16_t", "uint32_t", "uint64_t",
      "string", "vector", "map", "unordered_map", "set", "unique_ptr", "shared_ptr"
    ]),
    csharp: new Set([
      "bool", "byte", "char", "decimal", "double", "dynamic", "float", "int", "long",
      "nint", "nuint", "object", "sbyte", "short", "string", "uint", "ulong", "ushort",
      "void", "Task", "List", "Dictionary", "IEnumerable"
    ]),
    java: new Set([
      "boolean", "byte", "char", "double", "float", "int", "long", "short", "void",
      "Boolean", "Byte", "Character", "Double", "Float", "Integer", "List", "Long",
      "Map", "Object", "Optional", "Set", "Short", "String", "Void"
    ]),
    sql: new Set([
      "int", "integer", "bigint", "smallint", "serial", "bigserial", "text", "varchar",
      "char", "boolean", "bool", "timestamp", "timestamptz", "date", "time", "uuid",
      "json", "jsonb", "numeric", "decimal", "real", "double", "precision", "bytea",
      "float", "blob"
    ])
  };

  const CONSTANTS = new Set([
    "true", "false", "null", "nil", "None", "True", "False", "undefined", "NaN",
    "Infinity", "nullptr", "NULL", "TRUE", "FALSE", "iota"
  ]);

  function normalizeLang(language) {
    if (!language) return "plain";
    const key = String(language).trim().toLowerCase();
    return LANG_ALIASES[key] || "plain";
  }

  function pushToken(tokens, text, type) {
    if (!text) return;
    const len = tokens.length;
    if (len > 0 && tokens[len - 1].type === type && type === "plain") {
      tokens[len - 1].text += text;
      return;
    }
    tokens.push({ text: text, type: type });
  }

  function isIdentStart(ch) {
    const c = ch.charCodeAt(0);
    return (c >= 65 && c <= 90) || (c >= 97 && c <= 122) || c === 95 || c === 36;
  }

  function isIdentChar(ch) {
    const c = ch.charCodeAt(0);
    return (c >= 65 && c <= 90) || (c >= 97 && c <= 122) || (c >= 48 && c <= 57) || c === 95 || c === 36;
  }

  function isDigit(ch) {
    const c = ch.charCodeAt(0);
    return c >= 48 && c <= 57;
  }

  function isHexDigit(ch) {
    const c = ch.charCodeAt(0);
    return (c >= 48 && c <= 57) || (c >= 65 && c <= 70) || (c >= 97 && c <= 102);
  }

  function isWhitespace(ch) {
    return ch === " " || ch === "\t" || ch === "\r";
  }

  function scanQuoted(line, start, quoteChar) {
    let i = start + 1;
    while (i < line.length) {
      const ch = line[i];
      if (ch === "\\" && quoteChar !== "`") {
        i += 2;
        continue;
      }
      if (ch === "\\" && quoteChar === "`") {
        i += 2;
        continue;
      }
      if (ch === quoteChar) {
        return i + 1;
      }
      i += 1;
    }
    return line.length;
  }

  function scanNumber(line, start) {
    let i = start;
    if (line[i] === "0" && i + 1 < line.length) {
      const next = line[i + 1].toLowerCase();
      if (next === "x") {
        i += 2;
        while (i < line.length && (isHexDigit(line[i]) || line[i] === "_")) i += 1;
        while (i < line.length && "uUlLfF".indexOf(line[i]) !== -1) i += 1;
        return i;
      }
      if (next === "b" || next === "o") {
        i += 2;
        while (i < line.length && (isDigit(line[i]) || line[i] === "_")) i += 1;
        while (i < line.length && "uUlLfF".indexOf(line[i]) !== -1) i += 1;
        return i;
      }
    }
    while (i < line.length && (isDigit(line[i]) || line[i] === "_")) i += 1;
    if (i < line.length && line[i] === "." && i + 1 < line.length && isDigit(line[i + 1])) {
      i += 1;
      while (i < line.length && (isDigit(line[i]) || line[i] === "_")) i += 1;
    }
    if (i < line.length && (line[i] === "e" || line[i] === "E")) {
      let j = i + 1;
      if (j < line.length && (line[j] === "+" || line[j] === "-")) j += 1;
      if (j < line.length && isDigit(line[j])) {
        i = j;
        while (i < line.length && (isDigit(line[i]) || line[i] === "_")) i += 1;
      }
    }
    while (i < line.length && "uUlLfFiIdD".indexOf(line[i]) !== -1) i += 1;
    return i;
  }

  function nextNonSpaceChar(line, pos) {
    let i = pos;
    while (i < line.length && isWhitespace(line[i])) i += 1;
    return i < line.length ? line[i] : "";
  }

  function isPascalCase(word) {
    if (word.length < 2) return false;
    const c0 = word.charCodeAt(0);
    if (c0 < 65 || c0 > 90) return false;
    for (let i = 1; i < word.length; i += 1) {
      const c = word.charCodeAt(i);
      if (c >= 97 && c <= 122) return true;
    }
    return false;
  }

  function isAllUpper(word) {
    if (word.length < 2) return false;
    let hasAlpha = false;
    for (let i = 0; i < word.length; i += 1) {
      const c = word.charCodeAt(i);
      if (c >= 97 && c <= 122) return false;
      if (c >= 65 && c <= 90) hasAlpha = true;
    }
    return hasAlpha;
  }

  function tokenizeJson(line) {
    const tokens = [];
    let i = 0;
    const len = line.length;
    while (i < len) {
      const ch = line[i];
      if (isWhitespace(ch)) {
        let j = i + 1;
        while (j < len && isWhitespace(line[j])) j += 1;
        pushToken(tokens, line.slice(i, j), "plain");
        i = j;
        continue;
      }
      if (ch === "\"") {
        const end = scanQuoted(line, i, "\"");
        const next = nextNonSpaceChar(line, end);
        pushToken(tokens, line.slice(i, end), next === ":" ? "type" : "string");
        i = end;
        continue;
      }
      if (isDigit(ch) || (ch === "-" && i + 1 < len && isDigit(line[i + 1]))) {
        let j = ch === "-" ? i + 1 : i;
        j = scanNumber(line, j);
        pushToken(tokens, line.slice(i, j), "number");
        i = j;
        continue;
      }
      if ("{}[]:,".indexOf(ch) !== -1) {
        pushToken(tokens, ch, "punct");
        i += 1;
        continue;
      }
      if (isIdentStart(ch)) {
        let j = i + 1;
        while (j < len && isIdentChar(line[j])) j += 1;
        const word = line.slice(i, j);
        const type = (word === "true" || word === "false" || word === "null") ? "number" : "plain";
        pushToken(tokens, word, type);
        i = j;
        continue;
      }
      pushToken(tokens, ch, "plain");
      i += 1;
    }
    return tokens;
  }

  function tokenizeHtml(line) {
    const tokens = [];
    let i = 0;
    const len = line.length;
    while (i < len) {
      if (line.startsWith(HTML_COMM_START, i)) {
        const endIdx = line.indexOf(HTML_COMM_END, i + 4);
        const end = endIdx === -1 ? len : endIdx + 3;
        pushToken(tokens, line.slice(i, end), "comment");
        i = end;
        continue;
      }
      const ch = line[i];
      if (ch === "<") {
        let j = i + 1;
        if (j < len && (line[j] === "/" || line[j] === "!")) j += 1;
        pushToken(tokens, line.slice(i, j), "punct");
        i = j;
        let nameStart = i;
        while (i < len && (isIdentChar(line[i]) || line[i] === "-" || line[i] === ":")) i += 1;
        if (i > nameStart) {
          pushToken(tokens, line.slice(nameStart, i), "keyword");
        }
        while (i < len && line[i] !== ">") {
          const inner = line[i];
          if (isWhitespace(inner)) {
            let ws = i + 1;
            while (ws < len && isWhitespace(line[ws])) ws += 1;
            pushToken(tokens, line.slice(i, ws), "plain");
            i = ws;
          } else if (inner === "\"" || inner === "'") {
            const qEnd = scanQuoted(line, i, inner);
            pushToken(tokens, line.slice(i, qEnd), "string");
            i = qEnd;
          } else if (inner === "=" || inner === "/") {
            pushToken(tokens, inner, inner === "=" ? "operator" : "punct");
            i += 1;
          } else if (isIdentStart(inner)) {
            let aEnd = i + 1;
            while (aEnd < len && (isIdentChar(line[aEnd]) || line[aEnd] === "-" || line[aEnd] === ":")) aEnd += 1;
            pushToken(tokens, line.slice(i, aEnd), "type");
            i = aEnd;
          } else {
            pushToken(tokens, inner, "plain");
            i += 1;
          }
        }
        if (i < len && line[i] === ">") {
          pushToken(tokens, ">", "punct");
          i += 1;
        }
        continue;
      }
      if (ch === "&") {
        const semi = line.indexOf(";", i);
        if (semi !== -1 && semi - i <= 10) {
          pushToken(tokens, line.slice(i, semi + 1), "number");
          i = semi + 1;
          continue;
        }
      }
      let j = i + 1;
      while (j < len && line[j] !== "<" && line[j] !== "&") j += 1;
      pushToken(tokens, line.slice(i, j), "plain");
      i = j;
    }
    return tokens;
  }

  function tokenizeCss(line) {
    const tokens = [];
    let i = 0;
    const len = line.length;
    while (i < len) {
      if (line.startsWith(SLASH_STAR, i)) {
        const endIdx = line.indexOf(STAR_SLASH, i + 2);
        const end = endIdx === -1 ? len : endIdx + 2;
        pushToken(tokens, line.slice(i, end), "comment");
        i = end;
        continue;
      }
      if (line.startsWith(SLASH_SLASH, i)) {
        pushToken(tokens, line.slice(i), "comment");
        break;
      }
      const ch = line[i];
      if (isWhitespace(ch)) {
        let j = i + 1;
        while (j < len && isWhitespace(line[j])) j += 1;
        pushToken(tokens, line.slice(i, j), "plain");
        i = j;
        continue;
      }
      if (ch === "\"" || ch === "'") {
        const end = scanQuoted(line, i, ch);
        pushToken(tokens, line.slice(i, end), "string");
        i = end;
        continue;
      }
      if (ch === "@") {
        let j = i + 1;
        while (j < len && (isIdentChar(line[j]) || line[j] === "-")) j += 1;
        pushToken(tokens, line.slice(i, j), "keyword");
        i = j;
        continue;
      }
      if (ch === "#") {
        let j = i + 1;
        while (j < len && (isHexDigit(line[j]) || isIdentChar(line[j]) || line[j] === "-")) j += 1;
        const hexBody = line.slice(i + 1, j);
        const isHexColor = (hexBody.length === 3 || hexBody.length === 4 || hexBody.length === 6 || hexBody.length === 8) &&
          Array.prototype.every.call(hexBody, isHexDigit);
        pushToken(tokens, line.slice(i, j), isHexColor ? "number" : "type");
        i = j;
        continue;
      }
      if (isDigit(ch) || (ch === "." && i + 1 < len && isDigit(line[i + 1]))) {
        let j = scanNumber(line, i);
        while (j < len && ((line[j] >= "a" && line[j] <= "z") || line[j] === "%")) j += 1;
        pushToken(tokens, line.slice(i, j), "number");
        i = j;
        continue;
      }
      if (ch === "." && i + 1 < len && isIdentStart(line[i + 1])) {
        let j = i + 1;
        while (j < len && (isIdentChar(line[j]) || line[j] === "-")) j += 1;
        pushToken(tokens, line.slice(i, j), "type");
        i = j;
        continue;
      }
      if ("{}()[];:,".indexOf(ch) !== -1) {
        pushToken(tokens, ch, "punct");
        i += 1;
        continue;
      }
      if (isIdentStart(ch) || (ch === "-" && i + 1 < len && (isIdentStart(line[i + 1]) || line[i + 1] === "-"))) {
        let j = i + 1;
        while (j < len && (isIdentChar(line[j]) || line[j] === "-")) j += 1;
        const word = line.slice(i, j);
        const next = nextNonSpaceChar(line, j);
        let type = "plain";
        if (next === "(") type = "func";
        else if (next === ":") type = "type";
        else if (CONSTANTS.has(word)) type = "number";
        pushToken(tokens, word, type);
        i = j;
        continue;
      }
      if ("/+-=<>!~&|*".indexOf(ch) !== -1) {
        pushToken(tokens, ch, "operator");
        i += 1;
        continue;
      }
      pushToken(tokens, ch, "plain");
      i += 1;
    }
    return tokens;
  }

  function tokenizeYaml(line) {
    const tokens = [];
    const len = line.length;
    let i = 0;
    while (i < len && isWhitespace(line[i])) i += 1;
    if (i > 0) pushToken(tokens, line.slice(0, i), "plain");
    if (i >= len) return tokens;

    if (line[i] === "#") {
      pushToken(tokens, line.slice(i), "comment");
      return tokens;
    }
    if (line.startsWith("---", i) || line.startsWith("...", i)) {
      pushToken(tokens, line.slice(i, i + 3), "punct");
      i += 3;
    }
    if (i < len && line[i] === "-" && (i + 1 === len || isWhitespace(line[i + 1]))) {
      pushToken(tokens, "-", "punct");
      i += 1;
      while (i < len && isWhitespace(line[i])) {
        pushToken(tokens, line[i], "plain");
        i += 1;
      }
    }

    const colonIdx = line.indexOf(":", i);
    if (colonIdx !== -1 && (colonIdx + 1 === len || isWhitespace(line[colonIdx + 1]))) {
      const keyPart = line.slice(i, colonIdx);
      if (keyPart.indexOf("#") === -1) {
        pushToken(tokens, keyPart, "type");
        pushToken(tokens, ":", "punct");
        i = colonIdx + 1;
      }
    }

    while (i < len) {
      const ch = line[i];
      if (ch === "#" && (i === 0 || isWhitespace(line[i - 1]))) {
        pushToken(tokens, line.slice(i), "comment");
        break;
      }
      if (isWhitespace(ch)) {
        let j = i + 1;
        while (j < len && isWhitespace(line[j])) j += 1;
        pushToken(tokens, line.slice(i, j), "plain");
        i = j;
        continue;
      }
      if (ch === "\"" || ch === "'") {
        const end = scanQuoted(line, i, ch);
        pushToken(tokens, line.slice(i, end), "string");
        i = end;
        continue;
      }
      if (isDigit(ch) || (ch === "-" && i + 1 < len && isDigit(line[i + 1]))) {
        let j = ch === "-" ? i + 1 : i;
        j = scanNumber(line, j);
        pushToken(tokens, line.slice(i, j), "number");
        i = j;
        continue;
      }
      if ("[]{}," .indexOf(ch) !== -1) {
        pushToken(tokens, ch, "punct");
        i += 1;
        continue;
      }
      if (isIdentStart(ch)) {
        let j = i + 1;
        while (j < len && (isIdentChar(line[j]) || line[j] === "-")) j += 1;
        const word = line.slice(i, j);
        const low = word.toLowerCase();
        const isBool = low === "true" || low === "false" || low === "null" || low === "yes" || low === "no" || low === "on" || low === "off";
        pushToken(tokens, word, isBool ? "number" : "string");
        i = j;
        continue;
      }
      pushToken(tokens, ch, "plain");
      i += 1;
    }
    return tokens;
  }

  function tokenizeMarkdown(line) {
    const tokens = [];
    let i = 0;
    const len = line.length;
    while (i < len && isWhitespace(line[i])) i += 1;
    if (i > 0) pushToken(tokens, line.slice(0, i), "plain");
    if (i >= len) return tokens;

    if (line[i] === "#") {
      let j = i;
      while (j < len && line[j] === "#") j += 1;
      if (j === len || isWhitespace(line[j])) {
        pushToken(tokens, line.slice(i, j), "keyword");
        if (j < len) pushToken(tokens, line.slice(j), "func");
        return tokens;
      }
    }
    if (line.startsWith("```", i) || line.startsWith("~~~", i)) {
      pushToken(tokens, line.slice(i), "comment");
      return tokens;
    }
    if (line[i] === ">") {
      pushToken(tokens, line.slice(i), "comment");
      return tokens;
    }
    if ((line[i] === "-" || line[i] === "*" || line[i] === "+") && i + 1 < len && isWhitespace(line[i + 1])) {
      pushToken(tokens, line[i], "keyword");
      i += 1;
    }

    while (i < len) {
      const ch = line[i];
      if (ch === "`") {
        let j = i + 1;
        while (j < len && line[j] !== "`") j += 1;
        if (j < len) j += 1;
        pushToken(tokens, line.slice(i, j), "string");
        i = j;
        continue;
      }
      if (ch === "[" || ch === "]") {
        pushToken(tokens, ch, "punct");
        i += 1;
        continue;
      }
      if (ch === "(" && i > 0 && line[i - 1] === "]") {
        const close = line.indexOf(")", i + 1);
        if (close !== -1) {
          pushToken(tokens, "(", "punct");
          pushToken(tokens, line.slice(i + 1, close), "string");
          pushToken(tokens, ")", "punct");
          i = close + 1;
          continue;
        }
      }
      if (ch === "*" && i + 1 < len && line[i + 1] === "*") {
        const close = line.indexOf("**", i + 2);
        if (close !== -1) {
          pushToken(tokens, line.slice(i, close + 2), "keyword");
          i = close + 2;
          continue;
        }
      }
      let j = i + 1;
      while (j < len && line[j] !== "`" && line[j] !== "[" && line[j] !== "]" && line[j] !== "*") j += 1;
      pushToken(tokens, line.slice(i, j), "plain");
      i = j;
    }
    return tokens;
  }

  function tokenizeCode(line, lang) {
    const tokens = [];
    const len = line.length;
    const kwSet = KEYWORDS[lang] || KEYWORDS.javascript;
    const typeSet = TYPES[lang] || null;
    const hasSlashSlash = lang !== "python" && lang !== "sql" && lang !== "shell";
    const hasSlashStar = lang !== "python" && lang !== "shell";
    const hasHashComment = lang === "python" || lang === "shell";
    const hasDashDash = lang === "sql";
    const hasPreprocessor = lang === "c" || lang === "cpp" || lang === "csharp";
    const hasBacktick = lang === "go" || lang === "javascript" || lang === "typescript" || lang === "sql";

    let i = 0;
    while (i < len) {
      const ch = line[i];

      if (isWhitespace(ch)) {
        let j = i + 1;
        while (j < len && isWhitespace(line[j])) j += 1;
        pushToken(tokens, line.slice(i, j), "plain");
        i = j;
        continue;
      }

      if (hasSlashSlash && line.startsWith(SLASH_SLASH, i)) {
        pushToken(tokens, line.slice(i), "comment");
        break;
      }

      if (hasSlashStar && line.startsWith(SLASH_STAR, i)) {
        const endIdx = line.indexOf(STAR_SLASH, i + 2);
        const end = endIdx === -1 ? len : endIdx + 2;
        pushToken(tokens, line.slice(i, end), "comment");
        i = end;
        continue;
      }

      if (hasHashComment && ch === "#") {
        pushToken(tokens, line.slice(i), "comment");
        break;
      }

      if (hasDashDash && ch === "-" && i + 1 < len && line[i + 1] === "-") {
        pushToken(tokens, line.slice(i), "comment");
        break;
      }

      if (hasPreprocessor && ch === "#") {
        let j = i + 1;
        while (j < len && isIdentChar(line[j])) j += 1;
        pushToken(tokens, line.slice(i, j), "keyword");
        i = j;
        continue;
      }

      if (lang === "rust" && ch === "#" && i + 1 < len && (line[i + 1] === "[" || line[i + 1] === "!")) {
        pushToken(tokens, ch, "punct");
        i += 1;
        continue;
      }

      if (ch === "@" && (lang === "python" || lang === "typescript" || lang === "javascript" || lang === "dart" || lang === "java")) {
        let j = i + 1;
        while (j < len && isIdentChar(line[j])) j += 1;
        pushToken(tokens, line.slice(i, j), "func");
        i = j;
        continue;
      }

      if (ch === "\"" || (hasBacktick && ch === "`")) {
        const end = scanQuoted(line, i, ch);
        pushToken(tokens, line.slice(i, end), "string");
        i = end;
        continue;
      }

      if (ch === "'") {
        if (lang === "rust" && i + 1 < len && isIdentStart(line[i + 1])) {
          const next2 = i + 2 < len ? line[i + 2] : "";
          if (next2 !== "'") {
            let j = i + 2;
            while (j < len && isIdentChar(line[j])) j += 1;
            pushToken(tokens, line.slice(i, j), "type");
            i = j;
            continue;
          }
        }
        const end = scanQuoted(line, i, "'");
        pushToken(tokens, line.slice(i, end), "string");
        i = end;
        continue;
      }

      if (isDigit(ch)) {
        const end = scanNumber(line, i);
        pushToken(tokens, line.slice(i, end), "number");
        i = end;
        continue;
      }

      if (isIdentStart(ch)) {
        let j = i + 1;
        while (j < len && isIdentChar(line[j])) j += 1;
        const word = line.slice(i, j);
        if ((lang === "python" || lang === "rust") && (word === "r" || word === "f" || word === "b" || word === "rf" || word === "fr") && j < len && (line[j] === "\"" || line[j] === "'")) {
          const end = scanQuoted(line, j, line[j]);
          pushToken(tokens, line.slice(i, end), "string");
          i = end;
          continue;
        }
        const low = lang === "sql" ? word.toLowerCase() : word;
        let type = "plain";
        if (kwSet.has(low)) {
          type = "keyword";
        } else if (CONSTANTS.has(word)) {
          type = "number";
        } else if (typeSet && typeSet.has(word)) {
          type = "type";
        } else if (nextNonSpaceChar(line, j) === "(") {
          type = "func";
        } else if (lang === "rust" && j < len && line[j] === "!") {
          type = "func";
        } else if (isAllUpper(word)) {
          type = "number";
        } else if (isPascalCase(word)) {
          type = "type";
        }
        pushToken(tokens, word, type);
        i = j;
        continue;
      }

      if ("()[]{},.;".indexOf(ch) !== -1) {
        pushToken(tokens, ch, "punct");
        i += 1;
        continue;
      }

      if ("/+-=!<>&|^~?:%*".indexOf(ch) !== -1) {
        let j = i + 1;
        while (j < len && "/+-=!<>&|^~?:%*".indexOf(line[j]) !== -1) {
          if (hasSlashSlash && line.startsWith(SLASH_SLASH, j)) break;
          if (hasSlashStar && line.startsWith(SLASH_STAR, j)) break;
          j += 1;
        }
        pushToken(tokens, line.slice(i, j), "operator");
        i = j;
        continue;
      }

      pushToken(tokens, ch, "plain");
      i += 1;
    }

    return tokens;
  }

  function tokenizeLine(line, language) {
    if (line === null || line === undefined || line === "") {
      return [];
    }
    const text = String(line);
    const lang = normalizeLang(language);
    if (lang === "json") return tokenizeJson(text);
    if (lang === "html") return tokenizeHtml(text);
    if (lang === "css") return tokenizeCss(text);
    if (lang === "yaml") return tokenizeYaml(text);
    if (lang === "markdown") return tokenizeMarkdown(text);
    if (lang === "plain") return [{ text: text, type: "plain" }];
    return tokenizeCode(text, lang);
  }

  function tokenColor(type) {
    return PALETTE[type] || PALETTE.plain;
  }

  global.ConfineTokenizer = {
    tokenizeLine: tokenizeLine,
    tokenColor: tokenColor
  };
})(typeof window !== "undefined" ? window : globalThis);
