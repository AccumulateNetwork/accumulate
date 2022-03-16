package chain

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

// addChainEntry adds an entry to a chain and records the chain update in the
// block state.
func addChainEntry(block *BlockState, batch *database.Batch, account *url.URL, name string, typ protocol.ChainType, entry []byte, sourceIndex, sourceBlock uint64) error {
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
	return didAddChainEntry(block, batch, account, name, typ, entry, uint64(index), sourceIndex, sourceBlock)
}

// didAddChainEntry records a chain update in the block state.
func didAddChainEntry(block *BlockState, batch *database.Batch, u *url.URL, name string, typ protocol.ChainType, entry []byte, index, sourceIndex, sourceBlock uint64) error {
	if name == protocol.SyntheticChain && typ == protocol.ChainTypeTransaction {
		err := indexing.BlockState(batch, u).DidProduceSynthTxn(&indexing.BlockStateSynthTxnEntry{
			Transaction: entry,
			ChainEntry:  index,
		})
		if err != nil {
			return err
		}
	}

	var update ChainUpdate
	update.Name = name
	update.Type = typ
	update.Account = u
	update.Index = index
	update.SourceIndex = sourceIndex
	update.SourceBlock = sourceBlock
	update.Entry = entry
	block.DidUpdateChain(update)
	return nil
}

// shouldIndexChain returns true if the given chain should be indexed.
func shouldIndexChain(_ *url.URL, _ string, typ protocol.ChainType) (bool, error) {
	switch typ {
	case protocol.ChainTypeIndex:
		// Index chains are unindexed
		return false, nil

	case protocol.ChainTypeData:
		// Data chains are unindexed
		return false, nil

	case protocol.ChainTypeTransaction:
		// Transaction chains are indexed
		return true, nil

	case protocol.ChainTypeAnchor:
		// Anchor chains are indexed
		return true, nil

	default:
		// m.logError("Unknown chain type", "type", typ, "name", name, "account", account)
		return false, fmt.Errorf("unknown chain type")
	}
}

// addIndexChainEntry adds an entry to an index chain.
func addIndexChainEntry(account *database.Account, name string, entry *protocol.IndexEntry) (uint64, error) {
	// Load the index chain
	indexChain, err := account.Chain(name, protocol.ChainTypeIndex)
	if err != nil {
		return 0, err
	}

	// Marshal the entry
	data, err := entry.MarshalBinary()
	if err != nil {
		return 0, err
	}

	// TODO Update SMT to handle non-32-byte entries?
	if len(data) > 32 {
		panic("Index entry is too big")
	}
	if len(data) < 32 {
		padding := make([]byte, 32-len(data))
		// TODO Remove once AC-1096 is done
		// Fake field number to make unmarshalling work
		padding[0] = 32
		data = append(data, padding...)
	}

	// Add the entry
	err = indexChain.AddEntry(data, false)
	if err != nil {
		return 0, err
	}

	// Return the index of the entry
	return uint64(indexChain.Height() - 1), nil
}

// addChainAnchor anchors the target chain into the root chain, adding an index
// entry to the target chain's index chain, if appropriate.
func addChainAnchor(rootChain *database.Chain, account *database.Account, accountUrl *url.URL, name string, typ protocol.ChainType) (indexIndex uint64, didIndex bool, err error) {
	// Load the chain
	accountChain, err := account.ReadChain(name)
	if err != nil {
		return 0, false, err
	}

	// Add its anchor to the root chain
	err = rootChain.AddEntry(accountChain.Anchor(), false)
	if err != nil {
		return 0, false, err
	}

	// Check if it should be indexed
	shouldIndex, err := shouldIndexChain(accountUrl, name, typ)
	if err != nil || !shouldIndex {
		return 0, false, err
	}

	// Add the index chain entry
	indexIndex, err = addIndexChainEntry(account, protocol.IndexChain(name, false), &protocol.IndexEntry{
		Source: uint64(accountChain.Height() - 1),
		Anchor: uint64(rootChain.Height() - 1),
	})
	if err != nil {
		return 0, false, err
	}

	return indexIndex, true, nil
}

func loadDirectoryMetadata(batch *database.Batch, chainId []byte) (*protocol.DirectoryIndexMetadata, error) {
	b, err := batch.AccountByID(chainId).Index("Directory", "Metadata").Get()
	if err != nil {
		return nil, err
	}

	md := new(protocol.DirectoryIndexMetadata)
	err = md.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}

	return md, nil
}

func loadDirectoryEntry(batch *database.Batch, chainId []byte, index uint64) (string, error) {
	b, err := batch.AccountByID(chainId).Index("Directory", index).Get()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func mirrorRecord(batch *database.Batch, u *url.URL) (protocol.AnchoredRecord, error) {
	var arec protocol.AnchoredRecord

	rec := batch.Account(u)
	state, err := rec.GetState()
	if err != nil {
		return arec, fmt.Errorf("failed to load %q: %v", u, err)
	}

	chain, err := rec.ReadChain(protocol.MainChain)
	if err != nil {
		return arec, fmt.Errorf("failed to load main chain of %q: %v", u, err)
	}

	arec.Record, err = state.MarshalBinary()
	if err != nil {
		return arec, fmt.Errorf("failed to marshal %q: %v", u, err)
	}

	copy(arec.Anchor[:], chain.Anchor())
	return arec, nil
}

func buildProof(batch *database.Batch, u *ChainUpdate, rootChain *database.Chain, rootIndex, rootHeight int64) (*managed.Receipt, error) {
	anchorChain, err := batch.Account(u.Account).ReadChain(u.Name)
	if err != nil {
		return nil, err
	}

	anchorHeight := anchorChain.Height()
	r1, err := anchorChain.Receipt(int64(u.Index), anchorHeight-1)
	if err != nil {
		return nil, fmt.Errorf("unable to construct a receipt for %s#chain/%s: %w", u.Account, u.Name, err)
	}

	r2, err := rootChain.Receipt(rootIndex, rootHeight-1)
	if err != nil {
		return nil, fmt.Errorf("unable to construct a receipt for the root chain: %w", err)
	}

	r, err := r1.Combine(r2)
	if err != nil {
		return nil, err
	}

	return r, nil
}

func countExceptAnchors(batch *database.Batch, txids [][32]byte) int {
	var count int
	for _, hash := range txids {
		txn, err := batch.Transaction(hash[:]).GetState()
		if err != nil {
			count++
			continue
		}

		if txn.Type() != protocol.TransactionTypeSyntheticAnchor {
			count++
			continue
		}
	}
	return count
}

func countExceptAnchors2(txns []*protocol.Transaction) int {
	var count int
	for _, txn := range txns {
		if txn.Type() != protocol.TransactionTypeSyntheticAnchor {
			count++
			continue
		}
	}
	return count
}

func getPendingStatus(batch *database.Batch, header *protocol.TransactionHeader, status *protocol.TransactionStatus, resp *query.ResponseByTxId) error {
	// If it's not pending, don't bother
	if !status.Pending {
		return nil
	}

	origin, err := batch.Account(header.Origin).GetState()
	if err != nil {
		return err
	}

	// Find the origin's key book
	keyBook, ok := origin.(*protocol.KeyBook)
	switch {
	case ok:
		// Key books are their own key books
	case origin.Header().KeyBook == nil:
		// Lite token accounts don't have key books (and thus can't do multisig)
		return nil
	default:
		// Load the origin's key book
		keyBook = new(protocol.KeyBook)
		err := batch.Account(origin.Header().KeyBook).GetStateAs(keyBook)
		if err != nil {
			return fmt.Errorf("failed to load key book of %q: %v", origin.Header().Url, err)
		}
	}

	// Sanity check
	if header.KeyPageIndex >= keyBook.PageCount {
		return fmt.Errorf("invalid transaction: book has %d pages, transaction specifies page %d", keyBook.PageCount, header.KeyPageIndex)
	}

	// Read the page's main chain
	pageAcnt := batch.Account(protocol.FormatKeyPageUrl(keyBook.Url, header.KeyPageIndex))
	pageChain, err := pageAcnt.ReadChain(protocol.MainChain)
	if err != nil {
		return fmt.Errorf("failed to load main chain of key page %d of %q: %v", header.KeyPageIndex, origin.Header().Url, err)
	}

	// If height no longer matches, the transaction is invalidated
	if header.KeyPageHeight != uint64(pageChain.Height()) {
		resp.Invalidated = true
		return nil
	}

	// Load the page's state
	keyPage := new(protocol.KeyPage)
	err = pageAcnt.GetStateAs(keyPage)
	if err != nil {
		return fmt.Errorf("failed to load key page %d of %q: %v", header.KeyPageIndex, origin.Header().Url, err)
	}

	// Set the threshold
	resp.SignatureThreshold = keyPage.Threshold
	return nil
}

func getRangeFromIndexEntry(chain *database.Chain, index uint64) (from, to, anchor uint64, err error) {
	entry := new(protocol.IndexEntry)
	err = chain.EntryAs(int64(index), entry)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("entry %d: %w", index, err)
	}

	if index == 0 {
		return 0, entry.Source, entry.Anchor, nil
	}

	prev := new(protocol.IndexEntry)
	err = chain.EntryAs(int64(index)-1, prev)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("entry %d: %w", index-1, err)
	}

	return prev.Source + 1, entry.Source, entry.Anchor, nil
}

// combineReceipts combines multiple receipts and verifies the final value.
func combineReceipts(final []byte, receipts ...*managed.Receipt) (*managed.Receipt, error) {
	r := receipts[0]
	var err error
	for _, s := range receipts[1:] {
		r, err = r.Combine(s)
		if err != nil {
			return nil, fmt.Errorf("failed to combine receipts: %v", err)
		}
	}

	if final != nil && !bytes.Equal(final, r.MDRoot) {
		return nil, fmt.Errorf("invalid receipt end: want %X, got %X", final, r.MDRoot)
	}

	return r, nil
}
