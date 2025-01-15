// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/shared"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// orderedMap is an ordered map from K to V implemented with a builtin map,
// slice of keys, and comparison function.
type orderedMap[K comparable, V any] struct {
	theMap map[K]V
	keys   []K
	cmp    func(u, v K) int
}

// Set sets the value of the given key to the given value.
func (m *orderedMap[K, V]) Set(k K, v V) {
	if m.theMap == nil {
		m.theMap = map[K]V{}
	}
	m.theMap[k] = v
	ptr, new := sortutil.BinaryInsert(&m.keys, func(l K) int { return m.cmp(l, k) })
	if new {
		*ptr = k
	}
}

// Get retrieves the value of the given key.
func (m *orderedMap[K, V]) Get(k K) (V, bool) {
	v, ok := m.theMap[k]
	return v, ok
}

// For iterates over the map in order.
func (m *orderedMap[K, V]) For(fn func(k K, v V) error) error {
	for _, k := range m.keys {
		k := k // See docs/developer/rangevarref.md
		err := fn(k, m.theMap[k])
		if err != nil {
			return err
		}
	}
	return nil
}

// shouldIndexChain returns true if the given chain should be indexed.
func shouldIndexChain(_ *url.URL, _ string, typ protocol.ChainType) (bool, error) {
	switch typ {
	case protocol.ChainTypeIndex:
		// Index chains are unindexed
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
func addIndexChainEntry(chain *database.Chain2, entry *protocol.IndexEntry) (uint64, error) {
	// Load the index chain
	indexChain, err := chain.Get()
	if err != nil {
		return 0, err
	}

	// Marshal the entry
	data, err := entry.MarshalBinary()
	if err != nil {
		return 0, err
	}

	// Add the entry
	_ = data
	err = indexChain.AddEntry(data, false)
	if err != nil {
		return 0, err
	}

	// Return the index of the entry
	return uint64(indexChain.Height() - 1), nil
}

// addChainAnchor anchors the target chain into the root chain, adding an index
// entry to the target chain's index chain, if appropriate.
func addChainAnchor(rootChain *database.Chain, chain *database.Chain2, blockIndex uint64) (indexIndex uint64, didIndex bool, err error) {
	// Load the chain
	accountChain, err := chain.Get()
	if err != nil {
		return 0, false, err
	}

	// Add its anchor to the root chain
	err = rootChain.AddEntry(accountChain.Anchor(), false)
	if err != nil {
		return 0, false, err
	}

	// Check if it should be indexed
	shouldIndex, err := shouldIndexChain(chain.Account(), chain.Name(), chain.Type())
	if err != nil || !shouldIndex {
		return 0, false, err
	}

	// Add the index chain entry
	indexIndex, err = addIndexChainEntry(chain.Index(), &protocol.IndexEntry{
		BlockIndex: blockIndex,
		Source:     uint64(accountChain.Height() - 1),
		Anchor:     uint64(rootChain.Height() - 1),
	})
	if err != nil {
		return 0, false, err
	}

	return indexIndex, true, nil
}

func (m *MessageContext) getAccountAuthoritySet(batch *database.Batch, account protocol.Account) (*protocol.AccountAuth, error) {
	auth, url, err := shared.GetAccountAuthoritySet(account)
	switch {
	case err != nil:
		return nil, errors.UnknownError.Wrap(err)

	case auth == nil:
		// Look at <url>
		break

	case len(auth.Authorities) > 0:
		// Account has it's own authorities
		return auth, nil

	case !m.GetActiveGlobals().ExecutorVersion.V2BaikonurEnabled():
		// Old logic
		return auth, nil

	case account.Type() == protocol.AccountTypeLiteDataAccount:
		// LDAs have no authorities
		return auth, nil

	default:
		// Inherit authority
		var ok bool
		url, ok = account.GetUrl().Parent()
		if !ok {
			return nil, errors.InvalidRecord.WithFormat("root account %v has an empty authority set", account.GetUrl())
		}
	}

	account, err = batch.Account(url).Main().Get()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return m.getAccountAuthoritySet(batch, account)
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
