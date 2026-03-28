package webdist

import "embed"

// FS contains the Astro build output.
// Build with: cd web && npm run build (outputs to ../internal/webdist/build/)
//
//go:embed all:build
var FS embed.FS
