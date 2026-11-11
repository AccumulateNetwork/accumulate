// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/cometbft/cometbft/crypto"
	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	tmp2p "github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (k *RawPrivateKey) get(inst *Instance) (address.Address, error) {
	// For a raw key, k.Address is the private key itself. Never log it (#4494).
	addr, err := address.Parse(k.Address)
	if err != nil {
		inst.logger.Error("Failed to parse RawPrivateKey", "error", err)
		return nil, err
	}
	inst.logger.Info("RawPrivateKey loaded", publicKeyLogAttrs(addr)...)
	return addr, nil
}

// publicKeyLogAttrs returns log attributes that identify a key without
// revealing it: its type and public key hash. It never includes the
// private key or the address string (which, for a private key, encodes the
// private key).
func publicKeyLogAttrs(addr address.Address) []any {
	attrs := []any{"key_type", addr.GetType().String()}
	if hash, ok := addr.GetPublicKeyHash(); ok {
		attrs = append(attrs, "public_key_hash", hex.EncodeToString(hash))
	}
	return attrs
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

func (k *CometPrivValFile) get(inst *Instance) (address.Address, error) {
	if k.key != nil {
		return k.key, nil
	}

	b, err := os.ReadFile(inst.path(k.Path))
	if err != nil {
		return nil, err
	}

	var pvKey privval.FilePVKey
	err = cmtjson.Unmarshal(b, &pvKey)
	if err != nil {
		return nil, err
	}

	return convertCometKey(pvKey.PrivKey, &k.key)
}

func (k *CometNodeKeyFile) get(inst *Instance) (address.Address, error) {
	if k.key != nil {
		return k.key, nil
	}

	nk, err := tmp2p.LoadNodeKey(inst.path(k.Path))
	if err != nil {
		return nil, err
	}
	return convertCometKey(nk.PrivKey, &k.key)
}

func convertCometKey(key crypto.PrivKey, ptr *address.Address) (address.Address, error) {
	switch sk := key.(type) {
	case tmed25519.PrivKey:
		*ptr = &address.PrivateKey{
			PublicKey: address.PublicKey{
				Type: protocol.SignatureTypeED25519,
				Key:  sk[32:],
			},
			Key: sk,
		}
	default:
		return nil, fmt.Errorf("comet key type %v not supported", key.Type())
	}
	return *ptr, nil
}
