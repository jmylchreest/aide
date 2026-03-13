---
name: semgrep
description: Run Semgrep security and code quality analysis
triggers:
  - semgrep
  - security scan
  - sast scan
  - vulnerability scan
  - code security
  - security audit
requires_binary:
  - semgrep
---

# Semgrep Security Analysis

Run Semgrep to detect security vulnerabilities and code quality issues in the codebase.

## Workflow

### 1. Run Semgrep scan

```bash
# Auto-detect rules for the project's languages
semgrep scan --config auto --json --quiet 2>/dev/null | head -c 50000
```

If JSON output is too large, use text output:

```bash
semgrep scan --config auto --quiet 2>/dev/null
```

### 2. For specific rule sets

```bash
# Security-focused rules only
semgrep scan --config "p/security-audit" --json --quiet

# OWASP Top 10
semgrep scan --config "p/owasp-top-ten" --json --quiet

# Language-specific
semgrep scan --config "p/golang" --json --quiet
semgrep scan --config "p/python" --json --quiet
semgrep scan --config "p/typescript" --json --quiet
```

### 3. Triage results

For each finding:

1. Read the file and surrounding context
2. Assess whether the finding is a true positive or false positive
3. For true positives, fix the issue following the suggestion in the finding
4. For false positives, consider adding a `# nosemgrep` inline comment with justification

### 4. Scan specific files

```bash
# Scan only changed files
semgrep scan --config auto --json --quiet -- path/to/file.py
```

## Common Issues

- **Too many findings**: Use `--severity ERROR` to focus on critical issues first
- **Slow scan**: Use `--config auto` instead of multiple rule packs to avoid re-scanning
- **Missing rules**: Install additional rules with `semgrep registry`
- **False positives**: Add `# nosemgrep: rule-id` with a comment explaining why
