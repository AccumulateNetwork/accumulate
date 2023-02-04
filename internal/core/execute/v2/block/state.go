// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type Block struct {
	BlockMeta
	State BlockState
	Batch *database.Batch
}

// BlockMeta is metadata about a block.
type BlockMeta = execute.BlockParams

// BlockState tracks various metrics of a block of transactions as they are
// executed.
type BlockState struct {
	OpenedMajorBlock   bool
	MakeMajorBlock     uint64
	MakeMajorBlockTime time.Time
	Delivered          uint64
	Signed             uint64
	Produced           int
	ChainUpdates       chain.ChainUpdates
	ReceivedAnchors    []*chain.ReceivedAnchor

	Anchor *BlockAnchorState
}

// BlockAnchorState is used to construc the anchor for the block.
type BlockAnchorState struct {
	ShouldOpenMajorBlock bool
	OpenMajorBlockTime   time.Time
}

// ProducedMessage is a message produced by another message.
type ProducedMessage struct {
	// Producer is the ID of the producer.
	Producer *url.TxID

	// Message is the produced message.
	Message messaging.Message
}

// Empty returns true if nothing happened during the block.
func (s *BlockState) Empty() bool {
	return !s.OpenedMajorBlock &&
		s.Anchor == nil &&
		s.Delivered == 0 &&
		s.Signed == 0 &&
		s.Produced == 0 &&
		len(s.ChainUpdates.Entries) == 0
}

type ProcessSignatureState struct {
}

func (s *ProcessSignatureState) Merge(r *ProcessSignatureState) {
}

func (s *BlockState) MergeSignature(r *ProcessSignatureState) {
	s.Signed++
}

func (s *BlockState) MergeTransaction(r *chain.ProcessTransactionState) {
	s.Delivered++
	s.ChainUpdates.Merge(&r.ChainUpdates)
	s.ReceivedAnchors = append(s.ReceivedAnchors, r.ReceivedAnchors...)
	if r.MakeMajorBlock > 0 {
		s.MakeMajorBlock = r.MakeMajorBlock
		s.MakeMajorBlockTime = r.MakeMajorBlockTime
	}
}
