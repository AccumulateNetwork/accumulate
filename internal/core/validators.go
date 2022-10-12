// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package core

import (
	"fmt"
)

type ValidatorUpdate int

const ValidatorUpdateAdd = 1
const ValidatorUpdateRemove = 2

func (g *GlobalValues) DiffValidators(h *GlobalValues, partitionID string) (map[[32]byte]ValidatorUpdate, error) {
	updates := map[[32]byte]ValidatorUpdate{}

	// Mark the old keys for deletion
	if g != nil {
		for _, v := range g.Network.Validators {
			if !v.IsActiveOn(partitionID) {
				continue
			}

			if len(v.PublicKey) != 32 {
				return nil, fmt.Errorf("invalid ED25519 key: wrong length")
			}

			updates[*(*[32]byte)(v.PublicKey)] = ValidatorUpdateRemove
		}
	}

	// Process the new keys
	for _, v := range h.Network.Validators {
		if !v.IsActiveOn(partitionID) {
			continue
		}
		if len(v.PublicKey) != 32 {
			return nil, fmt.Errorf("invalid ED25519 key: wrong length")
		}
		k32 := *(*[32]byte)(v.PublicKey)

		if _, ok := updates[k32]; ok {
			// If the key is present in new and old, unmark it
			delete(updates, k32)
		} else {
			// If the key is only present in new, mark it for addition
			updates[k32] = 1
		}
	}

	return updates, nil
}
