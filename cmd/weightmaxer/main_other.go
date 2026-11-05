//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "Weightmaxer is a Windows app. Build it with scripts/build.py.")
	os.Exit(1)
}
