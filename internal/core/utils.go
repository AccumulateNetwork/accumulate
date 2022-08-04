package core

import (
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type globalValueMemos struct {
	bvns       []string
	partitions []string
}

func (g *GlobalValues) GetPartitions() []string {
	if g.memoize.partitions != nil {
		return g.memoize.partitions
	}

	g.memoize.partitions = make([]string, 0, len(g.Network.Partitions))
	for _, p := range g.Network.Partitions {
		g.memoize.partitions = append(g.memoize.partitions, p.ID)
	}
	return g.memoize.partitions
}

func (g *GlobalValues) GetBvns() []string {
	if g.memoize.bvns != nil {
		return g.memoize.bvns
	}

	g.memoize.bvns = make([]string, 0, len(g.Network.Partitions))
	for _, p := range g.Network.Partitions {
		if strings.EqualFold(p.ID, protocol.Directory) {
			continue
		}
		g.memoize.bvns = append(g.memoize.bvns, p.ID)
	}
	return g.memoize.bvns
}

type ValidatorUpdate int

const ValidatorUpdateAdd = 1
const ValidatorUpdateRemove = 2

func (g *GlobalValues) DiffValidators(h *GlobalValues, partitionID string) (map[[32]byte]ValidatorUpdate, error) {
	updates := map[[32]byte]ValidatorUpdate{}

	// Mark the old keys for deletion
	if g != nil {
		old := g.Network.Partition(partitionID)
		if old == nil {
			return nil, fmt.Errorf("partition %s is missing from network definition", partitionID)
		}

		for _, v := range old.Validators {
			if !v.Active {
				continue
			}
			if len(v.PublicKey) != 32 {
				return nil, fmt.Errorf("invalid ED25519 key: wrong length")
			}

			updates[*(*[32]byte)(v.PublicKey)] = ValidatorUpdateRemove
		}
	}

	// Process the new keys
	new := h.Network.Partition(partitionID)
	if new == nil {
		return nil, fmt.Errorf("partition %s is missing from network definition", partitionID)
	}

	for _, v := range new.Validators {
		if !v.Active {
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

func (g *GlobalValues) DiffAddressBook(h *GlobalValues) map[[32]byte]*protocol.InternetAddress {
	keys := map[[32]byte]struct{}{}
	old := map[[32]byte]*protocol.InternetAddress{}
	if g != nil {
		for _, entry := range g.AddressBook.Entries {
			keys[entry.PublicKeyHash] = struct{}{}
			old[entry.PublicKeyHash] = entry.Address
		}
	}

	new := map[[32]byte]*protocol.InternetAddress{}
	for _, entry := range h.AddressBook.Entries {
		keys[entry.PublicKeyHash] = struct{}{}
		new[entry.PublicKeyHash] = entry.Address
	}

	diff := map[[32]byte]*protocol.InternetAddress{}
	for key := range keys {
		old, new := old[key], new[key]
		switch {
		case old == new:
			continue // No change
		case old == nil:
			diff[key] = new // Add new address
		default:
			diff[key] = nil // Remove old address
		}
	}
	return diff
}
