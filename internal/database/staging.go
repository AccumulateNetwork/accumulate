// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Staging (#4189) is what a node has received on an ordered cross-partition
// stream and cannot execute yet: Sequenced(source, number) is the message,
// Sighted(source) is how far the stream has been seen.
//
// These index functions exist so that a SNAPSHOT collects them. A parameterised
// record is walked only if it can be enumerated, and one that cannot be walked
// is silently absent from the snapshot — which for staging would be a divergent
// block hash on the restored node's first block, since staging decides what a
// block executes. Nothing else uses them; the executor and healing address
// these records by name.

// getSightedKeys enumerates the sources this account has staged anything from.
func (c *Account) getSightedKeys() ([]accountSightedKey, error) {
	sources, err := c.StagedSources().Get()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	keys := make([]accountSightedKey, 0, len(sources))
	for _, u := range sources {
		keys = append(keys, accountSightedKey{Source: u})
	}
	return keys, nil
}

// getSequencedKeys enumerates the numbers this account is HOLDING — received
// and above the stream's delivery watermark.
//
// Only what is held, deliberately. A record below `Delivered` is a message that
// has executed; nothing consults it, because the watermark is the cutoff, and
// carrying the whole history of every stream into every snapshot would be
// enormous and would restore state that answers no question.
//
// The scan probes the range rather than reading a list of held numbers, because
// a list is a record that grows without bound and gets rewritten whole — which
// is precisely what staging exists to stop doing. This runs when a snapshot is
// collected, not on the block path, and its cost is the size of the backlog.
func (c *Account) getSequencedKeys() ([]accountSequencedKey, error) {
	sources, err := c.StagedSources().Get()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if len(sources) == 0 {
		return nil, nil
	}

	// Delivered comes from the ledger, and only Delivered. An account that
	// staged something but carries no sequence ledger has delivered nothing,
	// which is a watermark of zero and not an error.
	// Read, never Partition(): that finds OR CREATES, and creating an entry on
	// the batch's memoized copy of Main would edit hashed state from a read.
	delivered := map[[32]byte]uint64{}
	var seq []*protocol.PartitionSyntheticLedger
	switch main, err := c.Main().Get(); {
	case errors.Is(err, errors.NotFound):
		// Staged something, delivered nothing. Zero is the answer.
	case err != nil:
		return nil, errors.UnknownError.Wrap(err)
	default:
		switch l := main.(type) {
		case *protocol.SyntheticLedger:
			seq = l.Sequence
		case *protocol.AnchorLedger:
			seq = l.Sequence
		}
	}
	for _, part := range seq {
		if part.Url != nil {
			delivered[part.Url.AccountID32()] = part.Delivered
		}
	}

	var keys []accountSequencedKey
	for _, source := range sources {
		high, err := c.Sighted(source).Get()
		switch {
		case errors.Is(err, errors.NotFound):
			continue
		case err != nil:
			return nil, errors.UnknownError.Wrap(err)
		}

		for n := delivered[source.AccountID32()] + 1; n <= high; n++ {
			_, err := c.Sequenced(source, n).Get()
			switch {
			case errors.Is(err, errors.NotFound):
				continue // a hole, and a hole is not a record
			case err != nil:
				return nil, errors.UnknownError.Wrap(err)
			}
			keys = append(keys, accountSequencedKey{Source: source, Number: n})
		}
	}
	return keys, nil
}
