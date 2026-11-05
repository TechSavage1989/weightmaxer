// Package version identifies this build. Release builds set both variables
// with -ldflags -X from the VERSION file (see scripts/build.py).
package version

import "strings"

// AppID is the stable identity checked in downloaded update executables.
const AppID = "com.techsavage1989.weightmaxer"

var (
	// Version is the semantic version of this build, for example 1.0.0.
	Version = "0.0.0-dev"
	// Identity is embedded verbatim in the executable so the updater can
	// confirm that a downloaded file is Weightmaxer at the expected version.
	Identity = "WMXID|" + AppID + "|0.0.0-dev|END"
)

// IdentityFor returns the marker a Weightmaxer executable of version v contains.
func IdentityFor(v string) string { return "WMXID|" + AppID + "|" + v + "|END" }

// IsDev reports whether this is a local development build without a release version.
func IsDev() bool { return strings.HasSuffix(Version, "-dev") }
