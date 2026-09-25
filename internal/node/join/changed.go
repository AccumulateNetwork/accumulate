// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"sort"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// maxBlockEntries is how many entries of one block's ledger are read per call.
const maxBlockEntries = 512

// ChangedAccounts is the set of accounts a block changed, from the block
// ledger's record of that block (executor.md, "Sync", step 3, and "The block
// ledger").
//
// Every block records (account, chain, index) for every chain its execution
// changed; the record is a chain on the partition's ledger account and the
// chain's anchor is part of that account's hash, so the state root commits to
// what each block changed. That is the set to pull, and a block's envelopes
// are not: envelopes name principals, signers and anchor pools, which is
// neither everything a block writes nor only things that can be routed.
//
// Two accounts are added because the record cannot carry them, and both change
// on every non-empty block:
//
//   - <partition>/synthetic. enumerateModifiedChains skips it outright
//     ("anchoring the synthetic transaction ledger causes sadness and
//     despair", block_end.go), so no block's record ever names it, yet its
//     delivery queues are hashed into its leaf.
//   - <partition>/ledger. Its own chains -- the root chain, the BPT chain, the
//     block-ledger chain -- are appended to after the entry list is built, so
//     the record names the ledger only incidentally, and its leaf moves every
//     block regardless.
//
// A tree built without them chases a root it can never reach, which is what
// #4306 is.
//
// Names that cannot be routed are dropped. acc://unknown is the one that
// occurs: normalize.go gives a SignatureMessage carrying only a hash a TxID
// whose account is protocol.UnknownUrl(), nothing can route it, and a name
// that can never be satisfied would be asked for again every round for the
// life of the process.
func ChangedAccounts(partition *url.URL, entries []*protocol.BlockEntry) []*url.URL {
	seen := map[string]*url.URL{}
	add := func(u *url.URL) {
		if !Routable(u) {
			return
		}
		k := strings.ToLower(u.String())
		if _, ok := seen[k]; !ok {
			seen[k] = u
		}
	}

	for _, e := range entries {
		if e == nil {
			continue
		}
		add(e.Account)
	}

	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*url.URL, 0, len(keys))
	for _, k := range keys {
		out = append(out, seen[k])
	}
	return out
}

// Routable reports whether a name is one a pull can ask anybody for. It is a
// property of the URL and not of a routing table: a name with no authority, or
// the placeholder acc://unknown, names no account anywhere.
func Routable(u *url.URL) bool {
	if u == nil || u.Authority == "" {
		return false
	}
	unknown := protocol.UnknownUrl()
	return !u.Equal(unknown) && !u.Identity().Equal(unknown)
}

// blockLedgerOf reads the accounts one block changed, from a peer, through the
// block query -- which answers from the block ledger
// (internal/api/v3/querier.go, queryMinorBlock) and, with EntryRange.Expand
// false, answers with the (account, chain, index) triples alone. NotFound is a
// block with no record: an empty block writes nothing, not even its index.
func blockLedgerOf(ctx context.Context, q api.Querier2, partition *url.URL, block uint64) ([]*protocol.BlockEntry, error) {
	var out []*protocol.BlockEntry
	var start uint64
	for {
		count, expand := uint64(maxBlockEntries), false
		rec, err := q.QueryMinorBlock(ctx, partition, &api.BlockQuery{
			Minor: &block,
			EntryRange: &api.RangeOptions{
				Start:  start,
				Count:  &count,
				Expand: &expand,
			},
		})
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		if rec == nil || rec.Entries == nil || len(rec.Entries.Records) == 0 {
			return out, nil
		}
		for _, e := range rec.Entries.Records {
			if e == nil || e.Account == nil {
				continue
			}
			out = append(out, &protocol.BlockEntry{Account: e.Account, Chain: e.Name, Index: e.Index})
		}
		start += uint64(len(rec.Entries.Records))
		if start >= rec.Entries.Total {
			return out, nil
		}
	}
}
