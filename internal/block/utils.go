package block

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

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

func loadDirectoryMetadata(batch *database.Batch, account *url.URL) (*protocol.DirectoryIndexMetadata, error) {
	b, err := batch.Account(account).Index("Directory", "Metadata").Get()
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

func loadDirectoryEntry(batch *database.Batch, account *url.URL, index uint64) (string, error) {
	b, err := batch.Account(account).Index("Directory", index).Get()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func mirrorRecord(batch *database.Batch, u *url.URL) (protocol.AnchoredAccount, error) {
	var arec protocol.AnchoredAccount

	rec := batch.Account(u)
	state, err := rec.GetState()
	if err != nil {
		return arec, fmt.Errorf("failed to load %q: %v", u, err)
	}

	chain, err := rec.ReadChain(protocol.MainChain)
	if err != nil {
		return arec, fmt.Errorf("failed to load main chain of %q: %v", u, err)
	}

	arec.Account = state
	copy(arec.Anchor[:], chain.Anchor())
	return arec, nil
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

func getAccountAuth(batch *database.Batch, account protocol.Account) (*protocol.AccountAuth, error) {
	switch account := account.(type) {
	case *protocol.LiteDataAccount:
		return &protocol.AccountAuth{
			Authorities: []protocol.AuthorityEntry{
				{Url: account.Url},
			},
		}, nil

	case protocol.FullAccount:
		return account.GetAuth(), nil

	case *protocol.KeyPage:
		bookUrl, _, ok := protocol.ParseKeyPageUrl(account.Url)
		if !ok {
			return nil, errors.Format(errors.StatusInternalError, "invalid key page URL: %v", account.Url)
		}
		var book *protocol.KeyBook
		err := batch.Account(bookUrl).GetStateAs(&book)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknown, err)
		}
		return book.GetAuth(), nil

	default:
		return &protocol.AccountAuth{}, nil
	}
}

func getValidator[T any](x *Executor, typ protocol.TransactionType) (T, bool) {
	var zero T

	txn, ok := x.executors[typ]
	if !ok {
		return zero, false
	}

	val, ok := txn.(T)
	return val, ok
}
