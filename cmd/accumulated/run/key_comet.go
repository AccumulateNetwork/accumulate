// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !dagbft

package run

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// cometKeyJSON represents the JSON structure of CometBFT key files
// (priv_validator_key.json and node_key.json)
type cometKeyJSON struct {
	PrivKey cometKeyData `json:"priv_key"`
}

type cometKeyData struct {
	Type  string `json:"type"`
	Value []byte `json:"value"` // base64-decoded by encoding/json
}

func (k *CometPrivValFile) get(inst *Instance) (address.Address, error) {
	if k.key != nil {
		return k.key, nil
	}

	b, err := os.ReadFile(inst.path(k.Path))
	if err != nil {
		// CometBFT key files are optional for DAG-BFT nodes
		// Generate a transient key instead
		inst.logger.WarnContext(inst.context, "CometBFT validator key not found, generating transient key (OK for DAG-BFT)", "error", err)
		pk, sk, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
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

	var keyFile cometKeyJSON
	err = json.Unmarshal(b, &keyFile)
	if err != nil {
		return nil, err
	}

	return convertCometKeyData(keyFile.PrivKey, &k.key)
}

func (k *CometNodeKeyFile) get(inst *Instance) (address.Address, error) {
	if k.key != nil {
		return k.key, nil
	}

	b, err := os.ReadFile(inst.path(k.Path))
	if err != nil {
		// CometBFT key files are optional for DAG-BFT nodes
		// Generate a transient key instead
		inst.logger.WarnContext(inst.context, "CometBFT node key not found, generating transient key (OK for DAG-BFT)", "error", err)
		pk, sk, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
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

	var keyFile cometKeyJSON
	err = json.Unmarshal(b, &keyFile)
	if err != nil {
		return nil, err
	}

	return convertCometKeyData(keyFile.PrivKey, &k.key)
}

func convertCometKeyData(key cometKeyData, ptr *address.Address) (address.Address, error) {
	switch key.Type {
	case "tendermint/PrivKeyEd25519":
		if len(key.Value) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("invalid ed25519 key size: %d", len(key.Value))
		}
		sk := ed25519.PrivateKey(key.Value)
		*ptr = &address.PrivateKey{
			PublicKey: address.PublicKey{
				Type: protocol.SignatureTypeED25519,
				Key:  sk.Public().(ed25519.PublicKey),
			},
			Key: sk,
		}
	default:
		return nil, fmt.Errorf("comet key type %q not supported", key.Type)
	}
	return *ptr, nil
}
