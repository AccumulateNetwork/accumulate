package accumulate

//go:generate go run ./tools/cmd/goimports

const unknownVersion = "version unknown"

var Version = unknownVersion
var Commit string

func IsVersionKnown() bool {
	return Version != unknownVersion
}
