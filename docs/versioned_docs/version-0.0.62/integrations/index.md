# Companion Tools

aide provides code intelligence via tree-sitter-based analysis: code indexing, static analysis (complexity, coupling, secrets, clones, security patterns), and codebase survey. These work without external dependencies.

For deeper analysis, the following tools complement aide well when added to your assistant's toolchain.

## Language Servers (LSP)

Your AI assistant likely supports LSP integration (Claude Code, OpenCode, Cursor all do). LSPs provide semantic understanding that tree-sitter cannot: cross-package go-to-definition, type inference, compiler diagnostics, and precise reference resolution.

aide's code index and LSP serve different purposes and coexist without conflict:

- **aide's code index** is pre-computed and searchable. Survey, findings, and call graphs consume it automatically.
- **LSP** is on-demand. The assistant invokes it when it needs semantic precision.

No configuration needed — they work together naturally.

## Web Search (Exa, Kagi)

MCP servers for web search give your assistant access to documentation, API references, and error resolution. This is especially useful for framework-specific questions that go beyond what code analysis can answer.

- [Exa MCP Server](https://github.com/exa-labs/exa-mcp-server) — Code-focused search
- [Kagi MCP Server](https://github.com/nicholasgriffintn/kagi-mcp-server) — General and code search

## Semgrep

[Semgrep](https://semgrep.dev) provides deep static analysis with taint tracking and cross-file dataflow analysis — capabilities beyond aide's pattern-based security analyzer.

aide includes a built-in `semgrep` skill that activates automatically when `semgrep` is installed on your PATH. The skill guides the assistant through running Semgrep, triaging results, and fixing issues.

Install Semgrep:

```bash
# Python
pip install semgrep

# macOS
brew install semgrep
```

The skill activates when you mention "semgrep", "security scan", "sast scan", or "vulnerability scan" in your prompt.

aide's built-in security analyzer catches common patterns (SQL injection, XSS, command injection, etc.) via language pack rules. Semgrep goes deeper with interprocedural analysis, custom rules, and a broader rule ecosystem. Use both for defense in depth.

## Other Linters

Language-specific linters complement aide's cross-language analysis:

| Language      | Tool            | What it adds                       |
| ------------- | --------------- | ---------------------------------- |
| Go            | `golangci-lint` | 100+ Go-specific linters           |
| Python        | `ruff`          | Fast Python linting and formatting |
| TypeScript/JS | `eslint`        | Configurable JS/TS rules           |
| Rust          | `clippy`        | Rust-specific idiom enforcement    |

These can be exposed to your assistant via shell commands or dedicated MCP servers.
