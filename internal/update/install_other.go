//go:build !windows

package update

import (
	"errors"
	"time"
)

var errWindowsOnly = errors.New("installing updates is only supported on Windows")

func Start(Plan, string) error            { return errWindowsOnly }
func RunHelper(string) int                { return 2 }
func WaitForExit(int, time.Duration) bool { return true }
