// Package version exposes build-time version metadata for the goruby
// binaries. Values default to "dev"; `make generate-version` (or any
// `go generate ./internal/version/...`) writes a sibling version.go
// whose init() overwrites them with VERSION + git state.
package version

var (
	Version   = "dev"
	CommitSHA = ""
	BuildDate = ""
	BuildInfo = ""
)

//go:generate go run github.com/lczyk/version/go/cmd/generate-version -out version.go -pkg version -init
