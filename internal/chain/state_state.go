package chain

import (
	"fmt"
	"strings"

	"github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type ValidatorUpdate struct {
	PubKey  ed25519.PubKey
	Enabled bool
}

type ProcessTransactionState struct {
	ValidatorsUpdates      []ValidatorUpdate
	ProducedTxns           []*protocol.Transaction
	AdditionalTransactions []*Delivery
	ChainUpdates           ChainUpdates
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
	s.ValidatorsUpdates = append(s.ValidatorsUpdates, r.ValidatorsUpdates...)
	s.ProducedTxns = append(s.ProducedTxns, r.ProducedTxns...)
	s.AdditionalTransactions = append(s.AdditionalTransactions, r.AdditionalTransactions...)
	s.ChainUpdates.Merge(&r.ChainUpdates)
}

type ChainUpdates struct {
	chains  map[string]*indexing.ChainUpdate
	Entries []indexing.ChainUpdate
}

func (c *ChainUpdates) Merge(d *ChainUpdates) {
	for _, u := range d.Entries {
		c.DidUpdateChain(u)
	}
}

// DidUpdateChain records a chain update.
func (c *ChainUpdates) DidUpdateChain(update indexing.ChainUpdate) {
	if c.chains == nil {
		c.chains = map[string]*indexing.ChainUpdate{}
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
	if name == protocol.SyntheticChain && typ == protocol.ChainTypeTransaction {
		err := indexing.BlockState(batch, u).DidProduceSynthTxn(&indexing.BlockStateSynthTxnEntry{
			Transaction: entry,
			ChainEntry:  index,
		})
		if err != nil {
			return err
		}
	}

	var update indexing.ChainUpdate
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
func (c *ChainUpdates) AddChainEntry(batch *database.Batch, account *url.URL, name string, typ protocol.ChainType, entry []byte, sourceIndex, sourceBlock uint64) error {
	// Check if the account exists
	_, err := batch.Account(account).GetState()
	if err != nil {
		return err
	}

	// Add an entry to the chain
	chain, err := batch.Account(account).Chain(name, typ)
	if err != nil {
		return err
	}

	index := chain.Height()
	err = chain.AddEntry(entry, true)
	if err != nil {
		return err
	}

	// Update the ledger
	return c.DidAddChainEntry(batch, account, name, typ, entry, uint64(index), sourceIndex, sourceBlock)
}
