// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"crypto/rand"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

// quietSection is a section holding only accounts no block touches: some of
// the accounts only the validators hold, and nothing else on any node.
func (j *sectionsJoin) quietSection() bpt.Section {
	t := j.t
	busy := map[bpt.Section]bool{}
	for _, u := range join.SystemAccounts(PartitionUrl("BVN0")) {
		busy[database.SectionOfAccount(u)] = true
	}
	for _, id := range []*url.URL{j.alice, j.bob, j.dave} {
		for _, u := range []*url.URL{id, id.JoinPath("book"), id.JoinPath("book", "1"), id.JoinPath("tokens")} {
			busy[database.SectionOfAccount(u)] = true
		}
	}
	many := map[[32]byte]bool{}
	for _, u := range j.many {
		many[record.NewKey("Account", u).Hash()] = true
	}
	for _, u := range j.many {
		s := database.SectionOfAccount(u)
		if busy[s] {
			continue
		}
		only := true
		for k := range sectionLeavesOf(t, j.part.NodeDatabase(0), s) {
			if !many[k] {
				only = false
			}
		}
		if only {
			return s
		}
	}
	t.Fatal("no section holds only accounts no block touches")
	return 0
}

// sectionLeavesOf is a node's leaves in a section, by key hash.
func sectionLeavesOf(t *testing.T, db *database.Database, s bpt.Section) map[[32]byte][32]byte {
	b := db.Ungated().Begin(false)
	defer b.Discard()
	out := map[[32]byte][32]byte{}
	start := s.ScanStart()
	for {
		pairs, next, err := b.BPT().GetRange(start, 256)
		require.NoError(t, err)
		for _, kv := range pairs {
			kh := kv.Key.Hash()
			switch es := bpt.SectionOf(kh); {
			case es > s:
				continue
			case es < s:
				return out
			}
			out[kh] = [32]byte(kv.Value)
		}
		if len(pairs) < 256 {
			return out
		}
		start = next
	}
}

// markSection makes a node hold a section by its hash alone, as a node that
// joined and has not loaded it does: its accounts forgotten and one marker
// leaf in their place. The root does not move.
func markSection(t *testing.T, db *database.Database, s bpt.Section) {
	leaves := sectionLeavesOf(t, db, s)
	var list []bpt.SectionLeaf
	for k, v := range leaves {
		list = append(list, bpt.SectionLeaf{KeyHash: k, Hash: v})
	}
	h, ok := bpt.SectionHash(list)
	require.True(t, ok)

	b := db.Ungated().Begin(true)
	defer b.Discard()
	before, err := b.GetBptRootHash()
	require.NoError(t, err)
	start := s.ScanStart()
	pairs, _, err := b.BPT().GetRange(start, len(leaves)+256)
	require.NoError(t, err)
	for _, kv := range pairs {
		if bpt.SectionOf(kv.Key.Hash()) != s {
			continue
		}
		u, ok := kv.Key.Get(1).(*url.URL)
		require.True(t, ok)
		require.NoError(t, b.ForgetAccount(u))
	}
	require.NoError(t, b.BPT().Insert(s.MarkerKey(), h[:]))
	after, err := b.GetBptRootHash()
	require.NoError(t, err)
	require.Equal(t, before, after, "a marker moved the root")
	require.NoError(t, b.Commit())
}

// dropPeers is a source wrap that leaves out the named peers.
func dropPeers(ids ...peer.ID) func([]pull.Source) []pull.Source {
	return func(srcs []pull.Source) []pull.Source {
		var out []pull.Source
	next:
		for _, s := range srcs {
			for _, id := range ids {
				if strings.Contains(sourceName(s), id.String()) {
					continue next
				}
			}
			out = append(out, s)
		}
		return out
	}
}

// TestASectionOnlyOnePeerHoldsIsLoadedThroughARedirect — #4485, Paul
// 2026-10-02 (DIFFERENCES E26): "nodes that can't answer should be
// redirecting". Two of the three validators hold a section by its hash alone,
// as joiners still filling do; only validator 0 holds it, and the joiner's
// own peers do not include validator 0. The joiner takes the top from a peer
// that holds markers; its fill asks those peers for the section, they refuse
// naming the peers to ask instead, and the fill loads it from validator 0,
// checked against the section's hash.
func TestASectionOnlyOnePeerHoldsIsLoadedThroughARedirect(t *testing.T) {
	j := prepareSectionsJoin(t)
	s := j.quietSection()
	for _, n := range []int{1, 2} {
		markSection(t, j.part.NodeDatabase(n), s)
	}
	j.part.SetSourceWrap(j.fresh, dropPeers(j.part.NodePeerID(0)))
	j.run()

	var in *url.URL
	for _, u := range j.many {
		if database.SectionOfAccount(u) == s {
			in = u
			break
		}
	}
	require.True(t, j.notLoaded(in), "precondition: the joiner loaded the section only one peer holds")

	j.fill(func(int) bool { return !j.notLoaded(in) })
	require.False(t, j.notLoaded(in), "the fill did not load the section only one peer holds")
	j.agree("after the redirected load")
	want, err := j.part.NodeDatabase(0).Begin(false).Account(in).Hash()
	require.NoError(t, err)
	got, err := j.part.NodeDatabase(j.fresh).Ungated().Begin(false).Account(in).Hash()
	require.NoError(t, err)
	require.Equal(t, want, got, "%v is not the holder's", in)
}

// forgingQuerier serves the partition honestly, except a page of one section,
// whose first leaf it forges.
type forgingQuerier struct {
	api.Querier
	section bpt.Section
	page    *api.BptPageRecord
	asked   *atomic.Int64
}

func (f forgingQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	if q, ok := query.(*api.BptPageQuery); ok && bpt.SectionOf(q.StartHash) == f.section {
		f.asked.Add(1)
		forged := *f.page
		forged.Entries = append([]*api.BptLeafSummary(nil), f.page.Entries...)
		for i, e := range forged.Entries {
			if e != nil && bpt.SectionOf(e.KeyHash) == f.section {
				c := *e
				c.ValueHash[0] ^= 0xff
				forged.Entries[i] = &c
				break
			}
		}
		return &forged, nil
	}
	return f.Querier.Query(ctx, scope, query)
}

// TestAForgedRedirectTargetIsRefusedByTheHash — #4485, DIFFERENCES E26.
// Every validator holds a section by its hash alone, and a stranger -- a peer
// that is no node of the network -- serves it forged. The joiner reaches the
// stranger only through the validators' redirect; its answer does not hash
// to the section's marker, and nothing of it is written.
func TestAForgedRedirectTargetIsRefusedByTheHash(t *testing.T) {
	j := prepareSectionsJoin(t)
	s := j.quietSection()

	honest := apiimpl.NewQuerier(apiimpl.QuerierParams{Database: j.part.NodeDatabase(0), Partition: "BVN0"})
	rec, err := honest.Query(context.Background(), PartitionUrl("BVN0"), &api.BptPageQuery{StartHash: s.ScanStart(), Count: 256})
	require.NoError(t, err)
	page, ok := rec.(*api.BptPageRecord)
	require.True(t, ok)
	for _, n := range []int{0, 1, 2} {
		markSection(t, j.part.NodeDatabase(n), s)
	}

	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	require.NoError(t, err)
	stranger, err := peer.IDFromPrivateKey(key)
	require.NoError(t, err)
	var asked atomic.Int64
	h, err := message.NewHandler(message.Querier{Querier: forgingQuerier{Querier: honest, section: s, page: page, asked: &asked}})
	require.NoError(t, err)
	require.True(t, j.sim.S.RegisterPeerService(stranger, api.ServiceTypeQuery.AddressFor("BVN0"), h.Handle))

	// The joiner's own peers do not include the stranger: it is reached
	// only by a redirect.
	j.part.SetSourceWrap(j.fresh, dropPeers(stranger))
	j.run()

	var in *url.URL
	for _, u := range j.many {
		if database.SectionOfAccount(u) == s {
			in = u
			break
		}
	}
	require.True(t, j.notLoaded(in), "precondition: the joiner loaded the section no validator holds")

	rounds := 0
	j.fill(func(int) bool { rounds++; return asked.Load() > 0 && rounds > 20 })
	require.Positive(t, asked.Load(), "no redirect reached the stranger")
	require.True(t, j.notLoaded(in), "the stranger's forged section was loaded")
	_, err = j.part.NodeDatabase(j.fresh).Ungated().Begin(false).Account(in).Main().Get()
	require.ErrorIs(t, err, errors.NotFound, "what the stranger served was written")
	j.agree("after the stranger's section was refused")
}
