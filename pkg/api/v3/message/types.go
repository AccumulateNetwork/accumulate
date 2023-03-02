// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

//go:generate go run github.com/vektra/mockery/v2
//go:generate go run github.com/rinchsan/gosimports/cmd/gosimports -w mocks
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package message enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --elide-package-type --package message messages.yml private.yml --reference ../options.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --elide-package-type --package message --language go-union --out unions_gen.go messages.yml private.yml --reference ../options.yml

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

// Shims for code gen
type (
	ConsensusStatusOptions = api.ConsensusStatusOptions
	NetworkStatusOptions   = api.NetworkStatusOptions
	MetricsOptions         = api.MetricsOptions
	SubscribeOptions       = api.SubscribeOptions
	SubmitOptions          = api.SubmitOptions
	ValidateOptions        = api.ValidateOptions
	FaucetOptions          = api.FaucetOptions
)
