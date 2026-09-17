// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"bytes"
	"crypto/ed25519"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// healCadence is how many blocks pass between healing activations (#4201).
//
// Not every block, because a request crosses to another partition and its
// answer returns through consensus, which takes blocks. Activating every block
// would re-request gaps whose answers are still in flight, so a stream that is
// behind would generate requests at the block rate for messages already on
// their way.
//
// The number is small and not magic; two may be enough. It is a constant rather
// than a setting because healing has no switches: a lost message wedges a
// stream permanently, so recovery is not something to configure and get wrong.
const healCadence = 4

// healActivates reports whether healing runs for this block.
//
// The block index, so every node activates on the same blocks — which is what
// makes "two of us send" mean two nodes and not two per-node timers that happen
// to overlap.
func healActivates(blockIndex uint64) bool {
	return blockIndex%healCadence == 0
}

// sendersPerActivation is how many validators actually send an activation's
// requests.
//
// Two rather than one, because one is a single point of failure: if the chosen
// validator is down or cannot reach the source, the gap goes unrequested for
// that activation and healing waits on a node that is not going to answer.
//
// Two rather than all, because N validators asking for the same message is N−1
// wasted round trips at exactly the moment a stream is already behind — and the
// extra answers are discarded anyway, since the block's sort keeps the first
// sighting of a sequence number.
const sendersPerActivation = 2

// partitionValidators returns this partition's active validator keys, sorted,
// so every node derives the same list in the same order from the same globals.
func (c *Conductor) partitionValidators() ([]ed25519.PublicKey, error) {
	globals := c.Globals.Load()
	if globals == nil || globals.Network == nil {
		return nil, errors.NotReady.With("globals are not loaded")
	}

	var keys []ed25519.PublicKey
	for _, v := range globals.Network.Validators {
		if v.IsActiveOn(c.Partition.ID) && len(v.PublicKey) == ed25519.PublicKeySize {
			keys = append(keys, ed25519.PublicKey(v.PublicKey))
		}
	}
	sort.Slice(keys, func(i, j int) bool { return bytes.Compare(keys[i], keys[j]) < 0 })
	return keys, nil
}
