package frontend

import "embed"

//go:embed all:css all:js all:vendor *.html *.ico *.png *.yaml
var FS embed.FS
