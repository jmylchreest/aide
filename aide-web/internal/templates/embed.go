package templates

import "embed"

//go:embed *.html
//go:embed all:partials
var FS embed.FS
