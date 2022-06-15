package core

import (
	"fmt"
)

type ValidatorUpdate int

const ValidatorUpdateAdd = 1
const ValidatorUpdateRemove = 2

func (g *GlobalValues) DiffValidators(h *GlobalValues, subnetID string) (map[[32]byte]ValidatorUpdate, error) {
	updates := map[[32]byte]ValidatorUpdate{}

	// Mark the old keys for deletion
	if g != nil {
		old := g.Network.Subnet(subnetID)
		if old == nil {
			return nil, fmt.Errorf("subnet %s is missing from network definition", subnetID)
		}

		for _, key := range old.ValidatorKeys {
			if len(key) != 32 {
				return nil, fmt.Errorf("invalid ED25519 key: wrong length")
			}

			updates[*(*[32]byte)(key)] = ValidatorUpdateRemove
		}
	}

	// Process the new keys
	new := h.Network.Subnet(subnetID)
	if new == nil {
		return nil, fmt.Errorf("subnet %s is missing from network definition", subnetID)
	}

	for _, key := range new.ValidatorKeys {
		if len(key) != 32 {
			return nil, fmt.Errorf("invalid ED25519 key: wrong length")
		}
		k32 := *(*[32]byte)(key)

		// If the key is present in new and old, unmark it
		delete(updates, k32)

		// If the key is only present in new, mark it for addition
		updates[k32] = 1
	}

	return updates, nil
}
