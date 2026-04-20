package tmpl

import "embed"

//go:embed *.html partials/*.html
var Files embed.FS
