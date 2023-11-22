// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum  --package run enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package run config.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package run --language go-union --out unions_gen.go config.yml

type (
	ConsensusAppType int
	ServiceType      int
	StorageType      int
	PrivateKeyType   int
)

type Service interface {
	ioc.Factory

	Type() ServiceType
	CopyAsInterface() any

	start(inst *Instance) error
}

func setDefault[V any](ptr *V, def V) {
	var z V
	if any(*ptr) == any(z) {
		*ptr = def
	}
}

func setDefault2[V any](ptr **V, def V) {
	if *ptr == nil {
		*ptr = &def
	}
}

func mustParseMulti(s string) multiaddr.Multiaddr {
	addr, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		panic(err)
	}
	return addr
}
