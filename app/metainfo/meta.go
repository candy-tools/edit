// Package metainfo holds build metadata. The variables are overridden at build
// time with -ldflags -X (see the Makefile / goreleaser); the defaults are what
// a plain `go build` or `go run` produces.
package metainfo

var (
	// Version is the release version, e.g. "v1.2.3".
	Version = "dev"
	// Commit is the short git SHA the binary was built from.
	Commit = "none"
	// BuildTime is the UTC build timestamp.
	BuildTime = "unknown"
)
