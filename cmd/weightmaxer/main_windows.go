//go:build windows

// Command weightmaxer is the Weightmaxer desktop app for Windows.
package main

import (
	"os"
	"strings"

	"github.com/TechSavage1989/weightmaxer/internal/host"
	"github.com/TechSavage1989/weightmaxer/internal/update"
)

func main() {
	var opts host.Options
	for _, a := range os.Args[1:] {
		switch {
		case strings.HasPrefix(a, "--apply-update="):
			// Started from the staging folder by the old version: install and exit.
			os.Exit(update.RunHelper(strings.TrimPrefix(a, "--apply-update=")))
		case strings.HasPrefix(a, "--post-update="):
			opts.PostUpdate = strings.TrimPrefix(a, "--post-update=")
		case strings.HasPrefix(a, "--update-failed="):
			opts.UpdateFailed = strings.TrimPrefix(a, "--update-failed=")
		case strings.HasPrefix(a, "--self-test="):
			opts.SelfTest = strings.TrimPrefix(a, "--self-test=")
		}
	}
	os.Exit(host.Run(opts))
}
