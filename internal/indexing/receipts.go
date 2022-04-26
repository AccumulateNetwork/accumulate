package indexing

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	. "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

// LoadIndexEntryFromEnd loads the Nth-to-last entry from an index chain.
// LoadIndexEntryFromEnd will panic if the offset is zero. If the offset is
// greater than the chain height, LoadIndexEntryFromEnd returns nil, nil.
func LoadIndexEntryFromEnd(account *Account, indexChain string, offset uint64) (*protocol.IndexEntry, error) {
	if offset == 0 {
		panic("offset must be > 0")
	}

	// Load the chain
	chain, err := account.ReadChain(indexChain)
	if err != nil {
		return nil, errors.Unknown("get account chain %s: %w", indexChain, err)
	}

	if chain.Height() < int64(offset) {
		return nil, nil
	}
	index := uint64(chain.Height()) - offset

	// Load the entry
	entry := new(protocol.IndexEntry)
	err = chain.EntryAs(int64(index), entry)
	if err != nil {
		return nil, errors.Unknown("get account chain %s entry %d: %w", indexChain, index, err)
	}

	return entry, nil
}

// LoadLastTwoIndexEntries loads the last and next to last entries of the index
// chain.
func LoadLastTwoIndexEntries(account *Account, indexChain string) (last, nextLast *protocol.IndexEntry, err error) {
	last, err = LoadIndexEntryFromEnd(account, indexChain, 1)
	if err != nil {
		return nil, nil, errors.Wrap(errors.StatusUnknown, err)
	}
	if last == nil {
		return
	}

	nextLast, err = LoadIndexEntryFromEnd(account, indexChain, 2)
	if err != nil {
		return nil, nil, errors.Wrap(errors.StatusUnknown, err)
	}
	return
}

func getRootReceipt(net *config.Network, batch *Batch, from, to int64) (*managed.Receipt, error) {
	localChain, err := batch.Account(net.Ledger()).ReadChain(protocol.MinorRootChain)
	if err != nil {
		return nil, errors.Unknown("get minor root chain: %w", err)
	}

	local, err := localChain.Receipt(from, to)
	if err != nil {
		return nil, errors.Unknown("unable to construct a receipt from %d to %d for the local root chain chain: %w", from, to, err)
	}

	// TODO Include the part of the receipt from the DN

	return local, nil
}

// loadIndexEntry loads an entry from an index chain.
func loadIndexEntry(account *Account, chainName, indexChain string, index uint64) (*protocol.IndexEntry, error) {
	// Load the chain
	chain, err := account.ReadChain(indexChain)
	if err != nil {
		return nil, fmt.Errorf("unable to load the index chain of the %s chain: %w", chainName, err)
	}

	// Load the entry
	entry := new(protocol.IndexEntry)
	err = chain.EntryAs(int64(index), entry)
	if err != nil {
		return nil, fmt.Errorf("unable to load index entry %d for the index chain of the %s chain: %w", index, chainName, err)
	}

	return entry, nil
}

// getIndexedChainReceipt locates a chain entry and gets a receipt from that
// entry to an indexed anchor.
func getIndexedChainReceipt(account *Account, name string, chainEntry []byte, indexEntry *protocol.IndexEntry) (*managed.Receipt, error) {
	// Load the chain
	chain, err := account.ReadChain(name)
	if err != nil {
		return nil, fmt.Errorf("unable to load the %s chain: %w", name, err)
	}

	// Find the entry
	entryIndex, err := chain.HeightOf(chainEntry)
	if err != nil {
		return nil, fmt.Errorf("unable to find entry %X of the %s chain: %w", chainEntry, name, err)
	}

	// Get the receipt
	receipt, err := chain.Receipt(entryIndex, int64(indexEntry.Source))
	if err != nil {
		return nil, fmt.Errorf("unable to construct a receipt from %d to %d for chain %s: %w", entryIndex, indexEntry.Source, name, err)
	}

	return receipt, nil
}

func ReceiptForAccountState(net *config.Network, batch *Batch, account *Account) (block uint64, receipt *managed.Receipt, err error) {
	// Get a receipt from the BPT
	rState, err := account.StateReceipt()
	if err != nil {
		return 0, nil, errors.Unknown("get account state receipt: %w", err)
	}

	// Load the latest root index entry
	ledger := batch.Account(net.Ledger())
	rootEntry, err := LoadIndexEntryFromEnd(ledger, protocol.MinorRootIndexChain, 1)
	if err != nil {
		return 0, nil, errors.Wrap(errors.StatusUnknown, err)
	}

	// Get the receipt for the BPT - the BPT's entry is always the last entry in
	// a block
	rRoot, err := getRootReceipt(net, batch, int64(rootEntry.Source), int64(rootEntry.Source))
	if err != nil {
		return 0, nil, errors.Wrap(errors.StatusUnknown, err)
	}

	// Create the full receipt
	r, err := managed.CombineReceipts(rState, rRoot)
	if err != nil {
		return 0, nil, errors.Unknown("unable to combine state-BPT receipt with BPT-root receipt: %w", err)
	}

	return rootEntry.BlockIndex, r, nil
}

func ReceiptForChainEntry(net *config.Network, batch *Batch, account *Account, hash []byte, entry *TransactionChainEntry) (uint64, *managed.Receipt, error) {
	// Load the index entry
	accountIndex, err := loadIndexEntry(account, entry.Chain, protocol.IndexChain(entry.Chain, false), entry.ChainIndex)
	if err != nil {
		return 0, nil, err
	}

	// Load the root index entry
	ledger := batch.Account(net.Ledger())
	rootIndex, err := loadIndexEntry(ledger, protocol.MinorRootChain, protocol.MinorRootIndexChain, entry.AnchorIndex)
	if err != nil {
		return 0, nil, err
	}

	// Get a receipt from the account's chain
	accountReceipt, err := getIndexedChainReceipt(account, entry.Chain, hash, accountIndex)
	if err != nil {
		return 0, nil, err
	}

	// Get a receipt from the root chain
	rootReceipt, err := getRootReceipt(net, batch, int64(accountIndex.Anchor), int64(rootIndex.Source))
	if err != nil {
		return 0, nil, err
	}

	// Finalize the receipt
	r, err := managed.CombineReceipts(accountReceipt, rootReceipt)
	if err != nil {
		return 0, nil, err
	}

	return rootIndex.BlockIndex, r, nil
}

func ReceiptForChainIndex(net *config.Network, batch *Batch, account *Account, name string, index int64) (uint64, *managed.Receipt, error) {
	indexChain, err := account.ReadIndexChain(name, false)
	if err != nil {
		return 0, nil, fmt.Errorf("unable to load %s index chain of %v: %w", name, account, err)
	}

	_, entry, err := SearchIndexChain(indexChain, uint64(indexChain.Height())-1, MatchAfter, SearchIndexChainBySource(uint64(index)))
	if err != nil {
		return 0, nil, fmt.Errorf("unable to locate index entry for entry %d of %s chain of %v: %w", index, name, account, err)
	}

	chain, err := account.ReadChain(name)
	if err != nil {
		return 0, nil, fmt.Errorf("unable to load %s chain of %v: %w", name, account, err)
	}

	rootIndexChain, err := batch.Account(net.Ledger()).ReadChain(protocol.MinorRootIndexChain)
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
	r, err := managed.CombineReceipts(accountReceipt, rootReceipt)
	if err != nil {
		return 0, nil, err
	}

	return rootEntry.BlockIndex, r, nil
}
