// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (k *RawPrivateKey) get(inst *Instance) (address.Address, error) {
	return address.Parse(k.Address)
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

// cometKeyFile represents the JSON structure of a CometBFT key file.
// This allows parsing CometBFT key files without importing the CometBFT package.
type cometKeyFile struct {
	PrivKey cometPrivKey `json:"priv_key"`
}

type cometPrivKey struct {
	Type  string `json:"type"`
	Value string `json:"value"` // base64-encoded key
}

func (k *CometPrivValFile) get(inst *Instance) (address.Address, error) {
	if k.key != nil {
		return k.key, nil
	}

	b, err := os.ReadFile(inst.path(k.Path))
	if err != nil {
		return nil, err
	}

	var pvKey cometKeyFile
	err = json.Unmarshal(b, &pvKey)
	if err != nil {
		return nil, fmt.Errorf("parse priv_validator_key.json: %w", err)
	}

	return parseCometKey(pvKey.PrivKey, &k.key)
}

// cometNodeKeyFile represents the JSON structure of a CometBFT node key file.
type cometNodeKeyFile struct {
	PrivKey cometPrivKey `json:"priv_key"`
}

func (k *CometNodeKeyFile) get(inst *Instance) (address.Address, error) {
	if k.key != nil {
		return k.key, nil
	}

	b, err := os.ReadFile(inst.path(k.Path))
	if err != nil {
		return nil, err
	}

	var nk cometNodeKeyFile
	err = json.Unmarshal(b, &nk)
	if err != nil {
		return nil, fmt.Errorf("parse node_key.json: %w", err)
	}

	return parseCometKey(nk.PrivKey, &k.key)
}

func parseCometKey(key cometPrivKey, ptr *address.Address) (address.Address, error) {
	// CometBFT uses "tendermint/PrivKeyEd25519" as the key type
	if key.Type != "tendermint/PrivKeyEd25519" {
		return nil, fmt.Errorf("unsupported key type: %s (expected tendermint/PrivKeyEd25519)", key.Type)
	}

	// Decode the base64-encoded key
	sk, err := base64.StdEncoding.DecodeString(key.Value)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}

	if len(sk) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid key size: %d (expected %d)", len(sk), ed25519.PrivateKeySize)
	}

	*ptr = &address.PrivateKey{
		PublicKey: address.PublicKey{
			Type: protocol.SignatureTypeED25519,
			Key:  sk[32:], // Public key is the last 32 bytes
		},
		Key: sk,
	}
	return *ptr, nil
}
