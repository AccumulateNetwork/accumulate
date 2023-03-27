// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package network

import (
	"fmt"
	"math"
	"strings"
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

type globalValueMemos struct {
	threshold map[string]uint64
	active    map[string]int
}

func (g *GlobalValues) memoizeValidators() {
	if g.memoize.active != nil {
		return
	}

	active := make(map[string]int, len(g.Network.Partitions))
	for _, v := range g.Network.Validators {
		for _, p := range v.Partitions {
			if p.Active {
				active[strings.ToLower(p.ID)]++
			}
		}
	}

	threshold := make(map[string]uint64, len(g.Network.Partitions))
	for partition, active := range active {
		threshold[partition] = g.Globals.ValidatorAcceptThreshold.Threshold(active)
	}

	g.memoize.active = active
	g.memoize.threshold = threshold
}

func (g *GlobalValues) ValidatorThreshold(partition string) uint64 {
	g.memoizeValidators()
	v, ok := g.memoize.threshold[strings.ToLower(partition)]
	if !ok {
		return math.MaxUint64
	}
	return v
}
