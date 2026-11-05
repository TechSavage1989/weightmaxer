// Command wmxtool checks release packages with the app's own update code.
//
//	go run ./cmd/wmxtool verify dist/Weightmaxer-win-x64.zip 1.0.0
//	go run ./cmd/wmxtool feedcheck [--expect 1.0.0]
//
// feedcheck runs the production update path against the live GitHub release
// (check, download, SHA-256 verification, archive and executable checks) as if
// it were an older version. It needs network access to GitHub.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TechSavage1989/weightmaxer/internal/update"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "verify":
		if len(os.Args) != 4 {
			usage()
		}
		dir, err := os.MkdirTemp("", "wmx-verify-")
		check(err)
		defer os.RemoveAll(dir)
		exe, err := update.ExtractExe(os.Args[2], os.Args[3], dir)
		check(err)
		fi, _ := os.Stat(exe)
		fmt.Printf("ok: %s contains Weightmaxer %s (%d bytes)\n", filepath.Base(os.Args[2]), os.Args[3], fi.Size())
	case "feedcheck":
		fs := flag.NewFlagSet("feedcheck", flag.ExitOnError)
		expect := fs.String("expect", "", "version the latest release must have")
		_ = fs.Parse(os.Args[2:])
		dir, err := os.MkdirTemp("", "wmx-feed-")
		check(err)
		defer os.RemoveAll(dir)
		c := &update.Client{Config: update.Production("0.0.1"), Current: "0.0.1", CachePath: filepath.Join(dir, "cache.json")}
		r := c.Check(context.Background())
		if r.Kind != update.KindAvailable {
			check(fmt.Errorf("check: %s %s", r.Kind, r.Message))
		}
		if *expect != "" && r.Release.Version != *expect {
			check(fmt.Errorf("latest release is %s, expected %s", r.Release.Version, *expect))
		}
		zip, err := c.Download(context.Background(), r.Release, dir, nil)
		check(err)
		_, err = update.ExtractExe(zip, r.Release.Version, filepath.Join(dir, "staging"))
		check(err)
		// A second check must be answered from the ETag cache.
		r2 := c.Check(context.Background())
		fmt.Printf("ok: latest release %s downloaded and verified; repeat check cached=%v\n", r.Release.Version, r2.NotModified)
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: wmxtool verify <zip> <version> | wmxtool feedcheck [--expect VERSION]")
	os.Exit(2)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
