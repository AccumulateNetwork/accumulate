package snapshot

//go:generate go run ../../../tools/cmd/gen-enum --package snapshot enums.yml
//go:generate go run ../../../tools/cmd/gen-types --package snapshot types.yml

type SectionType uint64

type ChainState = Chain
