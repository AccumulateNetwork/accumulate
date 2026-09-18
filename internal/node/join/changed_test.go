// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func strs(urls []*url.URL) []string {
	out := make([]string, len(urls))
	for i, u := range urls {
		out[i] = u.String()
	}
	return out
}

// TestChangedAccounts_CarriesTheSystemAccountsTheRecordCannot — the two
// accounts that made the root unreachable (#4306).
//
// <partition>/synthetic is skipped by enumerateModifiedChains outright, and
// <partition>/ledger's own chains are anchored after the block's entry list is
// built, so neither is reliably in a block's record. Both change their leaf on
// every non-empty block: the ledger carries the scheduled-events BPT and the
// block index, the synthetic account the delivery queues. A set without them
// chases a root it can never reach, however long the node waits.
func TestChangedAccounts_CarriesTheSystemAccountsTheRecordCannot(t *testing.T) {
	part := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")

	got := strs(ChangedAccounts(part, []*protocol.BlockEntry{
		{Account: alice, Chain: "main", Index: 3},
	}))

	require.Contains(t, got, alice.String())
	require.Contains(t, got, part.JoinPath(protocol.Ledger).String(),
		"the partition's ledger changes every block and no block's record names it")
	require.Contains(t, got, part.JoinPath(protocol.Synthetic).String(),
		"the partition's synthetic account changes every block and enumerateModifiedChains skips it")
}

// TestChangedAccounts_DropsWhatCannotBeRouted — acc://unknown is what
// normalize.go gives a SignatureMessage carrying only a hash. Nothing can route
// it, so it is refused forever, and a refusal that never clears pins the retry
// set and disables the page-diff backstop for the life of the process (#4306).
func TestChangedAccounts_DropsWhatCannotBeRouted(t *testing.T) {
	part := protocol.PartitionUrl("BVN0")

	got := strs(ChangedAccounts(part, []*protocol.BlockEntry{
		{Account: protocol.UnknownUrl(), Chain: "main"},
		{Account: protocol.UnknownUrl().WithTxID([32]byte{1}).Account(), Chain: "signature"},
		{Account: nil},
		{Account: protocol.AccountUrl("bob"), Chain: "main"},
	}))

	require.NotContains(t, got, protocol.UnknownUrl().String())
	require.Contains(t, got, protocol.AccountUrl("bob").String())
	for _, u := range got {
		require.NotContains(t, u, "unknown", "an unroutable name survived: %s", u)
	}
}

// TestChangedAccounts_IsASetInOrder — a name too many costs one pull; a name
// twice costs one too, and the order has to be stable so a round's log means
// the same thing twice.
func TestChangedAccounts_IsASetInOrder(t *testing.T) {
	part := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")

	got := strs(ChangedAccounts(part, []*protocol.BlockEntry{
		{Account: alice, Chain: "main", Index: 1},
		{Account: alice, Chain: "signature", Index: 2},
		{Account: part.JoinPath(protocol.Ledger), Chain: "root", Index: 9},
	}))

	require.Equal(t, []string{
		alice.String(),
		part.JoinPath(protocol.Ledger).String(),
		part.JoinPath(protocol.Synthetic).String(),
	}, got)
}

// TestRoutable — the rule the pull applies before it asks anybody.
func TestRoutable(t *testing.T) {
	require.False(t, Routable(nil))
	require.False(t, Routable(protocol.UnknownUrl()))
	require.False(t, Routable(&url.URL{}))
	require.True(t, Routable(protocol.AccountUrl("alice")))
	require.True(t, Routable(protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger)))
}
