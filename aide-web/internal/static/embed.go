package static

import "embed"

//go:embed css/*.css js/*.js
var FS embed.FS
