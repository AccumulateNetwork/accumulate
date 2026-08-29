// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type fixedQuerier struct{ account protocol.Account }

func (q fixedQuerier) Query(context.Context, *url.URL, api.Query) (api.Record, error) {
	return &api.AccountRecord{Account: q.account}, nil
}

// awaitBalance gated every ADI's creation on "the token account holds
// tokens", and asserted an interface no account type implements — so it
// timed out on every identity the generator ever tried to grow, on every
// run since 807339184, and the transaction mix silently collapsed to
// lite-account actions only. It must return as soon as either kind of
// token account shows a balance.
func TestAwaitBalance_SeesAFundedAccount(t *testing.T) {
	for name, acct := range map[string]protocol.Account{
		"adi token account":  &protocol.TokenAccount{Balance: *big.NewInt(1)},
		"lite token account": &protocol.LiteTokenAccount{Balance: *big.NewInt(1)},
	} {
		t.Run(name, func(t *testing.T) {
			e := &env{Q: api.Querier2{Querier: fixedQuerier{acct}}}
			err := e.awaitBalance(context.Background(), url.MustParse("alice.acme/tokens"), 3*time.Second)
			require.NoError(t, err, "a funded account must satisfy the wait")
		})
	}
}

func TestAwaitBalance_WaitsForAnEmptyAccount(t *testing.T) {
	e := &env{Q: api.Querier2{Querier: fixedQuerier{&protocol.TokenAccount{}}}}
	err := e.awaitBalance(context.Background(), url.MustParse("alice.acme/tokens"), 100*time.Millisecond)
	require.Error(t, err)
}

// A page with no keys must be skipped, not fed to rand.Intn(0).
func TestSetThreshold_SkipsAnEmptyPage(t *testing.T) {
	u := newUniverse(nil)
	p := &keyPage{}
	require.Equal(t, 0, u.keyCount(p))
	require.NotPanics(t, func() {
		if n := u.keyCount(p); n > 0 {
			_ = u.intn(n)
		}
	})
}
