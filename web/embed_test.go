package web

import (
	"strings"
	"testing"
)

func TestPageInlinesAssets(t *testing.T) {
	p := Page()
	if strings.Contains(p, `href="app.css"`) || strings.Contains(p, `src="app.js"`) {
		t.Fatal("page still references external files")
	}
	if !strings.Contains(p, "--accent:") || !strings.Contains(p, "wmxReady") {
		t.Fatal("inlined CSS or JS missing")
	}
	if strings.Contains(p, "https://") && strings.Contains(p, "<script src=") {
		t.Fatal("page loads remote scripts")
	}
	if len(p) > 1<<20 {
		t.Fatal("page exceeds WebView2's in-memory size limit margin")
	}
}
