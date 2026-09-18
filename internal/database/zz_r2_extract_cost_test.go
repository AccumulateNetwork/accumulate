// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestR2_GenesisExtractCost measures the two-pass genesis.Extract, which the
// branch carries forward as unmeasured. Both passes now run the rebuild over
// every account in the BPT. REVIEW_LONG accounts have chains above the mark
// frequency (the rebuild stops at entry 0); REVIEW_SHORT accounts have chains
// below it (the rebuild indexes them on pass 1 and re-reads them on pass 2).
func TestR2_GenesisExtractCost(t *testing.T) {
	if os.Getenv("REVIEW_LONG") == "" {
		t.Skip("set REVIEW_LONG")
	}
	nLong, err := strconv.Atoi(os.Getenv("REVIEW_LONG"))
	require.NoError(t, err)
	nShort, err := strconv.Atoi(os.Getenv("REVIEW_SHORT"))
	require.NoError(t, err)

	built := database.OpenInMemory(nil)
	batch := built.Begin(true)
	mk := func(name string, a, n int) {
		u := protocol.AccountUrl(fmt.Sprintf("%s%d.acme", name, a), "tokens")
		acct := new(protocol.TokenAccount)
		acct.Url = u
		acct.TokenUrl = protocol.AcmeUrl()
		acct.Balance = *big.NewInt(1000)
		require.NoError(t, batch.Account(u).Main().Put(acct))
		c, err := batch.Account(u).MainChain().Get()
		require.NoError(t, err)
		var buf [24]byte
		copy(buf[:8], name)
		for i := 0; i < n; i++ {
			binary.BigEndian.PutUint64(buf[8:16], uint64(a))
			binary.BigEndian.PutUint64(buf[16:], uint64(i))
			h := sha256.Sum256(buf[:])
			require.NoError(t, c.AddEntry(h[:], false))
		}
	}
	for a := 0; a < nLong; a++ {
		mk("long", a, 300)
	}
	for a := 0; a < nShort; a++ {
		mk("short", a, 20)
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	batch.Discard()

	snap := collectSnapshot(t, built)
	t.Logf("snapshot: %d long (300 entries) + %d short (20 entries) accounts = %d entries, %.1f MiB",
		nLong, nShort, nLong*300+nShort*20, float64(len(snap))/(1<<20))

	db := database.OpenInMemory(nil)
	start := time.Now()
	accounts, err := genesis.Extract(db, ioutil.NewBuffer(snap), func(*url.URL) bool { return true })
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.Len(t, accounts, nLong+nShort)
	t.Logf("GENESIS.EXTRACT (two passes) over %d accounts: %v", nLong+nShort, elapsed)
}
