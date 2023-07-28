// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func DiffValidators(g, h *network.GlobalValues, partitionID string) []*ValidatorUpdate {
	updates := map[[32]byte]*ValidatorUpdate{}
	var order [][32]byte

	put := func(key []byte, power int64) {
		kh := sha256.Sum256(key)
		up, ok := updates[kh]
		if !ok {
			up = new(ValidatorUpdate)
			up.Type = protocol.SignatureTypeED25519
			up.PublicKey = key
			updates[kh] = up
			order = append(order, kh)
		}
		up.Power = power
	}

	// Mark the old keys for deletion
	if g != nil {
		for _, v := range g.Network.Validators {
			if !v.IsActiveOn(partitionID) {
				continue
			}
			put(v.PublicKey, 0)
		}
	}

	// Process the new keys
	for _, v := range h.Network.Validators {
		if !v.IsActiveOn(partitionID) {
			continue
		}
		kh := sha256.Sum256(v.PublicKey)
		if _, ok := updates[kh]; ok {
			// If the key is present in new and old, unmark it
			delete(updates, kh)
		} else {
			// If the key is only present in new, mark it for addition
			put(v.PublicKey, 1)
		}
	}

	final := make([]*ValidatorUpdate, 0, len(updates))
	for _, kh := range order {
		if up, ok := updates[kh]; ok {
			final = append(final, up)
		}
	}

	return final
}
