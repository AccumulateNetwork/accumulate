package staking

//go:generate go run ../../../tools/cmd/gen-enum --package staking enums.yml
//go:generate go run ../../../tools/cmd/gen-types --package staking types.yml

// AccountType is the stake type of a staking account.
type AccountType uint64
