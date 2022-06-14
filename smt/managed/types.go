package managed

//go:generate go run ../../tools/cmd/gen-enum --package managed enums.yml
//go:generate go run ../../tools/cmd/gen-types --package managed types.yml
//go:generate go run ../../tools/cmd/gen-record --package managed records.yml

// ChainType is the type of a chain belonging to an account.
type ChainType uint64
