// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"math/big"
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
	AcmeBurnt          big.Int
	NetworkUpdate      []*protocol.NetworkAccountUpdate

	Anchor     *BlockAnchorState
	MajorBlock *MajorBlockState

	PendingTxns map[[32]byte]*protocol.Transaction
	PendingSigs map[[32]byte]*PendingAuthSig
	Events      int
}

type PendingAuthSig struct {
	Transaction *protocol.Transaction
	Authorities map[[32]byte]*url.URL
}

// BlockAnchorState is used to construct the anchor for the block.
type BlockAnchorState struct {
}

type MajorBlockState struct {
	Time  time.Time
	Index uint64
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
		s.MajorBlock == nil &&
		s.Delivered == 0 &&
		s.Signed == 0 &&
		s.Produced == 0 &&
		len(s.ChainUpdates.Entries) == 0
}

func (s *BlockState) MarkTransactionPending(txn *protocol.Transaction) {
	if s.PendingTxns == nil {
		s.PendingTxns = map[[32]byte]*protocol.Transaction{}
	}
	s.PendingTxns[txn.Hash()] = txn
}

func (s *BlockState) MarkSignaturePending(txn *protocol.Transaction, auth *url.URL) {
	if s.PendingSigs == nil {
		s.PendingSigs = map[[32]byte]*PendingAuthSig{}
	}
	t, ok := s.PendingSigs[txn.Hash()]
	if !ok {
		t = new(PendingAuthSig)
		t.Transaction = txn
		t.Authorities = map[[32]byte]*url.URL{}
		s.PendingSigs[txn.Hash()] = t
	}
	t.Authorities[auth.Hash32()] = auth
}

func (s *BlockState) MarkTransactionDelivered(id *url.TxID) {
	if s.PendingTxns != nil {
		delete(s.PendingTxns, id.Hash())
	}
	if s.PendingSigs != nil {
		delete(s.PendingSigs, id.Hash())
	}
}

func (s *BlockState) GetPendingTxns() []*protocol.Transaction {
	return sortMapValues(s.PendingTxns, func(a, b *protocol.Transaction) int {
		return a.ID().Compare(b.ID())
	})
}

func (s *BlockState) GetPendingSigs() []*PendingAuthSig {
	return sortMapValues(s.PendingSigs, func(a, b *PendingAuthSig) int {
		return a.Transaction.ID().Compare(b.Transaction.ID())
	})
}

func (p *PendingAuthSig) GetAuthorities() []*url.URL {
	return sortMapValues(p.Authorities, (*url.URL).Compare)
}

func sortMapValues[V any](m map[[32]byte]V, cmp func(a, b V) int) []V {
	l := make([]V, 0, len(m))
	for _, p := range m {
		l = append(l, p)
	}
	sort.Slice(l, func(i, j int) bool {
		return cmp(l[i], l[j]) < 0
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
	s.AcmeBurnt.Add(&s.AcmeBurnt, &r.AcmeBurnt)
	s.NetworkUpdate = append(s.NetworkUpdate, r.NetworkUpdate...)
}
