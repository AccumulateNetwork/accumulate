// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package keybookat

import (
	"errors"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ErrUnsupportedTxType is returned when the page's main chain contains a
// transaction the replay logic doesn't know how to apply. Recoverable
// callers can fall back to the no-mutation-after-t scaffolding path.
var ErrUnsupportedTxType = errors.New("keybookat: unsupported transaction type during replay")

// replayPage replays the page's main chain forward to time t and returns
// the resulting page state. Starting state is an empty KeyPage with the
// page's URL; the chain's entries (Create*Page or UpdateKeyPage) build
// up the page from there.
//
// For block time per entry we walk the page's main-index chain and
// match each main-chain index against the most recent index entry with
// `Source >= main_index`. Block time comes from that index entry's
// BlockTime field.
//
// Returns ErrUnsupportedTxType if the chain contains a transaction type
// the replay logic doesn't yet handle (e.g., key updates from external
// keybook owners). Caller can decide whether to surface or fall back.
func replayPage(batch *database.Batch, book *protocol.KeyBook, pageUrl *url.URL, t time.Time) (*protocol.KeyPage, error) {
	acct := batch.Account(pageUrl)

	mainChain, err := acct.MainChain().Get()
	if err != nil {
		return nil, fmt.Errorf("get main chain: %w", err)
	}
	indexChain, err := acct.MainChain().Index().Get()
	if err != nil {
		return nil, fmt.Errorf("get main-index chain: %w", err)
	}
	height := mainChain.Height()

	// Working page state, built up from the chain.
	page := &protocol.KeyPage{Url: pageUrl}

	for i := int64(0); i < height; i++ {
		// Block time for this main-chain entry.
		blockTime, err := blockTimeForMainEntry(indexChain, uint64(i))
		if err != nil {
			return nil, fmt.Errorf("block time for entry %d: %w", i, err)
		}
		if blockTime != nil && blockTime.After(t) {
			// We've passed the target time.
			break
		}

		// Read the entry's transaction.
		entry, err := mainChain.Entry(i)
		if err != nil {
			return nil, fmt.Errorf("read main entry %d: %w", i, err)
		}
		var hash [32]byte
		copy(hash[:], entry)

		txn, err := loadTransaction(batch, hash)
		if err != nil {
			return nil, fmt.Errorf("load tx %x: %w", hash[:8], err)
		}
		if txn == nil {
			// Not a TransactionMessage — skip (e.g., signature messages
			// can land on chains in some contexts; replay only cares
			// about state-mutating transactions).
			continue
		}

		if err := applyTransactionToPage(page, book, txn); err != nil {
			return nil, fmt.Errorf("apply tx %x at entry %d: %w", hash[:8], i, err)
		}
	}

	return page, nil
}

// blockTimeForMainEntry finds the block time for main-chain index `mainIndex`
// by searching the main-index chain.
func blockTimeForMainEntry(indexChain *database.Chain, mainIndex uint64) (*time.Time, error) {
	if indexChain.Height() == 0 {
		return nil, nil
	}
	// Find the smallest index entry with Source >= mainIndex.
	_, ie, err := indexing.SearchIndexChain(indexChain, uint64(indexChain.Height())-1, indexing.MatchAfter,
		indexing.SearchIndexChainBySource(mainIndex))
	if err != nil {
		// Chain may not have an index entry covering this main index
		// yet (live tip), or none of the entries match. Treat as
		// unknown rather than failing.
		return nil, nil
	}
	if ie == nil || ie.BlockTime == nil {
		return nil, nil
	}
	return ie.BlockTime, nil
}

// loadTransaction fetches the transaction for the given hash. Returns
// (nil, nil) if the message at this hash is not a TransactionMessage
// (e.g., signature messages stored on the same chain in some flows).
func loadTransaction(batch *database.Batch, hash [32]byte) (*protocol.Transaction, error) {
	var msg messaging.Message
	err := batch.Message(hash).Main().GetAs(&msg)
	if err != nil {
		return nil, err
	}
	tm, ok := msg.(*messaging.TransactionMessage)
	if !ok {
		return nil, nil
	}
	return tm.Transaction, nil
}

// applyTransactionToPage applies the effect of `txn` to the working
// page. Supports CreateKeyPage and UpdateKeyPage. Returns
// ErrUnsupportedTxType for other types.
func applyTransactionToPage(page *protocol.KeyPage, book *protocol.KeyBook, txn *protocol.Transaction) error {
	switch body := txn.Body.(type) {
	case *protocol.UpdateKeyPage:
		// Only apply operations targeting *this* page. Pages on the same
		// keybook share the chain only via creation transactions; an
		// UpdateKeyPage's principal is a specific page.
		if !txn.Header.Principal.Equal(page.Url) {
			// Ignore: an UpdateKeyPage on a different page wouldn't
			// land on this page's main chain in practice, but be
			// defensive.
			return nil
		}
		for _, op := range body.Operation {
			if err := chain.ApplyKeyPageOperation(page, book, op); err != nil {
				return fmt.Errorf("op %v: %w", op.Type(), err)
			}
		}
		page.Version++
		return nil

	case *protocol.CreateKeyPage:
		// CreateKeyPage adds a NEW page with initial entries. The page
		// starts at version 1 with the listed keys.
		for _, key := range body.Keys {
			entry := new(protocol.KeySpec)
			entry.PublicKeyHash = key.KeyHash
			entry.Delegate = key.Delegate
			page.AddKeySpec(entry)
		}
		if page.Version == 0 {
			page.Version = 1
		}
		return nil

	case *protocol.UpdateKey:
		// UpdateKey changes the key of the signer's signing keypage.
		// For replay purposes: if the principal is this page, apply
		// the new key.
		if !txn.Header.Principal.Equal(page.Url) {
			return nil
		}
		// UpdateKey carries NewKeyHash; replacing the *signing* key spec
		// is application-level (depends on which key signed). Here we
		// treat it as best-effort — return ErrUnsupportedTxType so the
		// caller surfaces it, since faithful replay would require
		// knowing which signer initiated the update.
		return fmt.Errorf("%w: UpdateKey at %v", ErrUnsupportedTxType, txn.Header.Principal)

	default:
		// Other transaction types affecting this page (CreateIdentity
		// when bootstrapping a brand-new identity with embedded keys,
		// for example) need their own handlers. Surface for visibility.
		return fmt.Errorf("%w: %T", ErrUnsupportedTxType, txn.Body)
	}
}
