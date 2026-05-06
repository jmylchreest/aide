---
sidebar_position: 4
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
bunx @jmylchreest/aide-plugin install --plugin-path /path/to/aide
```

:::note
The `aide` Go binary is automatically downloaded when the plugin is installed via marketplace or npm. Building from source is only needed for development or customization.
:::
