// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type ProcessTransactionState struct {
	ProducedTxns           []*protocol.Transaction
	AdditionalTransactions []*Delivery
	ChainUpdates           ChainUpdates
	MakeMajorBlock         uint64
	MakeMajorBlockTime     time.Time
	ReceivedAnchors        []*ReceivedAnchor
}

type ReceivedAnchor struct {
	Partition string
	Body      protocol.AnchorBody
	Index     int64
}

// DidProduceTxn records a produced transaction.
func (s *ProcessTransactionState) DidProduceTxn(url *url.URL, body protocol.TransactionBody) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = url
	txn.Body = body
	s.ProducedTxns = append(s.ProducedTxns, txn)
}

func (s *ProcessTransactionState) DidReceiveAnchor(partition string, body protocol.AnchorBody, index int64) {
	s.ReceivedAnchors = append(s.ReceivedAnchors, &ReceivedAnchor{partition, body, index})
}

func (s *ProcessTransactionState) ProcessAdditionalTransaction(txn *Delivery) {
	s.AdditionalTransactions = append(s.AdditionalTransactions, txn)
}

func (s *ProcessTransactionState) Merge(r *ProcessTransactionState) {
	if r.MakeMajorBlock > 0 {
		s.MakeMajorBlock = r.MakeMajorBlock
		s.MakeMajorBlockTime = r.MakeMajorBlockTime
	}
	s.ProducedTxns = append(s.ProducedTxns, r.ProducedTxns...)
	s.AdditionalTransactions = append(s.AdditionalTransactions, r.AdditionalTransactions...)
	s.ChainUpdates.Merge(&r.ChainUpdates)
	s.ReceivedAnchors = append(s.ReceivedAnchors, r.ReceivedAnchors...)
}

type ChainUpdates struct {
	Entries      []*protocol.BlockEntry
	SynthEntries []*database.BlockStateSynthTxnEntry
}

func (c *ChainUpdates) Merge(d *ChainUpdates) {
	for _, u := range d.Entries {
		c.DidUpdateChain(u)
	}
	c.SynthEntries = append(c.SynthEntries, d.SynthEntries...)
}

// DidUpdateChain records a chain update.
func (c *ChainUpdates) DidUpdateChain(update *protocol.BlockEntry) {
	c.Entries = append(c.Entries, update)
}

// DidAddChainEntry records a chain update in the block state.
func (c *ChainUpdates) DidAddChainEntry(batch *database.Batch, u *url.URL, name string, typ protocol.ChainType, entry []byte, index, sourceIndex, sourceBlock uint64) error {
	if name == protocol.MainChain && typ == protocol.ChainTypeTransaction {
		partition, ok := protocol.ParsePartitionUrl(u)
		if ok && protocol.PartitionUrl(partition).JoinPath(protocol.Synthetic).Equal(u) {
			c.SynthEntries = append(c.SynthEntries, &database.BlockStateSynthTxnEntry{
				Account:     u,
				Transaction: entry,
				ChainEntry:  index,
			})
		}
	}

	var update protocol.BlockEntry
	update.Account = u
	update.Chain = name
	update.Index = index
	c.DidUpdateChain(&update)
	return nil
}

// AddChainEntry adds an entry to a chain and records the chain update in the
// block state.
func (u *ChainUpdates) AddChainEntry(batch *database.Batch, chain *database.Chain2, entry []byte, sourceIndex, sourceBlock uint64) error {
	_, err := u.AddChainEntry2(batch, chain, entry, sourceIndex, sourceBlock, true)
	return err
}

func (u *ChainUpdates) AddChainEntry2(batch *database.Batch, chain *database.Chain2, entry []byte, sourceIndex, sourceBlock uint64, unique bool) (int64, error) {
	// Add an entry to the chain
	c, err := chain.Get()
	if err != nil {
		return 0, errors.UnknownError.WithFormat("load %s chain: %w", chain.Name(), err)
	}

	index := c.Height()
	err = c.AddEntry(entry, true)
	if err != nil {
		return 0, errors.UnknownError.WithFormat("add entry to %s chain: %w", chain.Name(), err)
	}

	// The entry was a duplicate, do not update the ledger
	if index == c.Height() {
		return c.HeightOf(entry)
	}

	// Update the ledger
	err = u.DidAddChainEntry(batch, chain.Account(), chain.Name(), chain.Type(), entry, uint64(index), sourceIndex, sourceBlock)
	if err != nil {
		return 0, errors.UnknownError.Wrap(err)
	}

	return index, nil
}
