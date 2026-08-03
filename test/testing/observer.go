// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testing

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
)

// NullObserver returns a null hash.
type NullObserver struct{}

func (NullObserver) DidChangeAccount(batch *database.Batch, account *database.Account) (hash.Hasher, error) {
	// Don't use a null hash because that will trick the BPT into thinking it
	// doesn't need to do anything
	return hash.Hasher{account.Url().AccountID()}, nil
}

// VisitorObserver builds a list of hashes from a snapshot and then returns
// those exact same hashes.
type VisitorObserver map[[32]byte][32]byte

func (h VisitorObserver) VisitAccount(a *snapshot.Account, i int) error {
	if a != nil {
		h[storage.MakeKey("Account", a.Url)] = a.Hash
	}
	return nil
}

func (h VisitorObserver) DidChangeAccount(_ *database.Batch, a *database.Account) (hash.Hasher, error) {
	b := h[storage.MakeKey("Account", a.Url())]
	return hash.Hasher{b[:]}, nil
}
