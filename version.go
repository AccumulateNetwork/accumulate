package accumulate

//-go:generate go run ./internal/cmd/yaml2json -input=openrpc.yml -output=openrpc.json
//go:generate go run ./internal/cmd/goimports

const unknownVersion = "version unknown"

var Version = unknownVersion
var Commit string

func IsVersionKnown() bool {
	return Version != unknownVersion
}
