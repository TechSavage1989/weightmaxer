package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Semver is a strict MAJOR.MINOR.PATCH release version.
type Semver [3]int

// Parse accepts "1.2.3" or "v1.2.3". Pre-release and build suffixes are rejected
// because Weightmaxer only updates between stable releases.
func Parse(s string) (Semver, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	parts := strings.Split(s, ".")
	var v Semver
	if len(parts) != 3 {
		return v, fmt.Errorf("version %q is not MAJOR.MINOR.PATCH", s)
	}
	for i, p := range parts {
		if p == "" || len(p) > 6 || (len(p) > 1 && p[0] == '0') {
			return v, fmt.Errorf("version %q has an invalid number", s)
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, fmt.Errorf("version %q has an invalid number", s)
		}
		v[i] = n
	}
	return v, nil
}

// Compare returns -1, 0 or 1.
func (a Semver) Compare(b Semver) int {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func (a Semver) String() string { return fmt.Sprintf("%d.%d.%d", a[0], a[1], a[2]) }
