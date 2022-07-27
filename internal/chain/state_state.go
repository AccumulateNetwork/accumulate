package chain

import (
	"fmt"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type ProcessTransactionState struct {
	ProducedTxns           []*protocol.Transaction
	AdditionalTransactions []*Delivery
	ChainUpdates           ChainUpdates
	MakeMajorBlock         uint64
	MakeMajorBlockTime     time.Time

	SynthToSend []*protocol.Envelope
}

// DidProduceTxn records a produced transaction.
func (s *ProcessTransactionState) DidProduceTxn(url *url.URL, body protocol.TransactionBody) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = url
	txn.Body = body
	s.ProducedTxns = append(s.ProducedTxns, txn)
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

	s.SynthToSend = append(s.SynthToSend, r.SynthToSend...)
}

type ChainUpdates struct {
	chains  map[string]*database.ChainUpdate
	Entries []database.ChainUpdate
}

func (c *ChainUpdates) Merge(d *ChainUpdates) {
	for _, u := range d.Entries {
		c.DidUpdateChain(u)
	}
}

// DidUpdateChain records a chain update.
func (c *ChainUpdates) DidUpdateChain(update database.ChainUpdate) {
	if c.chains == nil {
		c.chains = map[string]*database.ChainUpdate{}
	}

	str := strings.ToLower(fmt.Sprintf("%s#chain/%s", update.Account, update.Name))
	ptr, ok := c.chains[str]
	if ok {
		*ptr = update
		return
	}

	i := len(c.Entries)
	c.Entries = append(c.Entries, update)
	c.chains[str] = &c.Entries[i]
}

// DidAddChainEntry records a chain update in the block state.
func (c *ChainUpdates) DidAddChainEntry(batch *database.Batch, u *url.URL, name string, typ protocol.ChainType, entry []byte, index, sourceIndex, sourceBlock uint64) error {
	if name == protocol.MainChain && typ == protocol.ChainTypeTransaction {
		partition, ok := protocol.ParsePartitionUrl(u)
		if ok && protocol.PartitionUrl(partition).JoinPath(protocol.Synthetic).Equal(u) {
			err := indexing.BlockState(batch, u).DidProduceSynthTxn(&database.BlockStateSynthTxnEntry{
				Transaction: entry,
				ChainEntry:  index,
			})
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "load block state: %w", err)
			}
		}
	}

	var update database.ChainUpdate
	update.Name = name
	update.Type = typ
	update.Account = u
	update.Index = index
	update.SourceIndex = sourceIndex
	update.SourceBlock = sourceBlock
	update.Entry = entry
	c.DidUpdateChain(update)
	return nil
}

// AddChainEntry adds an entry to a chain and records the chain update in the
// block state.
func (u *ChainUpdates) AddChainEntry(batch *database.Batch, chain *database.Chain2, entry []byte, sourceIndex, sourceBlock uint64) error {
	// Add an entry to the chain
	c, err := chain.Get()
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load %s chain: %w", chain.Name(), err)
	}

	index := c.Height()
	err = c.AddEntry(entry, true)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "add entry to %s chain: %w", chain.Name(), err)
	}

	// The entry was a duplicate, do not update the ledger
	if index == c.Height() {
		return nil
	}

	// Update the ledger
	return u.DidAddChainEntry(batch, chain.Account(), chain.Name(), chain.Type(), entry, uint64(index), sourceIndex, sourceBlock)
}
