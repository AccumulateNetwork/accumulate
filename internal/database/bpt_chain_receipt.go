// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

// hashStateChainsIndex is the position of the chains hash in the merkle hasher
// [observedAccount.hashState] builds: main state, secondary state, chains,
// pending. Reordering that list would move it, which is why
// [Account.ChainEntryReceipt] is covered by a test that validates the composed
// receipt against the BPT rather than asserting the index.
const hashStateChainsIndex = 2

// ChainEntryReceipt proves that an entry of one of the account's chains is
// committed into the BPT root.
//
// An account's BPT entry is a merkle hash over its main state, its secondary
// state, the anchor of every one of its chains, and its pending transactions
// (see [observedAccount.hashState] and [observedAccount.hashChains]). So a chain
// entry is bound to the BPT root by four steps, and the BPT root is what the
// network anchors and signs:
//
//	entry            -> chain anchor         chain receipt
//	chain anchor     -> chains hash          hasher receipt over hashChains
//	chains hash      -> account BPT entry    hasher receipt over hashState
//	account BPT entry-> BPT root             BPT membership receipt
//
// This is the same composition [Account.StateReceipt] uses to prove main state;
// it proves element 0 of hashState, this proves element 2 and one level below.
//
// AIP-58 uses it on the partition ledger's `bpt` chain, which records the BPT
// root of every state-changing block. That chain is NOT itself anchored into the
// root chain — its entry is written after enumerateModifiedChains has run, so it
// never reaches addChainAnchor — but it does not need to be: this receipt binds
// its entries to the BPT root by the route above.
func (a *Account) ChainEntryReceipt(name string, index int64) (*merkle.Receipt, error) {
	if a.IsDirty() {
		return nil, errors.InternalError.With("cannot generate a receipt when there are uncommitted changes")
	}

	c, err := a.ChainByName(name)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get chain %s: %w", name, err)
	}
	chain, err := c.Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load chain %s: %w", name, err)
	}
	if index < 0 || index >= chain.Height() {
		return nil, errors.NotFound.WithFormat("chain %s has no entry %d", name, index)
	}

	// Where does this chain sit among the account's chains? hashChains walks
	// them in the order Chains() yields, which is sorted by name — derive it,
	// never assume a position.
	metas, err := a.Chains().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load chain list: %w", err)
	}
	chainIndex := -1
	for i, m := range metas {
		if m.Name == name {
			chainIndex = i
			break
		}
	}
	if chainIndex < 0 {
		return nil, errors.InternalError.WithFormat("chain %s is not in %v's chain list", name, a.Url())
	}

	obs := observedAccount{a, a.parent}
	stateHasher, chainsHasher, err := obs.receiptHashers()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("hash account: %w", err)
	}

	rEntry, err := chain.Receipt(index, chain.Height()-1)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("receipt for entry %d of chain %s: %w", index, name, err)
	}
	rChains := chainsHasher.Receipt(chainIndex, len(chainsHasher)-1)
	rState := stateHasher.Receipt(hashStateChainsIndex, len(stateHasher)-1)
	rBPT, err := a.BptReceipt()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Each step must start where the previous ended. Checking rather than
	// trusting: a silent mismatch here would produce a receipt that fails to
	// validate somewhere far away from the cause.
	for _, j := range []struct {
		name string
		a, b *merkle.Receipt
	}{
		{"chain anchor to chains hash", rEntry, rChains},
		{"chains hash to account entry", rChains, rState},
		{"account entry to BPT root", rState, rBPT},
	} {
		if !bytes.Equal(j.a.Anchor, j.b.Start) {
			return nil, errors.InternalError.WithFormat(
				"%s: %x does not match %x", j.name, j.a.Anchor, j.b.Start)
		}
	}

	r, err := rEntry.Combine(rChains)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("combine chain and chains receipts: %w", err)
	}
	r, err = r.Combine(rState)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("combine chains and state receipts: %w", err)
	}
	r, err = r.Combine(rBPT)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("combine state and BPT receipts: %w", err)
	}
	return r, nil
}
