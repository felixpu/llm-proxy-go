package version

import (
	"fmt"
	"runtime"
)

// Build-time variables injected via -ldflags.
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

// Info returns full version string with build metadata.
func Info() string {
	return fmt.Sprintf("llm-proxy %s (commit: %s, built: %s, %s/%s)",
		Version, GitCommit, BuildTime, runtime.GOOS, runtime.GOARCH)
}

// Short returns the version string only.
func Short() string {
	return Version
}
