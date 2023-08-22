// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package network

import (
	"math"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type globalValueMemos struct {
	bvns      []string
	threshold map[string]uint64
	active    map[string]int
}

func (g *GlobalValues) memoizeValidators() {
	if g.memoize.active != nil {
		return
	}

	var bvns []string
	for _, p := range g.Network.Partitions {
		if p.Type == protocol.PartitionTypeBlockValidator {
			bvns = append(bvns, p.ID)
		}
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

	g.memoize.bvns = bvns
	g.memoize.active = active
	g.memoize.threshold = threshold
}

func (g *GlobalValues) BvnNames() []string {
	g.memoizeValidators()
	return g.memoize.bvns
}

func (g *GlobalValues) ValidatorThreshold(partition string) uint64 {
	g.memoizeValidators()
	v, ok := g.memoize.threshold[strings.ToLower(partition)]
	if !ok {
		return math.MaxUint64
	}
	return v
}
