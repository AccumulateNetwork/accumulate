// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// TestClassification states the classification as a table of the keys
// the model actually produces.  It is the readable form of the rules in
// route.go: the cases that look alike and disagree are next to each
// other, and the reason each one is where it is has to survive being
// written down.
//
// What this cannot check is drift -- a record renamed in model.yml
// leaves these keys valid and the rule dead.  TestSimulatorRouting is
// what catches that, by running real traffic and asking the store.
func TestClassification(t *testing.T) {
	alice := url.MustParse("alice.acme")
	var hash [32]byte
	hash[0] = 1

	cases := []struct {
		writeOnce bool
		why       string
		key       *record.Key
	}{
		// A message and a transaction are named by the hash of their
		// own content, so the content cannot change under the name
		{true, "a message is its hash", record.NewKey("Message", hash, "Main")},
		{true, "a transaction is its hash", record.NewKey("Transaction", hash, "Main")},

		// ... but the same hash also names what has happened to that
		// transaction since, and that does change
		{false, "status of the same hash", record.NewKey("Transaction", hash, "Status")},
		{false, "produced set grows", record.NewKey("Transaction", hash, "Produced")},
		{false, "signature set", record.NewKey("Transaction", hash, "Signatures", alice)},
		{false, "chain index set", record.NewKey("Transaction", hash, "Chains")},
		{false, "cause set", record.NewKey("Message", hash, "Cause")},
		{false, "signers set", record.NewKey("Message", hash, "Signers")},

		// An account is named by its URL, and its state is what
		// changes
		{false, "account state", record.NewKey("Account", alice, "Main")},
		{false, "pending set", record.NewKey("Account", alice, "Pending")},
		{false, "directory set", record.NewKey("Account", alice, "Directory")},
		{false, "chain metadata set", record.NewKey("Account", alice, "Chains")},
		{true, "the URL it is keyed by", record.NewKey("Account", alice, "Url")},

		// A merkle chain is a log: entries and mark points are facts
		// about a position, the head is where the log currently ends
		{true, "chain entry", record.NewKey("Account", alice, "MainChain", "Element", uint64(5))},
		{true, "where an entry landed", record.NewKey("Account", alice, "MainChain", "ElementIndex", hash)},
		{true, "mark point", record.NewKey("Account", alice, "MainChain", "States", uint64(64))},
		{false, "the end of the log", record.NewKey("Account", alice, "MainChain", "Head")},

		// The same rules have to reach every chain, including the ones
		// with a parameter of their own
		{true, "signature chain entry", record.NewKey("Account", alice, "SignatureChain", "Element", uint64(1))},
		{true, "per-partition chain entry", record.NewKey("Account", alice, "SyntheticSequenceChain", "BVN1", "Element", uint64(1))},
		{true, "anchor chain entry", record.NewKey("Account", alice, "AnchorChain", "BVN1", "Root", "Element", uint64(1))},
		{false, "anchor chain head", record.NewKey("Account", alice, "AnchorChain", "BVN1", "Root", "Head")},

		// A counted collection is an element per index plus a count,
		// and the count changes with every element
		{true, "data entry", record.NewKey("Account", alice, "Data", "Entry", uint64(3))},
		{false, "the count of them", record.NewKey("Account", alice, "Data", "Entry")},
		// ... but which transaction wrote an entry is rewritten when a
		// second transaction writes the same entry (#4174)
		{false, "which txn wrote an entry", record.NewKey("Account", alice, "Data", "Transaction", hash)},

		// The BPT is a hash tree over mutable state, so its nodes are
		// mutable however they are keyed
		{false, "BPT node", record.NewKey("BPT", hash)},
		{false, "BPT parameters", record.NewKey("BPT", "Root")},

		// Scheduled events, block ledgers and index logs all change
		{false, "scheduled blocks", record.NewKey("Account", alice, "Events", "Minor", "Blocks")},
		{false, "held votes", record.NewKey("Account", alice, "Events", "Minor", "Votes", uint64(7))},
		{false, "expired backlog", record.NewKey("Account", alice, "Events", "Backlog", "Expired")},
		{false, "block ledger head", record.NewKey("Account", alice, "BlockLedger", "Head")},
		{false, "block ledger block", record.NewKey("Account", alice, "BlockLedger", "Block", uint64(0), uint64(3))},

		// Which synthetic index entry covers a block does not change
		{true, "block to synth index", record.NewKey("SystemData", "BVN1", "SyntheticIndexIndex", uint64(9))},

		// The BSN's block summary is named by its hash and recorded
		// once; its signature set grows
		{true, "a summary is its hash", record.NewKey("Summary", hash, "Main")},
		{false, "summary signatures", record.NewKey("Summary", hash, "Signatures")},

		// A record the classification has never heard of is mutable,
		// which is the direction that fails quietly
		{false, "unknown record", record.NewKey("Account", alice, "SomethingNew")},
	}

	for _, c := range cases {
		t.Run(c.key.String(), func(t *testing.T) {
			require.Equal(t, c.writeOnce, isWriteOnce(c.key), c.why)
		})
	}
}

// TestTail checks the walk that finds the record name, because every
// rule in route.go is stated in terms of what it returns.
func TestTail(t *testing.T) {
	alice := url.MustParse("alice.acme")
	var hash [32]byte

	last, prev, trailing := tail(record.NewKey("Account", alice, "MainChain", "Element", uint64(5)))
	require.Equal(t, "Element", last)
	require.Equal(t, "MainChain", prev)
	require.Equal(t, 1, trailing)

	last, prev, trailing = tail(record.NewKey("Message", hash, "Main"))
	require.Equal(t, "Main", last)
	require.Equal(t, "Message", prev)
	require.Equal(t, 0, trailing)

	// A parameter that happens to be a string is not a record name,
	// but it is indistinguishable from one, so the rules that look at
	// prev must not be the rules that fire here
	last, prev, trailing = tail(record.NewKey("Account", alice, "SyntheticSequenceChain", "BVN1", "Element", uint64(1)))
	require.Equal(t, "Element", last)
	require.Equal(t, "BVN1", prev)
	require.Equal(t, 1, trailing)

	// Nothing to say about a key with no names, and nothing to crash on
	last, prev, trailing = tail(record.NewKey(hash))
	require.Equal(t, "", last)
	require.Equal(t, "", prev)
	require.Equal(t, 1, trailing)

	last, _, _ = tail(nil)
	require.Equal(t, "", last)
}
