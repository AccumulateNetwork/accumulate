// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"sort"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

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
	PreviousStateHash  [32]byte

	Anchor *BlockAnchorState

	Pending map[[32]byte]*protocol.Transaction
	Events  int
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

	// Destination is the account that the message should be routed to.
	Destination *url.URL

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

func (s *BlockState) MarkTransactionPending(txn *protocol.Transaction) {
	if s.Pending == nil {
		s.Pending = map[[32]byte]*protocol.Transaction{}
	}
	s.Pending[txn.Hash()] = txn
}

func (s *BlockState) MarkTransactionDelivered(id *url.TxID) {
	if s.Pending == nil {
		return
	}
	delete(s.Pending, id.Hash())
}

func (s *BlockState) GetPending() []*protocol.Transaction {
	l := make([]*protocol.Transaction, 0, len(s.Pending))
	for _, p := range s.Pending {
		l = append(l, p)
	}
	sort.Slice(l, func(i, j int) bool {
		return l[i].ID().Compare(l[j].ID()) < 0
	})
	return l
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
