//go:build updatetest

package paths

import "os"

// Update test builds keep every file inside WEIGHTMAXER_TEST_HOME.
func testHome() string { return os.Getenv("WEIGHTMAXER_TEST_HOME") }
