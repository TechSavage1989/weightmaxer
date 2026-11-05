//go:build !updatetest

package update

func applyTestOverrides(*Config) {}

// TestAutoUpdate is false in release builds.
func TestAutoUpdate() bool { return false }

// TestFailHealth is false in release builds.
func TestFailHealth() bool { return false }
