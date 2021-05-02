package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// Version is this program's version string
var Version string

// UsageVersion introspects the process debug data for Go modules to return a
// version string.
func UsageVersion(includeDeps bool) string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		panic("failed to read BuildInfo because the program was compiled with Go " + runtime.Version())
	}

	if Version == "" {
		// The version wasn't set by ldflags, so fallback to the Go module version.
		// Although, this value is pretty much guaranteed to just be "(devel)".
		Version = bi.Main.Version
	}

	if !includeDeps {
		if Version == "(devel)" {
			return "zed development build (unknown exact version)"
		}
		return "zed " + Version
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", bi.Path, Version)
	for _, dep := range bi.Deps {
		fmt.Fprintf(&b, "\n\t%s %s", dep.Path, dep.Version)
	}
	return b.String()
}
