// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package message enums.yml --registry Type:messageRegistry
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --elide-package-type --package message messages.yml private.yml --reference api:../options.yml,private:../../../../internal/api/private/types.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --elide-package-type --package message --language go-union --out unions_gen.go messages.yml private.yml --reference api:../options.yml,private:../../../../internal/api/private/types.yml

// Type is the type of a [Message].
type Type int

// A Message is a binary message used to transport API v3 RPCs.
type Message interface {
	encoding.UnionValue
	Type() Type
}

// AddressOf returns the message's address if it is an Addressed.
func AddressOf(msg Message) multiaddr.Multiaddr {
	if msg, ok := msg.(*Addressed); ok {
		return msg.Address
	}
	return nil
}

type msgStructPtr[T any] interface {
	*T
	Message
	Equal(*T) bool
}

// RegisterMessageType registers a message type.
func RegisterMessageType[P msgStructPtr[T], T any](name string) error {
	new := func() Message { return P(new(T)) }
	equal := func(a, b Message) bool {
		u, ok1 := a.(P)
		if u == nil {
			return b == nil
		}
		v, ok2 := b.(P)
		if !ok1 || !ok2 {
			return false
		}
		if u == v {
			return true
		}
		return u.Equal((*T)(v))
	}
	return messageRegistry.Register(name, new, equal)
}
