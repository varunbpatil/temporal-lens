// Package version provides build-time version information.
package version

//nolint:gochecknoglobals // Set via -ldflags at build time.
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)
