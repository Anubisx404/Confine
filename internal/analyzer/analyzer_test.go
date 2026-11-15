package analyzer

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAnalyzeEmptyAndNonCode(t *testing.T) {
	cases := []struct {
		name     string
		language string
		content  string
	}{
		{name: "empty go", language: "Go", content: ""},
		{name: "markdown", language: "Markdown", content: "# Title\nfunc fake() {}\nfoo()\n"},
		{name: "json", language: "JSON", content: `{"func": "call()"}`},
		{name: "unknown", language: "", content: "func ignored() {}"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Analyze(tc.language, tc.content)
			if res.Symbols == nil || len(res.Symbols) != 0 {
				t.Fatalf("expected non-nil empty Symbols, got %#v", res.Symbols)
			}
			if res.Imports == nil || len(res.Imports) != 0 {
				t.Fatalf("expected non-nil empty Imports, got %#v", res.Imports)
			}
			if res.TopCalls == nil || len(res.TopCalls) != 0 {
				t.Fatalf("expected non-nil empty TopCalls, got %#v", res.TopCalls)
			}
			raw, err := json.Marshal(res)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}
			want := `{"symbols":[],"imports":[],"topCalls":[]}`
			if string(raw) != want {
				t.Fatalf("json = %s, want %s", string(raw), want)
			}
		})
	}
}

func TestAnalyzeGo(t *testing.T) {
	lineComment := "/" + "/"
	blockStart := "/" + "*"
	blockEnd := "*" + "/"

	src := strings.Join([]string{
		`package main`,
		``,
		`import "fmt"`,
		`import (`,
		`	"context"`,
		`	myhttp "net/http"`,
		`)`,
		``,
		lineComment + ` func fakeComment() { ignoredCall() }`,
		blockStart + ` type FakeStruct struct { fakeCall() } ` + blockEnd,
		``,
		`type Config struct {`,
		`	Port int`,
		`}`,
		``,
		`type Reader interface {`,
		`	Read(p []byte) (int, error)`,
		`	Close() error`,
		`}`,
		``,
		`type Box[T any] struct {`,
		`	Item T`,
		`}`,
		``,
		`func (c *Config) Validate() bool {`,
		`	raw := "` + lineComment + ` fakeInString()"`,
		`	backtick := ` + "`\nfunc fakeInRaw() {\n\tfakeRawCall()\n}\n`" + ``,
		`	_ = raw + backtick`,
		`	return c.Port > 0`,
		`}`,
		``,
		`func Run(ctx context.Context, srv *myhttp.Server) {`,
		`	fmt.Println("ok")`,
		`	fmt.Println("done")`,
		`	srv.ListenAndServe()`,
		`}`,
	}, "\n")

	res := Analyze("Go", src)

	wantImports := []ImportRef{
		{Target: "fmt", Line: 3},
		{Target: "context", Line: 5},
		{Target: "net/http", Line: 6},
	}
	assertImports(t, res.Imports, wantImports)

	assertSymbol(t, res.Symbols, "Config", "struct", 12, 14)
	assertSymbol(t, res.Symbols, "Reader", "interface", 16, 19)
	assertSymbol(t, res.Symbols, "Read", "method", 17, 17)
	assertSymbol(t, res.Symbols, "Close", "method", 18, 18)
	assertSymbol(t, res.Symbols, "Box", "struct", 21, 23)
	assertSymbol(t, res.Symbols, "Validate", "method", 25, 34)
	assertSymbol(t, res.Symbols, "Run", "function", 36, 40)

	assertNoSymbolNamed(t, res.Symbols, "fakeComment")
	assertNoSymbolNamed(t, res.Symbols, "FakeStruct")
	assertNoSymbolNamed(t, res.Symbols, "fakeInRaw")

	assertCallCount(t, res.TopCalls, "Println", 2)
	assertCallCount(t, res.TopCalls, "ListenAndServe", 1)
	assertCallAbsent(t, res.TopCalls, "ignoredCall")
	assertCallAbsent(t, res.TopCalls, "fakeCall")
	assertCallAbsent(t, res.TopCalls, "fakeInString")
	assertCallAbsent(t, res.TopCalls, "fakeRawCall")
	assertCallAbsent(t, res.TopCalls, "Validate")
	assertCallAbsent(t, res.TopCalls, "Run")
}

func TestAnalyzeRust(t *testing.T) {
	src := strings.Join([]string{
		`use std::collections::HashMap;`,
		`pub use crate::model::{Node, Edge};`,
		``,
		`pub struct Engine<T> {`,
		`    items: Vec<T>,`,
		`}`,
		``,
		`pub enum State {`,
		`    Idle,`,
		`    Running,`,
		`}`,
		``,
		`pub trait Analyzer {`,
		`    fn inspect(&self) -> usize;`,
		`}`,
		``,
		`impl<T> Analyzer for Engine<T> {`,
		`    fn inspect(&self) -> usize {`,
		`        self.items.len()`,
		`    }`,
		`}`,
		``,
		`pub(crate) async fn fetch<'a>(input: &'a str) -> &'a str {`,
		`    let ch = 'x';`,
		`    process(input)`,
		`}`,
	}, "\n")

	res := Analyze("Rust", src)

	assertImports(t, res.Imports, []ImportRef{
		{Target: "std::collections::HashMap", Line: 1},
		{Target: "crate::model::{Node, Edge}", Line: 2},
	})

	assertSymbol(t, res.Symbols, "Engine", "struct", 4, 6)
	assertSymbol(t, res.Symbols, "State", "enum", 8, 11)
	assertSymbol(t, res.Symbols, "Analyzer", "trait", 13, 15)
	assertSymbol(t, res.Symbols, "inspect", "method", 14, 14)
	assertSymbol(t, res.Symbols, "Analyzer for Engine", "impl", 17, 21)
	assertSymbol(t, res.Symbols, "inspect", "method", 18, 20)
	assertSymbol(t, res.Symbols, "fetch", "function", 23, 26)

	assertCallCount(t, res.TopCalls, "len", 1)
	assertCallCount(t, res.TopCalls, "process", 1)
}

func TestAnalyzeTypeScriptAndJavaScript(t *testing.T) {
	src := strings.Join([]string{
		`import React, { useState } from 'react';`,
		`import {`,
		`  NodeItem,`,
		`  EdgeItem,`,
		`} from "./types";`,
		`import "./polyfill";`,
		`const fs = require("fs");`,
		``,
		`export interface Props {`,
		`  title: string;`,
		`}`,
		``,
		`export type Status = "idle" | "busy";`,
		``,
		`export enum Role {`,
		`  Admin,`,
		`  Viewer,`,
		`}`,
		``,
		`export class Store {`,
		`  async load(id: string): Promise<void> {`,
		`    const data = fetchItem(id);`,
		`    if (isValid(data)) {`,
		`      saveItem(data);`,
		`    }`,
		`  }`,
		`}`,
		``,
		`export async function bootApp(cfg: Props) {`,
		`  initLogger(cfg.title);`,
		`}`,
		``,
		`export const add = (a: number, b: number): number => a + b;`,
		`const typedCb: (x: number) => void = (x) => {`,
		`  emitEvent(x);`,
		`};`,
	}, "\n")

	res := Analyze("TypeScript", src)

	assertImports(t, res.Imports, []ImportRef{
		{Target: "react", Line: 1},
		{Target: "./types", Line: 5},
		{Target: "./polyfill", Line: 6},
		{Target: "fs", Line: 7},
	})

	assertSymbol(t, res.Symbols, "Props", "interface", 9, 11)
	assertSymbol(t, res.Symbols, "Status", "type", 13, 13)
	assertSymbol(t, res.Symbols, "Role", "enum", 15, 18)
	assertSymbol(t, res.Symbols, "Store", "class", 20, 27)
	assertSymbol(t, res.Symbols, "load", "method", 21, 26)
	assertSymbol(t, res.Symbols, "bootApp", "function", 29, 31)
	assertSymbol(t, res.Symbols, "add", "function", 33, 33)
	assertSymbol(t, res.Symbols, "typedCb", "function", 34, 36)

	assertCallAbsent(t, res.TopCalls, "if")
	assertCallCount(t, res.TopCalls, "fetchItem", 1)
	assertCallCount(t, res.TopCalls, "isValid", 1)
	assertCallCount(t, res.TopCalls, "saveItem", 1)
	assertCallCount(t, res.TopCalls, "initLogger", 1)
	assertCallCount(t, res.TopCalls, "emitEvent", 1)
}

func TestAnalyzePython(t *testing.T) {
	src := strings.Join([]string{
		`import os, sys as system`,
		`from pathlib import Path`,
		`from .utils import clean`,
		``,
		`class Pipeline:`,
		`    """`,
		`    def fake_docstring_fn():`,
		`        ignored_doc_call()`,
		`    """`,
		`    def process(`,
		`        self,`,
		`        items: list[str],`,
		`    ) -> int:`,
		`        total = len(items)`,
		`        return total`,
		``,
		`    async def fetch(self, url: str):`,
		`        return await get_url(url)`,
		``,
		`def main():`,
		`    p = Pipeline()`,
		`    print(p.process(["a"]))`,
	}, "\n")

	res := Analyze("Python", src)

	assertImports(t, res.Imports, []ImportRef{
		{Target: "os", Line: 1},
		{Target: "sys", Line: 1},
		{Target: "pathlib", Line: 2},
		{Target: ".utils", Line: 3},
	})

	assertSymbol(t, res.Symbols, "Pipeline", "class", 5, 18)
	assertSymbol(t, res.Symbols, "process", "method", 10, 15)
	assertSymbol(t, res.Symbols, "fetch", "method", 17, 18)
	assertSymbol(t, res.Symbols, "main", "function", 20, 22)

	assertNoSymbolNamed(t, res.Symbols, "fake_docstring_fn")
	assertCallAbsent(t, res.TopCalls, "ignored_doc_call")
	assertCallCount(t, res.TopCalls, "len", 1)
	assertCallCount(t, res.TopCalls, "get_url", 1)
	assertCallCount(t, res.TopCalls, "Pipeline", 1)
	assertCallCount(t, res.TopCalls, "print", 1)
	assertCallCount(t, res.TopCalls, "process", 1)
}

func TestAnalyzeDartJavaCSharp(t *testing.T) {
	dartSrc := strings.Join([]string{
		`import 'package:flutter/material.dart';`,
		``,
		`enum LoadState { idle, ready }`,
		``,
		`mixin LoggerMixin {`,
		`  void logMsg(String msg) {`,
		`    debugPrint(msg);`,
		`  }`,
		`}`,
		``,
		`extension StringX on String {`,
		`  bool isBlank() => trim().isEmpty;`,
		`}`,
		``,
		`class DashboardPage extends StatelessWidget {`,
		`  Widget build(BuildContext context) {`,
		`    return Container();`,
		`  }`,
		`}`,
		``,
		`Future<void> bootstrapApp() async {`,
		`  runApp(DashboardPage());`,
		`}`,
	}, "\n")

	dartRes := Analyze("Dart", dartSrc)
	assertImports(t, dartRes.Imports, []ImportRef{{Target: "package:flutter/material.dart", Line: 1}})
	assertSymbol(t, dartRes.Symbols, "LoadState", "enum", 3, 3)
	assertSymbol(t, dartRes.Symbols, "LoggerMixin", "mixin", 5, 9)
	assertSymbol(t, dartRes.Symbols, "logMsg", "method", 6, 8)
	assertSymbol(t, dartRes.Symbols, "StringX", "extension", 11, 13)
	assertSymbol(t, dartRes.Symbols, "isBlank", "method", 12, 12)
	assertSymbol(t, dartRes.Symbols, "DashboardPage", "class", 15, 19)
	assertSymbol(t, dartRes.Symbols, "build", "method", 16, 18)
	assertSymbol(t, dartRes.Symbols, "bootstrapApp", "function", 21, 23)

	javaSrc := strings.Join([]string{
		`import java.util.List;`,
		`import static org.junit.Assert.assertEquals;`,
		``,
		`public interface Repository {`,
		`  void save(String item);`,
		`}`,
		``,
		`public class ServiceImpl implements Repository {`,
		`  public void save(String item) {`,
		`    persist(item);`,
		`  }`,
		`}`,
	}, "\n")

	javaRes := Analyze("Java", javaSrc)
	assertImports(t, javaRes.Imports, []ImportRef{
		{Target: "java.util.List", Line: 1},
		{Target: "org.junit.Assert.assertEquals", Line: 2},
	})
	assertSymbol(t, javaRes.Symbols, "Repository", "interface", 4, 6)
	assertSymbol(t, javaRes.Symbols, "ServiceImpl", "class", 8, 12)
	assertSymbol(t, javaRes.Symbols, "save", "method", 9, 11)

	csSrc := strings.Join([]string{
		`using System.Collections.Generic;`,
		`global using System.Text;`,
		``,
		`public struct Point`,
		`{`,
		`    public int X;`,
		`}`,
		``,
		`public class Worker`,
		`{`,
		`    public async Task<int> ExecuteAsync(int id)`,
		`    {`,
		`        using (var scope = OpenScope(id))`,
		`        {`,
		`            return await RunCore(scope);`,
		`        }`,
		`    }`,
		`}`,
	}, "\n")

	csRes := Analyze("C#", csSrc)
	assertImports(t, csRes.Imports, []ImportRef{
		{Target: "System.Collections.Generic", Line: 1},
		{Target: "System.Text", Line: 2},
	})
	assertSymbol(t, csRes.Symbols, "Point", "struct", 4, 7)
	assertSymbol(t, csRes.Symbols, "Worker", "class", 9, 18)
	assertSymbol(t, csRes.Symbols, "ExecuteAsync", "method", 11, 17)
	assertCallCount(t, csRes.TopCalls, "OpenScope", 1)
	assertCallCount(t, csRes.TopCalls, "RunCore", 1)
}

func TestAnalyzeCCpp(t *testing.T) {
	src := strings.Join([]string{
		`#include <stdio.h>`,
		`#include "parser.hpp"`,
		``,
		`typedef struct {`,
		`    int x;`,
		`} Point;`,
		``,
		`enum class TokenKind {`,
		`    Ident,`,
		`    Number`,
		`};`,
		``,
		`class Parser {`,
		`public:`,
		`    void parseToken(int tok) {`,
		`        consume(tok);`,
		`    }`,
		`};`,
		``,
		`void Parser::reset() {`,
		`    clearState();`,
		`}`,
		``,
		`int main(int argc, char** argv) {`,
		`    printf("hi");`,
		`    return 0;`,
		`}`,
	}, "\n")

	res := Analyze("C++", src)
	assertImports(t, res.Imports, []ImportRef{
		{Target: "stdio.h", Line: 1},
		{Target: "parser.hpp", Line: 2},
	})
	assertSymbol(t, res.Symbols, "Point", "struct", 4, 6)
	assertSymbol(t, res.Symbols, "TokenKind", "enum", 8, 11)
	assertSymbol(t, res.Symbols, "Parser", "class", 13, 18)
	assertSymbol(t, res.Symbols, "parseToken", "method", 15, 17)
	assertSymbol(t, res.Symbols, "Parser::reset", "method", 20, 22)
	assertSymbol(t, res.Symbols, "main", "function", 24, 27)
}

func TestAnalyzeSQL(t *testing.T) {
	blockStart := "/" + "*"
	blockEnd := "*" + "/"

	src := strings.Join([]string{
		`-- CREATE TABLE ignored_comment (id int);`,
		blockStart + ` CREATE VIEW ignored_block AS SELECT 1; ` + blockEnd,
		`CREATE TABLE IF NOT EXISTS public.users (`,
		`    id bigint PRIMARY KEY,`,
		`    email text NOT NULL`,
		`);`,
		``,
		`CREATE MATERIALIZED VIEW active_users AS`,
		`SELECT id, LOWER(email) FROM public.users;`,
		``,
		`CREATE OR REPLACE FUNCTION calc_total(order_id int)`,
		`RETURNS numeric AS $$`,
		`BEGIN`,
		`    RETURN 42;`,
		`END;`,
		`$$ LANGUAGE plpgsql;`,
		``,
		`CREATE POLICY user_isolation ON public.users`,
		`FOR ALL USING (auth_uid() = id);`,
	}, "\n")

	res := Analyze("SQL", src)
	assertSymbol(t, res.Symbols, "public.users", "table", 3, 6)
	assertSymbol(t, res.Symbols, "active_users", "view", 8, 9)
	assertSymbol(t, res.Symbols, "calc_total", "function", 11, 16)
	assertSymbol(t, res.Symbols, "user_isolation", "policy", 18, 19)
	assertNoSymbolNamed(t, res.Symbols, "ignored_comment")
	assertNoSymbolNamed(t, res.Symbols, "ignored_block")

	assertCallCount(t, res.TopCalls, "LOWER", 1)
	assertCallCount(t, res.TopCalls, "auth_uid", 1)
}

func TestTopCallsRankingAndCap(t *testing.T) {
	var lines []string
	lines = append(lines, "package bench")
	lines = append(lines, "func runner() {")
	for i := 0; i < 30; i++ {
		name := fmt.Sprintf("fn%02d", i)
		repeats := (i % 5) + 1
		if i == 28 {
			name = "alpha"
			repeats = 10
		}
		if i == 29 {
			name = "beta"
			repeats = 10
		}
		for r := 0; r < repeats; r++ {
			lines = append(lines, "\t"+name+"()")
		}
	}
	lines = append(lines, "\tif (true) {}")
	lines = append(lines, "\tfor (x) {}")
	lines = append(lines, "}")

	res := Analyze("Go", strings.Join(lines, "\n"))
	if len(res.TopCalls) != 25 {
		t.Fatalf("expected TopCalls capped at 25, got %d", len(res.TopCalls))
	}
	if res.TopCalls[0].Name != "alpha" || res.TopCalls[0].Count != 10 {
		t.Fatalf("rank 0 = %+v, want alpha:10", res.TopCalls[0])
	}
	if res.TopCalls[1].Name != "beta" || res.TopCalls[1].Count != 10 {
		t.Fatalf("rank 1 = %+v, want beta:10", res.TopCalls[1])
	}
	for i := 1; i < len(res.TopCalls); i++ {
		prev := res.TopCalls[i-1]
		cur := res.TopCalls[i]
		if prev.Count < cur.Count {
			t.Fatalf("not sorted descending at %d: %+v before %+v", i, prev, cur)
		}
		if prev.Count == cur.Count && prev.Name > cur.Name {
			t.Fatalf("tie-break not alphabetical at %d: %+v before %+v", i, prev, cur)
		}
	}
	assertCallAbsent(t, res.TopCalls, "if")
	assertCallAbsent(t, res.TopCalls, "for")
	assertCallAbsent(t, res.TopCalls, "runner")
}

func assertImports(t *testing.T, got []ImportRef, want []ImportRef) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("imports len = %d (%+v), want %d (%+v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("import[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func assertSymbol(t *testing.T, symbols []Symbol, name string, kind string, line int, endLine int) {
	t.Helper()
	for _, s := range symbols {
		if s.Name == name && s.Kind == kind && s.Line == line && s.EndLine == endLine {
			return
		}
	}
	t.Fatalf("missing symbol {Name:%q Kind:%q Line:%d EndLine:%d} in %+v", name, kind, line, endLine, symbols)
}

func assertNoSymbolNamed(t *testing.T, symbols []Symbol, name string) {
	t.Helper()
	for _, s := range symbols {
		if s.Name == name {
			t.Fatalf("unexpected symbol %+v", s)
		}
	}
}

func assertCallCount(t *testing.T, calls []CallMetric, name string, count int) {
	t.Helper()
	for _, c := range calls {
		if c.Name == name {
			if c.Count != count {
				t.Fatalf("call %q count = %d, want %d", name, c.Count, count)
			}
			return
		}
	}
	t.Fatalf("missing call %q in %+v", name, calls)
}

func assertCallAbsent(t *testing.T, calls []CallMetric, name string) {
	t.Helper()
	for _, c := range calls {
		if c.Name == name {
			t.Fatalf("unexpected call %+v", c)
		}
	}
}
