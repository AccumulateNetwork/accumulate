package messaging

import "gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package messaging enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package messaging types.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package messaging --out unions_gen.go --language go-union types.yml

// MessageType is the type of a [Message].
type MessageType int

type Message interface {
	encoding.UnionValue
	Type() MessageType
}
