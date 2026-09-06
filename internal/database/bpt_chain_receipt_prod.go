// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !debug

package database

import "gitlab.com/accumulatenetwork/accumulate/internal/core/hash"

// receiptHashers returns the account's state hasher and chains hasher, in a form
// receipts can be taken from.
//
// The production observer builds merkle hashers directly, so this is a
// pass-through. The debug observer builds an annotated tree instead and needs a
// conversion — see the debug variant. Receipts must be identical either way,
// which a test asserts by validating the composed receipt against the BPT.
func (a *observedAccount) receiptHashers() (state, chains hash.Hasher, err error) {
	state, err = a.hashState()
	if err != nil {
		return nil, nil, err
	}
	chains, err = a.hashChains()
	if err != nil {
		return nil, nil, err
	}
	return state, chains, nil
}
