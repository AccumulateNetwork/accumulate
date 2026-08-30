// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build debug

package database

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// receiptHashers returns the account's state hasher and chains hasher, in a form
// receipts can be taken from.
//
// The debug observer builds an annotated tree of hashables rather than a merkle
// hasher, so this flattens it. hashSet.Hash does exactly this — add each
// element's hash to a merkle hasher — so the hasher built here produces the same
// root, and therefore the same receipts, as the production build.
func (a *observedAccount) receiptHashers() (state, chains hash.Hasher, err error) {
	flatten := func(h *hashSet) hash.Hasher {
		var out hash.Hasher
		for _, e := range *h {
			out.AddHash2(e.Hash())
		}
		return out
	}

	s, err := a.hashState()
	if err != nil {
		return nil, nil, err
	}
	set, ok := s.(*hashSet)
	if !ok {
		return nil, nil, errors.InternalError.WithFormat("account state hash is %T, want *hashSet", s)
	}

	var chainSet hashSet
	err = a.hashChains(&chainSet)
	if err != nil {
		return nil, nil, err
	}
	return flatten(set), flatten(&chainSet), nil
}
