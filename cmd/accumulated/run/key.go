// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"crypto/ed25519"
	"crypto/rand"

	. "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type PrivateKey interface {
	Type() PrivateKeyType
	CopyAsInterface() any

	get() address.Address
}

func (k *TransientPrivateKey) get() address.Address {
	if k.key != nil {
		return k.key
	}

	Warnf("Generating a new key. This is highly discouraged for permanent infrastructure.")
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	k.key = &address.PrivateKey{
		PublicKey: address.PublicKey{
			Type: protocol.SignatureTypeED25519,
			Key:  pk,
		},
		Key: sk,
	}
	return k.key
}

func (k *PrivateKeySeed) get() address.Address {
	if k.key != nil {
		return k.key
	}

	Warnf("Generating a new key from a seed. This is not at all secure.")
	h := k.Seed.Hash()
	sk := ed25519.NewKeyFromSeed(h[:])
	k.key = &address.PrivateKey{
		PublicKey: address.PublicKey{
			Type: protocol.SignatureTypeED25519,
			Key:  sk[32:],
		},
		Key: sk,
	}
	return k.key
}
