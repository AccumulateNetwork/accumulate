// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"crypto/ed25519"
	"crypto/rand"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (k *RawPrivateKey) get(inst *Instance) (address.Address, error) {
	inst.logger.Info("Loading RawPrivateKey", "address", k.Address)
	addr, err := address.Parse(k.Address)
	if err != nil {
		inst.logger.Error("Failed to parse RawPrivateKey address", "address", k.Address, "error", err)
		return nil, err
	}
	inst.logger.Info("RawPrivateKey loaded successfully", "address", k.Address)
	return addr, nil
}

func (k *TransientPrivateKey) get(inst *Instance) (address.Address, error) {
	if k.key != nil {
		return k.key, nil
	}

	inst.logger.WarnContext(inst.context, "Generating a new key. This is highly discouraged for permanent infrastructure.")
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
	return k.key, nil
}

func (k *PrivateKeySeed) get(inst *Instance) (address.Address, error) {
	if k.key != nil {
		return k.key, nil
	}

	inst.logger.WarnContext(inst.context, "Generating a new key from a seed. This is not at all secure.")
	h := k.Seed.Hash()
	sk := ed25519.NewKeyFromSeed(h[:])
	k.key = &address.PrivateKey{
		PublicKey: address.PublicKey{
			Type: protocol.SignatureTypeED25519,
			Key:  sk[32:],
		},
		Key: sk,
	}
	return k.key, nil
}
