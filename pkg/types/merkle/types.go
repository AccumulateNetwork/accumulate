package merkle

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package merkle types.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package merkle enums.yml

// ChainType is the type of a chain belonging to an account.
type ChainType uint64
