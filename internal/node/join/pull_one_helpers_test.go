// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// noValidators is the validators of a test's peers that reach none: the
// join's anchor source finds no validator and takes no anchor.
type noValidators struct{}

func (noValidators) ValidatorsOf(context.Context, *url.URL) ([]anchorsrc.Validator, error) {
	return nil, errors.NotReady.With("these test peers reach no validator")
}

// oneSource hands the fetch loop a single peer for every account.
type oneSource struct {
	noValidators
	part *url.URL
	src  pull.Source
}

func (o *oneSource) For(context.Context, *url.URL) ([]pull.Source, *url.URL, error) {
	return []pull.Source{o.src}, o.part, nil
}
func (o *oneSource) Querier(*url.URL) api.Querier { return nil }

func writeAccount(t *testing.T, db *database.Database, u *url.URL, entries int) {
	t.Helper()
	b := db.Begin(true)
	defer b.Discard()
	require.NoError(t, b.Account(u).Main().Put(&protocol.DataAccount{Url: u}))
	c, err := b.Account(u).ChainByName("main")
	require.NoError(t, err)
	_, err = c.Get()
	require.NoError(t, err)
	for i := 0; i < entries; i++ {
		e := make([]byte, 32)
		e[0], e[1] = byte(i), byte(i>>8)
		require.NoError(t, c.Inner().AddEntry(e, false))
	}
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
}

// servedAt is the production querier with a receipt stapled on that ends at
// the root the test names for the account, zero for an account it names no
// root for. The peer here is a store with no block index, so it cannot build a
// proof to its BPT root; the receipt is the account's own state-tree receipt,
// which ends at the account's leaf, with the named root written over its end.
// The join verifies nothing against a root (its one proof is the match), so
// what matters to it is only that an answer carries a receipt: the answer with
// one is the one that carries the rest of the leaf and whose NotFound means no
// leaf.
type servedAt struct {
	pull.Source
	db    *database.Database
	roots map[string][32]byte
}

func (s servedAt) QueryAccount(ctx context.Context, u *url.URL, _ *api.DefaultQuery) (*api.AccountRecord, error) {
	r, err := s.Source.QueryAccount(ctx, u, nil)
	if err != nil {
		return nil, err
	}
	r.Receipt = &api.Receipt{Partition: "BVN0", LocalBlock: 7}
	root, ok := s.roots[strings.ToLower(u.String())]
	if !ok {
		return r, nil // Served at no root
	}
	batch := s.db.Begin(false)
	defer batch.Discard()
	leaf, err := batch.Account(u).StateTreeReceipt()
	if err != nil {
		return nil, err
	}
	if leaf == nil {
		return nil, errors.InternalError.WithFormat("%v has no main state receipt under this observer", u)
	}
	r.Receipt.Receipt = *leaf
	r.Receipt.Anchor = root[:]
	return r, nil
}

func urlsOf(us []*url.URL) []string {
	var out []string
	for _, u := range us {
		out = append(out, u.String())
	}
	return out
}
