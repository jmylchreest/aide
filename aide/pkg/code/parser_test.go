package code

import (
	"os"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/grammar"
)

// newTestParser creates a Parser with a real CompositeLoader (builtins only, no auto-download).
func newTestParser() *Parser {
	loader := grammar.NewCompositeLoader(grammar.WithAutoDownload(false))
	return NewParser(loader)
}

// ---------------------------------------------------------------------------
// DetectLanguage — extension, filename, shebang
// ---------------------------------------------------------------------------

func TestDetectLanguageByExtension(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"app.ts", "typescript"},
		{"app.tsx", "typescript"},
		{"index.js", "javascript"},
		{"index.jsx", "javascript"},
		{"lib.py", "python"},
		{"main.rs", "rust"},
		{"App.java", "java"},
		{"main.c", "c"},
		{"main.h", "c"},
		{"main.cpp", "cpp"},
		{"main.cc", "cpp"},
		{"main.hpp", "cpp"},
		{"lib.cs", "csharp"},
		{"app.rb", "ruby"},
		{"index.php", "php"},
		{"main.swift", "swift"},
		{"main.kt", "kotlin"},
		{"main.scala", "scala"},
		{"app.ex", "elixir"},
		{"app.exs", "elixir"},
		{"main.lua", "lua"},
		{"script.sh", "bash"},
		{"script.bash", "bash"},
		{"query.sql", "sql"},
		{"index.html", "html"},
		{"style.css", "css"},
		{"config.yaml", "yaml"},
		{"config.yml", "yaml"},
		{"config.toml", "toml"},
		{"data.json", "json"},
		{"main.hcl", "hcl"},
		{"main.tf", "hcl"},
		{"schema.proto", "protobuf"},
		{"lib.ml", "ocaml"},
		{"Main.elm", "elm"},
		{"build.groovy", "groovy"},
		{"build.gradle", "groovy"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := DetectLanguage(tt.path, nil)
			if got != tt.want {
				t.Errorf("DetectLanguage(%q) = %q; want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestDetectLanguageByFilename(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"Jenkinsfile", "groovy"},
		{"Vagrantfile", "ruby"},
		{"Rakefile", "ruby"},
		{"Gemfile", "ruby"},
		{"BUILD", "python"},
		{"BUILD.bazel", "python"},
		{"WORKSPACE", "python"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := DetectLanguage(tt.path, nil)
			if got != tt.want {
				t.Errorf("DetectLanguage(%q) = %q; want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestDetectLanguageByShebang(t *testing.T) {
	tests := []struct {
		name    string
		shebang string
		want    string
	}{
		{"python3", "#!/usr/bin/env python3\nimport sys\n", "python"},
		{"python", "#!/usr/bin/python\nimport sys\n", "python"},
		{"bash", "#!/bin/bash\necho hi\n", "bash"},
		{"sh", "#!/bin/sh\necho hi\n", "bash"},
		{"node", "#!/usr/bin/env node\nconsole.log('hi');\n", "javascript"},
		{"ruby", "#!/usr/bin/env ruby\nputs 'hi'\n", "ruby"},
		{"lua", "#!/usr/bin/env lua\nprint('hi')\n", "lua"},
		{"php", "#!/usr/bin/env php\n<?php\n", "php"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a file with no extension so extension detection doesn't match.
			got := DetectLanguage("script", []byte(tt.shebang))
			if got != tt.want {
				t.Errorf("DetectLanguage with shebang %q = %q; want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestDetectLanguageUnknown(t *testing.T) {
	got := DetectLanguage("unknown.xyz", nil)
	if got != "" {
		t.Errorf("DetectLanguage(unknown) = %q; want empty", got)
	}
}

func TestDetectLanguageShebangPriority(t *testing.T) {
	// Extension should take priority over shebang.
	got := DetectLanguage("script.py", []byte("#!/usr/bin/env ruby\nputs 'hi'\n"))
	if got != "python" {
		t.Errorf("extension should take priority: got %q; want %q", got, "python")
	}
}

// ---------------------------------------------------------------------------
// ParseContent — for each of the 9 core compiled-in grammars
// ---------------------------------------------------------------------------

func TestParseContentGo(t *testing.T) {
	p := newTestParser()
	content := []byte(`package main

func main() {
	fmt.Println("hello")
}

type Server struct {
	addr string
}

func (s *Server) Start() error {
	return nil
}

type Handler interface {
	ServeHTTP()
}
`)

	symbols, err := p.ParseContent(content, "go", "main.go")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if len(symbols) == 0 {
		t.Fatal("expected symbols, got none")
	}

	names := symbolNames(symbols)
	assertContains(t, names, "main", "function main")
	assertContains(t, names, "Server", "struct Server")
	assertContains(t, names, "Start", "method Start")
	assertContains(t, names, "Handler", "interface Handler")
}

func TestParseContentTypeScript(t *testing.T) {
	p := newTestParser()
	content := []byte(`
function greet(name: string): string {
	return "Hello, " + name;
}

class UserService {
	getUser(id: string): User {
		return { id };
	}
}

interface User {
	id: string;
}

type UserID = string;

enum Role {
	Admin,
	User,
}
`)

	symbols, err := p.ParseContent(content, "typescript", "app.ts")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if len(symbols) == 0 {
		t.Fatal("expected symbols, got none")
	}

	names := symbolNames(symbols)
	assertContains(t, names, "greet", "function greet")
	assertContains(t, names, "UserService", "class UserService")
	assertContains(t, names, "getUser", "method getUser")
	assertContains(t, names, "User", "interface User")
	assertContains(t, names, "UserID", "type UserID")
	assertContains(t, names, "Role", "enum Role")
}

func TestParseContentJavaScript(t *testing.T) {
	p := newTestParser()
	content := []byte(`
function add(a, b) {
	return a + b;
}

class Calculator {
	multiply(a, b) {
		return a * b;
	}
}
`)

	symbols, err := p.ParseContent(content, "javascript", "calc.js")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if len(symbols) == 0 {
		t.Fatal("expected symbols, got none")
	}

	names := symbolNames(symbols)
	assertContains(t, names, "add", "function add")
	assertContains(t, names, "Calculator", "class Calculator")
	assertContains(t, names, "multiply", "method multiply")
}

func TestParseContentPython(t *testing.T) {
	p := newTestParser()
	content := []byte(`
def greet(name):
    return f"Hello, {name}"

class UserService:
    def get_user(self, user_id):
        return {"id": user_id}
`)

	symbols, err := p.ParseContent(content, "python", "app.py")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if len(symbols) == 0 {
		t.Fatal("expected symbols, got none")
	}

	names := symbolNames(symbols)
	assertContains(t, names, "greet", "function greet")
	assertContains(t, names, "UserService", "class UserService")
	assertContains(t, names, "get_user", "method get_user")
}

func TestParseContentRust(t *testing.T) {
	p := newTestParser()
	content := []byte(`
fn main() {
    println!("hello");
}

struct Server {
    addr: String,
}

impl Server {
    fn start(&self) -> Result<(), Box<dyn std::error::Error>> {
        Ok(())
    }
}

trait Handler {
    fn handle(&self);
}

enum Status {
    Active,
    Inactive,
}
`)

	symbols, err := p.ParseContent(content, "rust", "main.rs")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if len(symbols) == 0 {
		t.Fatal("expected symbols, got none")
	}

	names := symbolNames(symbols)
	assertContains(t, names, "main", "function main")
	assertContains(t, names, "Server", "struct Server (class)")
	assertContains(t, names, "Handler", "trait Handler (interface)")
	assertContains(t, names, "Status", "enum Status (class)")
}

func TestParseContentJava(t *testing.T) {
	p := newTestParser()
	content := []byte(`
public class UserService {
    public User getUser(String id) {
        return new User(id);
    }

    public UserService() {
        // constructor
    }
}

public interface Repository {
    User findById(String id);
}

public enum Role {
    ADMIN, USER
}
`)

	symbols, err := p.ParseContent(content, "java", "UserService.java")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if len(symbols) == 0 {
		t.Fatal("expected symbols, got none")
	}

	names := symbolNames(symbols)
	assertContains(t, names, "UserService", "class UserService")
	assertContains(t, names, "getUser", "method getUser")
	assertContains(t, names, "Repository", "interface Repository")
	assertContains(t, names, "Role", "enum Role")
}

func TestParseContentC(t *testing.T) {
	p := newTestParser()
	content := []byte(`
#include <stdio.h>

struct Point {
    int x;
    int y;
};

typedef int ErrorCode;

int add(int a, int b) {
    return a + b;
}

void greet(const char* name) {
    printf("Hello, %s\n", name);
}
`)

	symbols, err := p.ParseContent(content, "c", "main.c")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if len(symbols) == 0 {
		t.Fatal("expected symbols, got none")
	}

	names := symbolNames(symbols)
	assertContains(t, names, "Point", "struct Point")
	assertContains(t, names, "add", "function add")
	assertContains(t, names, "greet", "function greet")
	assertContains(t, names, "ErrorCode", "typedef ErrorCode")
}

func TestParseContentCPP(t *testing.T) {
	p := newTestParser()
	content := []byte(`
#include <string>

class Server {
public:
    void start();
};

void Server::start() {
    // implementation
}

struct Config {
    std::string host;
    int port;
};

enum Color {
    Red,
    Green,
    Blue
};

void freeFunction() {
    // standalone function
}
`)

	symbols, err := p.ParseContent(content, "cpp", "server.cpp")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if len(symbols) == 0 {
		t.Fatal("expected symbols, got none")
	}

	names := symbolNames(symbols)
	assertContains(t, names, "Server", "class Server")
	assertContains(t, names, "Config", "struct Config")
	assertContains(t, names, "Color", "enum Color")
	assertContains(t, names, "freeFunction", "function freeFunction")
}

func TestParseContentZig(t *testing.T) {
	p := newTestParser()
	// Zig doesn't have a TagQuery entry, so ParseContent should return nil, nil.
	content := []byte(`
const std = @import("std");

pub fn main() !void {
    const stdout = std.io.getStdOut().writer();
    try stdout.print("Hello, {s}!\n", .{"world"});
}
`)

	symbols, err := p.ParseContent(content, "zig", "main.zig")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	// Zig has no tag query defined and no legacy extractor, so symbols should be nil.
	// This is the expected behaviour — we still want the grammar to load without error.
	_ = symbols
}

// ---------------------------------------------------------------------------
// ParseContent — dynamic grammars (bash, lua)
// These require dynamic grammar libraries to be installed.
// Tests are skipped in CI (auto-download disabled, no grammars present).
// ---------------------------------------------------------------------------

func TestParseContentBash(t *testing.T) {
	p := newTestParser()
	content := []byte(`#!/bin/bash

function greet() {
    local name=$1
    echo "Hello, $name!"
}

function farewell() {
    local name=$1
    echo "Goodbye, $name!"
}

function main() {
    greet "world"
    farewell "world"
}
`)

	symbols, err := p.ParseContent(content, "bash", "script.sh")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if symbols == nil {
		t.Skip("bash grammar not available (dynamic grammar not installed)")
	}

	names := symbolNames(symbols)
	assertContains(t, names, "greet", "function greet")
	assertContains(t, names, "farewell", "function farewell")
	assertContains(t, names, "main", "function main")
}

func TestParseContentLua(t *testing.T) {
	p := newTestParser()
	content := []byte(`
local function fibonacci(n)
    if n <= 1 then
        return n
    end
    return fibonacci(n - 1) + fibonacci(n - 2)
end

local function factorial(n)
    if n == 0 then
        return 1
    end
    return n * factorial(n - 1)
end

local M = {}

function M.greet(name)
    print("Hello, " .. name)
end
`)

	symbols, err := p.ParseContent(content, "lua", "lib.lua")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if symbols == nil {
		t.Skip("lua grammar not available (dynamic grammar not installed)")
	}

	names := symbolNames(symbols)
	assertContains(t, names, "fibonacci", "function fibonacci")
	assertContains(t, names, "factorial", "function factorial")
	assertContains(t, names, "greet", "method M.greet")
}

// ---------------------------------------------------------------------------
// ParseContent — edge cases
// ---------------------------------------------------------------------------

func TestParseContentEmptyFile(t *testing.T) {
	p := newTestParser()

	symbols, err := p.ParseContent([]byte(""), "go", "empty.go")
	if err != nil {
		t.Fatalf("ParseContent empty: %v", err)
	}
	if len(symbols) != 0 {
		t.Errorf("expected 0 symbols for empty file, got %d", len(symbols))
	}
}

func TestParseContentUnsupportedLanguage(t *testing.T) {
	p := newTestParser()

	symbols, err := p.ParseContent([]byte("content"), "nonexistent", "file.xyz")
	if err != nil {
		t.Fatalf("ParseContent unsupported: %v", err)
	}
	if symbols != nil {
		t.Errorf("expected nil for unsupported language, got %v", symbols)
	}
}

func TestParseContentSymbolFields(t *testing.T) {
	p := newTestParser()
	content := []byte(`package main

func Add(a, b int) int {
	return a + b
}
`)

	symbols, err := p.ParseContent(content, "go", "math.go")
	if err != nil {
		t.Fatal(err)
	}

	var addSym *Symbol
	for _, s := range symbols {
		if s.Name == "Add" {
			addSym = s
			break
		}
	}

	if addSym == nil {
		t.Fatal("symbol 'Add' not found")
	}

	if addSym.Kind != KindFunction {
		t.Errorf("Kind = %q; want %q", addSym.Kind, KindFunction)
	}
	if addSym.Language != "go" {
		t.Errorf("Language = %q; want %q", addSym.Language, "go")
	}
	if addSym.FilePath != "math.go" {
		t.Errorf("FilePath = %q; want %q", addSym.FilePath, "math.go")
	}
	if addSym.StartLine <= 0 {
		t.Errorf("StartLine = %d; should be positive", addSym.StartLine)
	}
	if addSym.EndLine < addSym.StartLine {
		t.Errorf("EndLine (%d) < StartLine (%d)", addSym.EndLine, addSym.StartLine)
	}
	if addSym.ID == "" {
		t.Error("ID should be set (ULID)")
	}
}

// ---------------------------------------------------------------------------
// ParseFile (integration — writes temp file)
// ---------------------------------------------------------------------------

func TestParseFile(t *testing.T) {
	p := newTestParser()
	content := `package main

func Hello() string {
	return "hello"
}
`
	path := t.TempDir() + "/hello.go"
	if err := writeFile(path, content); err != nil {
		t.Fatal(err)
	}

	symbols, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(symbols) == 0 {
		t.Fatal("expected symbols from ParseFile")
	}

	names := symbolNames(symbols)
	assertContains(t, names, "Hello", "function Hello")
}

func TestParseFileUnknownExtension(t *testing.T) {
	p := newTestParser()
	path := t.TempDir() + "/data.xyz"
	if err := writeFile(path, "random content"); err != nil {
		t.Fatal(err)
	}

	symbols, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile unknown ext: %v", err)
	}
	if symbols != nil {
		t.Errorf("expected nil for unknown extension, got %v", symbols)
	}
}

func TestParseFileNotExist(t *testing.T) {
	p := newTestParser()
	_, err := p.ParseFile("/nonexistent/path/file.go")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

// ---------------------------------------------------------------------------
// TestPackRegistryQueriesCompile verifies that tag/ref queries from the
// PackRegistry compile against the corresponding grammar for all builtin
// languages.
func TestPackRegistryQueriesCompile(t *testing.T) {
	p := newTestParser()
	reg := grammar.DefaultPackRegistry()

	for _, name := range reg.All() {
		pack := reg.Get(name)
		if pack.Queries.Tags != "" {
			t.Run(name+"/tags", func(t *testing.T) {
				tsLang := p.getLanguage(name)
				if tsLang == nil {
					t.Skipf("grammar %q not available (dynamic grammar)", name)
					return
				}
				q := p.getTagQuery(name)
				if q == nil {
					t.Errorf("pack tag query for %q failed to compile", name)
				}
			})
		}
		if pack.Queries.Refs != "" {
			t.Run(name+"/refs", func(t *testing.T) {
				tsLang := p.getLanguage(name)
				if tsLang == nil {
					t.Skipf("grammar %q not available (dynamic grammar)", name)
					return
				}
				q := p.getRefQuery(name)
				if q == nil {
					t.Errorf("pack ref query for %q failed to compile", name)
				}
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func symbolNames(symbols []*Symbol) map[string]bool {
	m := make(map[string]bool, len(symbols))
	for _, s := range symbols {
		m[s.Name] = true
	}
	return m
}

func assertContains(t *testing.T, names map[string]bool, name, desc string) {
	t.Helper()
	if !names[name] {
		t.Errorf("expected symbol %q (%s) not found in: %v", name, desc, mapKeys(names))
	}
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
