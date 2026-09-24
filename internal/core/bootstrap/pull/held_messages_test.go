// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// messagelessTail is the store #4421 left behind: the pool taken whole once,
// with the message behind every entry, and then pulled state-only after the
// peer moved on, so the entries past the first pull are held with no message
// behind them. It returns the peer, the pool, and every entry the peer holds.
func messagelessTail(t *testing.T) (peer, node *database.Database, u *url.URL, entries [][32]byte) {
	t.Helper()
	peer, u, entries = spineWithMessages(t)
	node = newObservedDB(t)

	b := node.Begin(true)
	require.NoError(t, Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: ModeFullSpine}))
	require.NoError(t, b.Commit())

	func() {
		b := peer.Begin(true)
		defer b.Discard()
		for i := 0; i < 3; i++ {
			entries = append(entries, addTransactionEntry(t, b, u, 100+i, 0x42))
		}
		require.NoError(t, b.UpdateBPT())
		require.NoError(t, b.Commit())
	}()

	b = node.Begin(true)
	require.NoError(t, Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: ModeStateOnly}))
	require.NoError(t, b.Commit())

	missing := messagesMissing(t, node, u)
	require.NotEmpty(t, missing, "precondition: the state-only pull left entries with no message behind them")
	return peer, node, u, entries
}

// messagesMissing is the position of every entry of u's main chain the store
// holds with no message behind it.
func messagesMissing(t *testing.T, db *database.Database, u *url.URL) []int64 {
	t.Helper()
	b := db.Begin(false)
	defer b.Discard()
	c := b.Account(u).MainChain()
	head, err := c.Head().Get()
	require.NoError(t, err)
	var missing []int64
	for i := int64(0); i < head.Count; i++ {
		h, err := c.Entry(i)
		require.NoError(t, err)
		if _, err := b.Message2(h).Main().Get(); err != nil {
			missing = append(missing, i)
		}
	}
	return missing
}

// TestFullSpine_AHeldEntryWithNoMessageHasItFetched — #4421. A node that
// joined before the fix holds pool entries with no message behind them, and
// a full pull resumes from the local head, so it brings nothing for them: the
// node's seed then fails at every process start while those entries are the
// newest. A pull that checks what it holds fetches the message behind each
// held entry that has none, proven against its entry like any other.
func TestFullSpine_AHeldEntryWithNoMessageHasItFetched(t *testing.T) {
	for _, check := range []bool{false, true} {
		peer, node, u, _ := messagelessTail(t)
		b := node.Begin(true)
		p, _, err := FetchFrom(context.Background(), []Source{&dbSource{db: peer}}, b, u, Options{Mode: ModeFullSpine, CheckHeld: check})
		require.NoError(t, err)
		require.NoError(t, p.Keep())
		require.NoError(t, b.Commit())

		missing := messagesMissing(t, node, u)
		if check {
			require.Empty(t, missing, "entries are still held with no message behind them")
		} else {
			require.NotEmpty(t, missing, "precondition: a pull that does not check what it holds resumes from the local head and brings nothing for them")
		}
	}
}

// TestFullSpine_AHeldEntryWithNoMessageAtThePeerIsRefused — the message is
// proven by its entry like any other, and a peer that does not hold it has not
// served the chain: the fetch fails, nothing is kept, and the next peer is
// asked.
func TestFullSpine_AHeldEntryWithNoMessageAtThePeerIsRefused(t *testing.T) {
	peer, node, u, _ := messagelessTail(t)

	// A peer that itself holds the same entries with no message: the node's
	// own store, served back to it.
	b := node.Begin(true)
	defer b.Discard()
	_, _, err := FetchFrom(context.Background(), []Source{&dbSource{db: node}}, b, u, Options{Mode: ModeFullSpine, CheckHeld: true})
	require.Error(t, err, "a peer that holds no message behind an entry was believed")

	p, i, err := FetchFrom(context.Background(), []Source{&dbSource{db: node}, &dbSource{db: peer}}, b, u, Options{Mode: ModeFullSpine, CheckHeld: true})
	require.NoError(t, err)
	require.Equal(t, 1, i, "the next peer answered")
	require.NoError(t, p.Keep())
	require.NoError(t, b.Commit())
	require.Empty(t, messagesMissing(t, node, u))
}
