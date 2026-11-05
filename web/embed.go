// Package web holds the user interface shown in the WebView2 window.
package web

import (
	_ "embed"
	"strings"
)

var (
	//go:embed index.html
	indexHTML string
	//go:embed app.css
	appCSS string
	//go:embed app.js
	appJS string
)

// Page returns index.html with the stylesheet and script inlined, so the
// window loads it from memory without a local web server or network access.
func Page() string {
	p := strings.Replace(indexHTML, `<link rel="stylesheet" href="app.css">`, "<style>\n"+appCSS+"\n</style>", 1)
	return strings.Replace(p, `<script src="app.js"></script>`, "<script>\n"+appJS+"\n</script>", 1)
}
