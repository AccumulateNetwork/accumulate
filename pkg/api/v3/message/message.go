package message

import (
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

type Message interface {
	encoding.UnionValue
	Type() Type
}

func AddressOf(msg Message) multiaddr.Multiaddr {
	if msg, ok := msg.(*Addressed); ok {
		return msg.Address
	}
	return nil
}
