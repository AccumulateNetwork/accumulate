package indexing

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

// LoadIndexEntryFromEnd loads the Nth-to-last entry from an index chain.
// LoadIndexEntryFromEnd will panic if the offset is zero. If the offset is
// greater than the chain height, LoadIndexEntryFromEnd returns nil, nil.
func LoadIndexEntryFromEnd(c *database.Chain2, offset uint64) (*protocol.IndexEntry, error) {
	if offset == 0 {
		panic("offset must be > 0")
	}

	// Load the chain
	chain, err := c.Get()
	if err != nil {
		return nil, errors.StatusUnknownError.Format("get account chain %s: %w", c.Name(), err)
	}

	if chain.Height() < int64(offset) {
		return nil, nil
	}
	index := uint64(chain.Height()) - offset

	// Load the entry
	entry := new(protocol.IndexEntry)
	err = chain.EntryAs(int64(index), entry)
	if err != nil {
		return nil, errors.StatusUnknownError.Format("get account chain %s entry %d: %w", c.Name(), index, err)
	}

	return entry, nil
}

// LoadLastTwoIndexEntries loads the last and next to last entries of the index
// chain.
func LoadLastTwoIndexEntries(chain *database.Chain2) (last, nextLast *protocol.IndexEntry, err error) {
	last, err = LoadIndexEntryFromEnd(chain, 1)
	if err != nil {
		return nil, nil, errors.StatusUnknownError.Wrap(err)
	}
	if last == nil {
		return
	}

	nextLast, err = LoadIndexEntryFromEnd(chain, 2)
	if err != nil {
		return nil, nil, errors.StatusUnknownError.Wrap(err)
	}
	return
}

func getRootReceipt(net *config.Describe, batch *database.Batch, from, to int64) (*managed.Receipt, error) {
	localChain, err := batch.Account(net.Ledger()).RootChain().Get()
	if err != nil {
		return nil, errors.StatusUnknownError.Format("get minor root chain: %w", err)
	}

	local, err := localChain.Receipt(from, to)
	if err != nil {
		return nil, errors.StatusUnknownError.Format("unable to construct a receipt from %d to %d for the local root chain chain: %w", from, to, err)
	}

	// TODO Include the part of the receipt from the DN

	return local, nil
}

// loadIndexEntry loads an entry from an index chain.
func loadIndexEntry(c *database.Chain2, index uint64) (*protocol.IndexEntry, error) {
	// Load the chain
	chain, err := c.Index().Get()
	if err != nil {
		return nil, fmt.Errorf("unable to load the index chain of the %s chain: %w", c.Name(), err)
	}

	// Load the entry
	entry := new(protocol.IndexEntry)
	err = chain.EntryAs(int64(index), entry)
	if err != nil {
		return nil, fmt.Errorf("unable to load index entry %d for the index chain of the %s chain: %w", index, c.Name(), err)
	}

	return entry, nil
}

// getIndexedChainReceipt locates a chain entry and gets a receipt from that
// entry to an indexed anchor.
func getIndexedChainReceipt(c *database.Chain2, chainEntry []byte, indexEntry *protocol.IndexEntry) (*managed.Receipt, error) {
	// Load the chain
	chain, err := c.Get()
	if err != nil {
		return nil, fmt.Errorf("unable to load the %s chain: %w", c.Name(), err)
	}

	// Find the entry
	entryIndex, err := chain.HeightOf(chainEntry)
	if err != nil {
		return nil, fmt.Errorf("unable to find entry %X of the %s chain: %w", chainEntry, c.Name(), err)
	}

	// Get the receipt
	receipt, err := chain.Receipt(entryIndex, int64(indexEntry.Source))
	if err != nil {
		return nil, fmt.Errorf("unable to construct a receipt from %d to %d for chain %s: %w", entryIndex, indexEntry.Source, c.Name(), err)
	}

	return receipt, nil
}

func ReceiptForAccountState(net *config.Describe, batch *database.Batch, account *database.Account) (block uint64, receipt *managed.Receipt, err error) {
	// Get a receipt from the BPT
	r, err := account.StateReceipt()
	if err != nil {
		return 0, nil, errors.StatusUnknownError.Format("get account state receipt: %w", err)
	}

	// Load the latest root index entry (just for the block index)
	ledger := batch.Account(net.Ledger())
	rootEntry, err := LoadIndexEntryFromEnd(ledger.RootChain().Index(), 1)
	if err != nil {
		return 0, nil, errors.StatusUnknownError.Wrap(err)
	}

	return rootEntry.BlockIndex, r, nil
}

func ReceiptForChainEntry(net *config.Describe, batch *database.Batch, account *database.Account, hash []byte, entry *database.TransactionChainEntry) (uint64, *managed.Receipt, error) {
	c, err := account.ChainByName(entry.Chain)
	if err != nil {
		return 0, nil, err
	}

	// Load the index entry
	accountIndex, err := loadIndexEntry(c, entry.ChainIndex)
	if err != nil {
		return 0, nil, err
	}

	// Load the root index entry
	ledger := batch.Account(net.Ledger())
	rootIndex, err := loadIndexEntry(ledger.RootChain(), entry.AnchorIndex)
	if err != nil {
		return 0, nil, err
	}

	// Get a receipt from the account's chain
	accountReceipt, err := getIndexedChainReceipt(c, hash, accountIndex)
	if err != nil {
		return 0, nil, err
	}

	// Get a receipt from the root chain
	rootReceipt, err := getRootReceipt(net, batch, int64(accountIndex.Anchor), int64(rootIndex.Source))
	if err != nil {
		return 0, nil, err
	}

	// Finalize the receipt
	r, err := accountReceipt.Combine(rootReceipt)
	if err != nil {
		return 0, nil, err
	}

	return rootIndex.BlockIndex, r, nil
}

func ReceiptForChainIndex(net *config.Describe, batch *database.Batch, c *database.Chain2, index int64) (uint64, *managed.Receipt, error) {
	indexChain, err := c.Index().Get()
	if err != nil {
		return 0, nil, fmt.Errorf("unable to load %s index chain: %w", c.Name(), err)
	}

	_, entry, err := SearchIndexChain(indexChain, uint64(indexChain.Height())-1, MatchAfter, SearchIndexChainBySource(uint64(index)))
	if err != nil {
		return 0, nil, fmt.Errorf("unable to locate index entry for entry %d of %s chain: %w", index, c.Name(), err)
	}

	chain, err := c.Get()
	if err != nil {
		return 0, nil, fmt.Errorf("unable to load %s chain: %w", c.Name(), err)
	}

	rootIndexChain, err := batch.Account(net.Ledger()).RootChain().Index().Get()
	if err != nil {
		return 0, nil, fmt.Errorf("unable to load minor root index chain: %w", err)
	}

	_, rootEntry, err := SearchIndexChain(rootIndexChain, uint64(rootIndexChain.Height())-1, MatchAfter, SearchIndexChainBySource(entry.Anchor))
	if err != nil {
		return 0, nil, fmt.Errorf("unable to locate index entry for entry %d of the minor root chain: %w", entry.Anchor, err)
	}

	// Get a receipt from the account's chain
	accountReceipt, err := chain.Receipt(index, int64(entry.Source))
	if err != nil {
		return 0, nil, err
	}

	// Get a receipt from the root chain
	rootReceipt, err := getRootReceipt(net, batch, int64(entry.Anchor), int64(rootEntry.Source))
	if err != nil {
		return 0, nil, err
	}

	// Finalize the receipt
	r, err := accountReceipt.Combine(rootReceipt)
	if err != nil {
		return 0, nil, err
	}

	return rootEntry.BlockIndex, r, nil
}
