// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package sidecar

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	ledger  = protocol.PartitionUrl("Cyclops").JoinPath(protocol.Ledger)
	anchors = protocol.PartitionUrl("Cyclops").JoinPath(protocol.AnchorPool)
	oldLedg = protocol.PartitionUrl("Apollo").JoinPath(protocol.Ledger)
	acct    = url.MustParse("acc://old.acme/data")
	hash    = [32]byte{1, 2, 3}
)

// #4560: what the API may read from the sidecar, and what it keeps.
func TestPolicy(t *testing.T) {
	p := PolicyFor("Cyclops")
	cases := []struct {
		name        string
		key         *record.Key
		fall, keep  bool
	}{
		{"the BPT", record.NewKey("BPT", "Root"), false, false},
		{"own ledger root chain", record.NewKey("Account", ledger, "RootChain", "Element", uint64(1)), false, false},
		{"own anchor pool", record.NewKey("Account", anchors, "AnchorChain", "Apollo", "Root", "Head"), false, false},
		{"an old partition's ledger", record.NewKey("Account", oldLedg, "RootChain", "Element", uint64(1)), true, true},
		{"a message", record.NewKey("Message", hash, "Main"), true, true},
		{"a transaction's chains", record.NewKey("Transaction", hash, "Chains"), true, true},
		{"a chain element", record.NewKey("Account", acct, "MainChain", "Element", uint64(1)), true, true},
		{"a mark point", record.NewKey("Account", acct, "MainChain", "States", uint64(255)), true, true},
		{"an element index", record.NewKey("Account", acct, "MainChain", "ElementIndex", hash[:]), true, true},
		{"an index chain element", record.NewKey("Account", acct, "MainChain", "Index", "Element", uint64(1)), true, true},
		{"a chain head", record.NewKey("Account", acct, "MainChain", "Head"), true, false},
		{"a data index entry", record.NewKey("Account", acct, "Data", "Entry", 3), true, false},
		{"a pending set", record.NewKey("Account", acct, "Pending"), true, false},
		{"account state", record.NewKey("Account", acct, "Main"), true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.fall, p.FallThrough(c.key), "fall through")
			require.Equal(t, c.keep, p.Keep(c.key), "keep")
		})
	}
}

// A record the policy refuses is not found even though the sidecar holds
// it; a record it keeps lands in the live store shortly after the read.
func TestPolicyApplied(t *testing.T) {
	side, live := memory.New(nil), memory.New(nil)
	refused := record.NewKey("Account", ledger, "RootChain", "Element", uint64(1))
	kept := record.NewKey("Message", hash, "Main")
	served := record.NewKey("Account", acct, "Main")
	put(t, side, refused, "consensus")
	put(t, side, kept, "history")
	put(t, side, served, "state")

	db := Over(live, serve(t, side), PolicyFor("Cyclops"))
	defer db.Close()

	_, err := get(db, refused)
	require.ErrorIs(t, err, errors.NotFound, "the node's own consensus record came from the sidecar")

	v, err := get(db, kept)
	require.NoError(t, err)
	require.Equal(t, "history", string(v))
	v, err = get(db, served)
	require.NoError(t, err)
	require.Equal(t, "state", string(v))

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := get(live, kept); err == nil {
			break
		}
		require.True(t, time.Now().Before(deadline), "the message was not written back")
		time.Sleep(time.Millisecond)
	}
	_, err = get(live, served)
	require.ErrorIs(t, err, errors.NotFound, "current state was written back")
}

// #4567: the era answers the overwritten store's value for a key the live
// database changed, the sidecar's for one it dropped, and nothing else.
func TestEra(t *testing.T) {
	changed := record.NewKey("Account", ledger, "RootChain", "Head")
	dropped := record.NewKey("Account", oldLedg, "RootChain", "Head")
	neither := record.NewKey("Account", acct, "Main")

	dir := t.TempDir()
	meta, _ := json.Marshal(map[string]any{"archives": []string{"directory/A", "directory/B"}})
	require.NoError(t, os.WriteFile(filepath.Join(dir, "overwritten.json"), meta, 0600))
	raw, err := leveldb.OpenFile(filepath.Join(dir, "overwritten.db"), nil)
	require.NoError(t, err)
	h := changed.Hash()
	require.NoError(t, raw.Put(append(h[:], 1), []byte("archive B"), nil)) // only the second archive holds it
	require.NoError(t, raw.Close())

	side := memory.New(nil)
	put(t, side, dropped, "from the sidecar")
	era, err := OpenEra([]string{dir}, side)
	require.NoError(t, err)
	defer era.Close()

	srv := httptest.NewServer(HandlerWithEra(side, era))
	t.Cleanup(srv.Close)
	c := (&Client{URL: srv.URL}).AsEra()

	v, err := get(c, changed)
	require.NoError(t, err)
	require.Equal(t, "archive B", string(v))
	v, err = get(c, dropped)
	require.NoError(t, err)
	require.Equal(t, "from the sidecar", string(v))
	_, err = get(c, neither)
	require.ErrorIs(t, err, errors.NotFound)

	// The plain endpoint does not see the era
	_, err = get(&Client{URL: srv.URL}, changed)
	require.ErrorIs(t, err, errors.NotFound)

	// A server without an era answers the era with not found
	srv2 := httptest.NewServer(Handler(side))
	t.Cleanup(srv2.Close)
	_, err = get((&Client{URL: srv2.URL}).AsEra(), dropped)
	require.ErrorIs(t, err, errors.NotFound)
}
