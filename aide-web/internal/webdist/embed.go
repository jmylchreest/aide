package webdist

import "embed"

// FS contains the Astro build output.
// Build with: cd aide-web/web && bun install && bun run build (outputs to internal/webdist/build/)
//
//go:embed all:build
var FS embed.FS
