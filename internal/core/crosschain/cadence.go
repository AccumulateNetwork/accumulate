// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
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

// selectedToSend reports whether this node is one of the validators that sends
// this activation's requests.
//
// The selection is over which node SENDS, not over what is requested. Every
// validator computes the same request set from the same agreed state, so a node
// that is not selected has already done the work and is ready to be selected at
// the next activation without discovering anything new.
//
// # Why the previous block's hash
//
// It is already agreed — consensus settled it, and every node has it before
// this block begins, so there is nothing new to distribute or to disagree
// about. It changes every block, where a clock need not: a partition producing
// several blocks a second would keep selecting the same pair while its stream
// fell further behind. And it is not anyone's to choose — a validator can nudge
// its own clock, and the node picking the senders should not be the node
// deciding who they are.
//
// The pair rotates with every activation because the hash does, so a validator
// that cannot reach a source stops being asked at the next activation and the
// load spreads instead of settling on whoever was picked first.
func (c *Conductor) selectedToSend(batch *database.Batch) (bool, error) {
	validators, err := c.partitionValidators()
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}

	// Nobody to choose between, or everybody is chosen anyway.
	if len(validators) <= sendersPerActivation {
		return true, nil
	}

	me := ed25519.PrivateKey(c.ValidatorKey).Public().(ed25519.PublicKey)
	mine := -1
	for i, v := range validators {
		if bytes.Equal(v, me) {
			mine = i
			break
		}
	}
	if mine < 0 {
		// Not a validator of this partition. Healing is a validator's job:
		// the answer re-enters through consensus, so a node that cannot
		// participate has nothing to contribute by asking.
		return false, nil
	}

	// The state hash the previous block committed. Reading it as this block
	// begins gives the hash of the block before — agreed, and available without
	// being distributed.
	prev, err := batch.GetBptRootHash()
	if err != nil {
		return false, errors.UnknownError.WithFormat("load previous block hash: %w", err)
	}

	// Adjacent indices rather than two independent draws, which could pick the
	// same validator twice and quietly leave one sender.
	first := int(binary.BigEndian.Uint64(prev[:8]) % uint64(len(validators)))
	for i := 0; i < sendersPerActivation; i++ {
		if (first+i)%len(validators) == mine {
			return true, nil
		}
	}
	return false, nil
}

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
