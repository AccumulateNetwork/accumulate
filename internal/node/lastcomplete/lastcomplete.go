// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package lastcomplete answers, for a partition that has lost its quorum,
// which block was its last complete one (executor spec, "A partition that
// loses a quorum of its validators' states"; #4512).
package lastcomplete

import (
	"context"
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// State says whether a node can obtain the state as of N.
type State int

const (
	// Unknown: N is not known, so nothing is said of its state.
	Unknown State = iota

	// AtHand: the node's executed height is N; its state is N's.
	AtHand

	// Retained: N is within the node's retained BPT history. The root the
	// node recorded for N is its own word, not signed: it must be matched
	// against the root N's anchor carries.
	Retained

	// NotObtainable: the node executed past N and keeps no history that
	// reaches it (the default configuration keeps none).
	NotObtainable

	// Unreadable: the node's retained history could not be read, so nothing
	// is said of whether it reaches N. StateErr says why.
	Unreadable
)

func (s State) String() string {
	switch s {
	case AtHand:
		return "at hand: the node's executed height is N"
	case Retained:
		return "in retained history: the root the node recorded for N must be matched against the anchored root"
	case NotObtainable:
		return "not obtainable: the node executed past N and keeps no history that reaches it"
	case Unreadable:
		return "unreadable: the node's retained history could not be read"
	}
	return "unknown"
}

// Answer is what a node can say about its partition's last complete block.
type Answer struct {
	// Known is true when N was read from the Directory's record. When false,
	// Why says why not; only Executed, and Anchor when the Directory
	// named one, are set.
	Known bool

	// Block is N: the block of the newest anchor of this partition the
	// Directory holds with the threshold's signatures.
	Block uint64

	// Root is the state root the anchor for N carries (StateTreeAnchor).
	Root [32]byte

	// Anchor is the number of that anchor in the partition's stream to the
	// Directory (the Directory's Delivered for the partition).
	Anchor uint64

	// Executed is the block this node has executed.
	Executed uint64

	// Ahead is how many blocks this node has executed past N. Nothing after
	// N is signed by a quorum; the anchor of a block is made as the next one
	// begins, so a node one block ahead is the usual case.
	Ahead uint64

	// State says whether the node can obtain the state as of N.
	State State

	// StateErr is why State is Unreadable.
	StateErr string

	// Why is the reason N is unknown.
	Why string
}

// Find answers for partition from the Directory's anchor ledger -- or the
// error that reading it ended in -- and this node's own store.
//
// The Directory executes an anchor only with the threshold's signatures of
// the partition's validator set (or a collection proof under a directory root
// it holds), so the number it has delivered from the partition names the
// newest anchor it holds complete. A partition's own pool never holds its
// peers' signatures, so nothing local can say it. A node that cannot read the
// Directory says N is unknown; it does not take it from its own unsigned
// state.
func Find(partition *url.URL, db *database.Database, directory *protocol.AnchorLedger, directoryErr error) Answer {
	var a Answer
	err := db.View(func(batch *database.Batch) error {
		var ledger *protocol.SystemLedger
		err := batch.Account(partition.JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
		if err != nil {
			return errors.UnknownError.WithFormat("load this node's ledger: %w", err)
		}
		a.Executed = ledger.Index

		switch {
		case directoryErr != nil:
			a.Why = fmt.Sprintf("the Directory could not be read: %v", directoryErr)
			return nil
		case directory == nil:
			a.Why = "the Directory's anchor ledger was not read"
			return nil
		}
		for _, s := range directory.Sequence {
			if s != nil && s.Url != nil && s.Url.RootIdentity().Equal(partition) {
				a.Anchor = s.Delivered
			}
		}
		if a.Anchor == 0 {
			a.Why = "the Directory has delivered none of this partition's anchors"
			return nil
		}
		pa, err := AnchorOf(batch, partition, a.Anchor)
		if err != nil {
			a.Why = fmt.Sprintf("this node cannot read the anchor the Directory delivered (number %d): %v", a.Anchor, err)
			return nil
		}
		a.Known, a.Block, a.Root = true, pa.MinorBlockIndex, pa.StateTreeAnchor
		if a.Executed >= a.Block {
			a.Ahead = a.Executed - a.Block
		}

		rng, rngErr := indexing.RetainedBlockRange(config.NetworkUrl{URL: partition}, batch)
		a.State, a.StateErr = StateAsOf(a.Executed, a.Block, rng, rngErr)
		return nil
	})
	if err != nil {
		return Answer{Executed: a.Executed, Why: err.Error()}
	}
	return a
}

// StateAsOf says whether a node that has executed height executed can obtain
// the state as of block, given the history it retains (or the error reading
// that ended in). A read failure is its own answer: it is not "keeps no
// history".
func StateAsOf(executed, block uint64, retained indexing.BlockRange, retainedErr error) (State, string) {
	switch {
	case executed == block:
		return AtHand, ""
	case executed < block:
		// A node that has not executed N holds no state as of it.
		return NotObtainable, ""
	case retainedErr != nil:
		return Unreadable, retainedErr.Error()
	case retained.Contains(block):
		return Retained, ""
	}
	return NotObtainable, ""
}

// String says it for a log line: N and its root, how far this node is past
// it, and whether the state as of N can be had.
func (a Answer) String() string {
	if !a.Known {
		if a.Anchor != 0 {
			return fmt.Sprintf("the last complete block is unknown (the Directory has delivered anchor %d of this partition): %s", a.Anchor, a.Why)
		}
		return fmt.Sprintf("the last complete block is unknown: %s", a.Why)
	}
	state := a.State.String()
	if a.State == Unreadable {
		state += ": " + a.StateErr
	}
	return fmt.Sprintf("last complete block %d (anchor %d, root %x), executed %d, %d past it; state as of it %s",
		a.Block, a.Anchor, a.Root[:8], a.Executed, a.Ahead, state)
}

// AnchorOf is the partition anchor number n in the partition's own anchor
// chain (the hash of entry n-1 of its anchor-sequence chain names the
// anchor transaction the node stored).
func AnchorOf(batch *database.Batch, partition *url.URL, n uint64) (*protocol.PartitionAnchor, error) {
	if n == 0 {
		return nil, errors.BadRequest.With("anchor numbers start at 1")
	}
	pool := partition.JoinPath(protocol.AnchorPool)
	chain, err := batch.Account(pool).AnchorSequenceChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor sequence chain: %w", err)
	}
	if uint64(chain.Height()) < n {
		return nil, errors.NotFound.WithFormat("this node has produced %d anchors, not %d", chain.Height(), n)
	}
	hash, err := chain.Entry(int64(n - 1))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor sequence entry %d: %w", n-1, err)
	}
	var msg messaging.MessageWithTransaction
	err = batch.Message2(hash).Main().GetAs(&msg)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor %d: %w", n, err)
	}
	body, ok := msg.GetTransaction().Body.(protocol.AnchorBody)
	if !ok || body.GetPartitionAnchor() == nil {
		return nil, errors.Conflict.WithFormat("entry %d of the anchor sequence is a %v, not an anchor", n, msg.GetTransaction().Body.Type())
	}
	return body.GetPartitionAnchor(), nil
}

// Source is the read a node makes to say its partition's last complete block:
// the Directory's anchor ledger through q, and this node's own store. It is
// nil for the Directory, whose anchor ledger counts the BVNs' anchors and
// holds its own (the answer would be a false "none delivered").
func Source(q api.Querier, db *database.Database, partition string) func(context.Context) Answer {
	if strings.EqualFold(partition, protocol.Directory) {
		return nil
	}
	return func(ctx context.Context) Answer {
		var ledger *protocol.AnchorLedger
		_, err := api.Querier2{Querier: q}.QueryAccountAs(ctx, protocol.DnUrl().JoinPath(protocol.AnchorPool), nil, &ledger)
		return Find(protocol.PartitionUrl(partition), db, ledger, err)
	}
}
