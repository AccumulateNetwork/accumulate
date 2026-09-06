// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// buildLedger creates a partition-ledger-shaped account with the given extra
// chains, and returns the database.
func buildLedger(t *testing.T, extraChains ...string) (*Database, *url.URL) {
	t.Helper()
	db := OpenInMemory(nil)
	ledgerUrl := protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger)

	batch := db.Begin(true)
	defer batch.Discard()
	acct := batch.Account(ledgerUrl)
	require.NoError(t, acct.Main().Put(&protocol.SystemLedger{Url: ledgerUrl, Index: 10}))
	for i := 0; i < 6; i++ {
		var h [32]byte
		h[0] = byte(i + 1)
		require.NoError(t, acct.BptChain().Inner().AddEntry(h[:], false))
		require.NoError(t, acct.RootChain().Inner().AddEntry(h[:], false))
		require.NoError(t, acct.MainChain().Inner().AddEntry(h[:], false))
		for _, name := range extraChains {
			c, err := acct.ChainByName(name)
			require.NoError(t, err, "chain %s", name)
			require.NoError(t, c.Inner().AddEntry(h[:], false))
		}
	}
	// More accounts, so the BPT is not trivial
	for i := 0; i < 8; i++ {
		u := protocol.PartitionUrl(fmt.Sprintf("P%d", i)).JoinPath(protocol.Ledger)
		require.NoError(t, batch.Account(u).Main().Put(&protocol.SystemLedger{Url: u, Index: uint64(i)}))
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	return db, ledgerUrl
}

func checkBinding(t *testing.T, db *Database, ledgerUrl *url.URL, index int64) *merkle.Receipt {
	t.Helper()
	batch := db.Begin(false)
	defer batch.Discard()
	acct := batch.Account(ledgerUrl)

	want, err := acct.BptChain().Entry(index)
	require.NoError(t, err)

	r, err := acct.ChainEntryReceipt("bpt", index)
	require.NoError(t, err)

	root, err := batch.BPT().GetRootHash()
	require.NoError(t, err)

	require.True(t, bytes.Equal(r.Start, want), "receipt does not start at the chain entry")
	require.True(t, bytes.Equal(r.Anchor, root[:]), "receipt does not terminate at the BPT root")
	require.True(t, r.Validate(nil), "receipt does not validate")
	return r
}

// TestChainEntryReceipt binds an entry of the ledger's bpt chain to the BPT
// root, which is what makes a historical root worth more than this node's word.
func TestChainEntryReceipt(t *testing.T) {
	db, ledgerUrl := buildLedger(t)
	for i := int64(0); i < 6; i++ {
		checkBinding(t, db, ledgerUrl, i)
	}
}

// TestChainEntryReceipt_ChainIndexIsDerived proves the position of the bpt chain
// among the account's chains is computed, not assumed.
//
// hashChains walks the chains in name order, so a chain that sorts before "bpt"
// shifts its index. Hard-coding the index — which is 0 on today's ledger — would
// pass every test until someone added such a chain, and would then produce a
// receipt that fails to validate for reasons nowhere near the cause.
func TestChainEntryReceipt_ChainIndexIsDerived(t *testing.T) {
	// "anchor-sequence" is a real ledger chain and sorts before "bpt"
	db, ledgerUrl := buildLedger(t, "anchor-sequence")

	batch := db.Begin(false)
	names := func() []string {
		metas, err := batch.Account(ledgerUrl).Chains().Get()
		require.NoError(t, err)
		var out []string
		for _, m := range metas {
			out = append(out, m.Name)
		}
		return out
	}()
	batch.Discard()
	t.Logf("chains in hash order: %v", names)
	require.NotEqual(t, "bpt", names[0], "the test did not actually displace bpt")

	for i := int64(0); i < 6; i++ {
		checkBinding(t, db, ledgerUrl, i)
	}
}

// TestChainEntryReceipt_RefusesUnknownEntry proves an out-of-range entry is
// refused rather than producing a receipt for the wrong entry.
func TestChainEntryReceipt_RefusesUnknownEntry(t *testing.T) {
	db, ledgerUrl := buildLedger(t)
	batch := db.Begin(false)
	defer batch.Discard()

	_, err := batch.Account(ledgerUrl).ChainEntryReceipt("bpt", 99)
	require.Error(t, err)
	require.Contains(t, err.Error(), "has no entry 99")
}
