// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bcdb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// openDisk opens a LevelDB database in dir: a store whose contents are not
// heap, so what the heap holds is what the pull holds.
func openDisk(t *testing.T, dir string) *database.Database {
	t.Helper()
	db, err := database.OpenLevelDB(dir, nil)
	require.NoError(t, err)
	db.SetObserver(database.NewDatabaseObserver())
	return db
}

// openBcdb opens the BlockchainDB backend in dir, as a node on the soak runs
// it. It keeps isolation by pre-image: while any view is open, every commit
// after the view began keeps the values it overwrote.
func openBcdb(t *testing.T, dir string) (*database.Database, *bcdb.Database) {
	t.Helper()
	store, err := bcdb.Open(filepath.Join(dir, "db"))
	require.NoError(t, err)
	db := database.New(store, nil)
	db.SetObserver(database.NewDatabaseObserver())
	return db, store
}

// largeAnchor is a directory anchor as the Directory's pool holds one on its
// anchor-sequence chain, the anchors it sent: with receipts for the
// partitions' anchors the block included, about 16 KiB encoded.
func largeAnchor(pool *url.URL, i int) *messaging.TransactionMessage {
	body := new(protocol.DirectoryAnchor)
	body.Source = protocol.DnUrl()
	body.MinorBlockIndex = uint64(i + 1)
	body.RootChainIndex = uint64(i)
	for r := 0; r < 8; r++ {
		rc := new(merkle.Receipt)
		rc.Start = hashOf(i, r, -1)
		rc.End = rc.Start
		rc.Anchor = hashOf(i, r, -2)
		for e := 0; e < 60; e++ {
			rc.Entries = append(rc.Entries, &merkle.ReceiptEntry{Right: e%2 == 0, Hash: hashOf(i, r, e)})
		}
		pa := new(protocol.PartitionAnchor)
		pa.Source = protocol.PartitionUrl("BVN1")
		pa.MinorBlockIndex = uint64(i)
		pa.RootChainAnchor = *(*[32]byte)(hashOf(i, r, -3))
		body.Receipts = append(body.Receipts, &protocol.PartitionAnchorReceipt{Anchor: pa, RootChainReceipt: rc})
	}
	txn := new(protocol.Transaction)
	txn.Header.Principal = pool
	txn.Body = body
	return &messaging.TransactionMessage{Transaction: txn}
}

func hashOf(v ...int) []byte {
	var b [8 * 3]byte
	for i, x := range v {
		binary.BigEndian.PutUint64(b[i*8:], uint64(int64(x)))
	}
	h := sha256.Sum256(b[:])
	return h[:]
}

// writeLargePool writes a pool of n directory anchors, each an entry of its
// anchor-sequence chain with the message behind it. (Its main chain holds the
// anchors it received and executed, which a peer serves only with their
// signatures.)
func writeLargePool(t *testing.T, db *database.Database, pool *url.URL, n int) {
	t.Helper()
	b := db.Begin(true)
	require.NoError(t, b.Account(pool).Main().Put(&protocol.DataAccount{Url: pool}))
	c, err := b.Account(pool).ChainByName("anchor-sequence")
	require.NoError(t, err)
	_, err = c.Get()
	require.NoError(t, err)
	for i := 0; i < n; i++ {
		msg := largeAnchor(pool, i)
		h := msg.Hash()
		require.NoError(t, b.Message(h).Main().Put(msg))
		require.NoError(t, b.Account(pool).AnchorSequenceChain().Inner().AddEntry(h[:], false))
		if (i+1)%1000 == 0 {
			require.NoError(t, b.Commit())
			b = db.Begin(true)
		}
	}
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
}

// diskState is a joining node over a store on disk, fed by one peer.
func diskState(db *database.Database, here *url.URL, src pull.Source) *PulledState {
	return &PulledState{
		partition: here,
		db:        db,
		sources:   &oneSource{part: here, src: src},
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// peakHeap samples HeapInuse, and the store's overlays when it has them,
// until stop is closed, and reports the most of each seen.
func peakHeap(stop <-chan struct{}, store *bcdb.Database) <-chan [2]uint64 {
	out := make(chan [2]uint64, 1)
	go func() {
		var peak, overlays uint64
		t := time.NewTicker(5 * time.Millisecond)
		defer t.Stop()
		for {
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			if ms.HeapInuse > peak {
				peak = ms.HeapInuse
			}
			if store != nil {
				if n := uint64(store.Overlays()); n > overlays {
					overlays = n
				}
			}
			select {
			case <-stop:
				out <- [2]uint64{peak, overlays}
				return
			case <-t.C:
			}
		}
	}()
	return out
}

// TestJoin_AWholeChainPullHoldsAPageNotTheChain — #4446. A follower added to
// a network with 13,400 blocks of history was killed at its 2 GiB limit in
// the join's first pull: the whole-account pull of the anchor pool fetched
// every entry and every message into slices, replayed them into a batch that
// held every record, and wrote nothing until the account was done, so its
// peak grew with the history. Driven through the join's own pullOne, with its
// own store -- the BlockchainDB backend, as the soak runs it: the first fix
// streamed the pages but kept pullOne's batch open across them, and on that
// store every page's pre-images stayed pinned for the account (1.06 GiB in 10
// minutes on a 37,000-block network, 4,400 overlays; a LevelDB store keeps no
// such overlay and did not show it). A pool of 20,000 directory anchors of about 16 KiB each (some
// 320 MiB of messages) must be taken with the heap staying under a fixed
// ceiling above where it started. Measured on this change: about 150 MiB
// above the baseline at 20,000 entries and 128 MiB at 5,000 -- most of it the
// stores' own buffers, the peer's LevelDB block buffers and both memtables,
// fixed by their options -- against 3,328 MiB before it.
func TestJoin_AWholeChainPullHoldsAPageNotTheChain(t *testing.T) {
	if testing.Short() {
		t.Skip("writes and pulls 320 MiB")
	}
	const entries = 20_000
	const ceiling = 200 << 20
	const maxOverlays = 4 // a page's own commit, not the chain's

	here := protocol.DnUrl()
	pool := here.JoinPath(protocol.AnchorPool)
	peer := openDisk(t, t.TempDir())
	t.Cleanup(func() { _ = peer.Close() })
	writeLargePool(t, peer, pool, entries)

	node, store := openBcdb(t, t.TempDir())
	t.Cleanup(func() { _ = node.Close() })
	src := api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peer, Partition: "BVN0"})}
	s := diskState(node, here, servedAt{Source: src, db: peer})
	s.checkedHeld = true
	p := newSyncing()

	// Collected often, so what is in use is what is held and not garbage
	// the collector has yet to reach.
	defer debug.SetGCPercent(debug.SetGCPercent(20))
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	base := ms.HeapInuse

	stop := make(chan struct{})
	peak := peakHeap(stop, store)
	got := s.pullOne(context.Background(), p, pool)
	close(stop)
	seen := <-peak
	grew := int64(seen[0]) - int64(base)
	t.Logf("heap in use: baseline %d MiB, peak %d MiB above it; most overlays held %d", base>>20, grew>>20, seen[1])

	require.Equal(t, taken, got, "the pool was not taken: %v", p.retry)
	requireSameChain(t, peer, node, pool, "anchor-sequence")
	require.LessOrEqual(t, seen[1], uint64(maxOverlays), "a view held open across the stream pinned the pages' pre-images")
	require.Less(t, grew, int64(ceiling), "the pull's heap grew with the chain it took")
}

// cutAfter is a peer whose chain entries stop answering after n pages: the
// process taking them is killed there.
type cutAfter struct {
	pull.Source
	n      int32
	cancel context.CancelFunc
	pages  atomic.Int32
	once   sync.Once
}

func (c *cutAfter) QueryChainEntries(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	if q.Range != nil && c.pages.Add(1) > c.n {
		c.once.Do(c.cancel)
		return nil, context.Canceled
	}
	return c.Source.QueryChainEntries(ctx, u, q)
}

// TestJoin_APullKilledMidAccountResumes — #4446. The pages of a whole-chain
// pull are written as they arrive, so a process killed part way through an
// account has written some of its chain under a head it does not yet hold.
// The next process takes the account again and ends holding the peer's chain
// entry by entry, with the message behind every entry, and the account
// hashes as the peer's does.
func TestJoin_APullKilledMidAccountResumes(t *testing.T) {
	const entries = 3_000 // a dozen pages

	here := protocol.DnUrl()
	pool := here.JoinPath(protocol.AnchorPool)
	peer := openDisk(t, t.TempDir())
	t.Cleanup(func() { _ = peer.Close() })
	writeLargePool(t, peer, pool, entries)
	src := api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peer, Partition: "BVN0"})}

	dir := t.TempDir()
	node := openDisk(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	cut := &cutAfter{Source: src, n: 5, cancel: cancel}
	s := diskState(node, here, servedAt{Source: cut, db: peer})
	s.checkedHeld = true
	require.Equal(t, owed, s.pullOne(ctx, newSyncing(), pool), "precondition: the pull was killed")

	// Killed there, the pages it took are on disk and the head is not.
	func() {
		b := node.Begin(false)
		defer b.Discard()
		c := b.Account(pool).AnchorSequenceChain().Inner()
		head, err := c.Head().Get()
		require.NoError(t, err)
		require.Zero(t, head.Count, "the head was written before the chain under it was whole")
		_, err = c.Element(0).Get()
		require.NoError(t, err, "a page taken before the kill was not written: the pull held it")
	}()
	require.NoError(t, node.Close())

	// The next process.
	node = openDisk(t, dir)
	t.Cleanup(func() { _ = node.Close() })
	s = diskState(node, here, servedAt{Source: src, db: peer})
	p := newSyncing()
	require.Equal(t, taken, s.pullOne(context.Background(), p, pool), "the pool was not taken on resuming: %v", p.retry)
	requireSameChain(t, peer, node, pool, "anchor-sequence")

	pb := peer.Begin(false)
	defer pb.Discard()
	nb := node.Begin(false)
	defer nb.Discard()
	want, err := pb.Account(pool).Hash()
	require.NoError(t, err)
	gotHash, err := nb.Account(pool).Hash()
	require.NoError(t, err)
	require.Equal(t, want, gotHash, "the resumed account does not hash as the peer's")
}

// requireSameChain holds the node's chain to the peer's entry by entry, with
// the message behind every entry.
func requireSameChain(t *testing.T, peer, node *database.Database, u *url.URL, name string) {
	t.Helper()
	// Deep: the whole history, past the window a BlockchainDB protocol read
	// answers from.
	pb := peer.Deep().Begin(false)
	defer pb.Discard()
	nb := node.Deep().Begin(false)
	defer nb.Discard()
	pc, err := pb.Account(u).ChainByName(name)
	require.NoError(t, err)
	nc, err := nb.Account(u).ChainByName(name)
	require.NoError(t, err)
	ph, err := pc.Inner().Head().Get()
	require.NoError(t, err)
	nh, err := nc.Inner().Head().Get()
	require.NoError(t, err)
	require.Equal(t, ph.Count, nh.Count, "chain height")
	require.Equal(t, ph.Anchor(), nh.Anchor(), "chain anchor")
	for i := int64(0); i < ph.Count; i++ {
		want, err := pc.Inner().Entry(i)
		require.NoError(t, err)
		got, err := nc.Inner().Entry(i)
		require.NoError(t, err, "entry %d", i)
		require.Equal(t, want, got, "entry %d", i)
		_, err = nb.Message(*(*[32]byte)(got)).Main().Get()
		require.NoError(t, err, "entry %d has no message behind it", i)
	}
}
