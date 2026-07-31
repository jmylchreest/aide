---
sidebar_position: 5
---

# From Source

## Prerequisites

- Node.js 20+
- Go 1.21+ (for building the aide binary)
- Git

## Build

```bash
git clone https://github.com/jmylchreest/aide && cd aide

# Build the Go binary
cd aide && go build -o ../bin/aide ./cmd/aide && cd ..

# Install JS dependencies and build
npm install && npm run build
```

## Install

### Claude Code

```bash
claude --plugin-dir /path/to/aide
```

### OpenCode

```bash
bunx @jmylchreest/aide-plugin@latest install --plugin-path /path/to/aide
```

:::note
The `aide` Go binary comes with the plugin: npm installs ship it as a per-platform package (`@jmylchreest/aide-binary-<platform>-<arch>`, an optionalDependency pinned to the plugin version), and Claude Code marketplace installs download it on first run. Building from source is only needed for development or customization.
:::
