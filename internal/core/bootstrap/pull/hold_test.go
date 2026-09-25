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
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// counting is a peer that says how many times its state was fetched, and which
// partition it puts in the receipts it serves.
type counting struct {
	*peer
	partition string
	fetches   int
}

func (c *counting) QueryAccount(ctx context.Context, u *url.URL, q *api.DefaultQuery) (*api.AccountRecord, error) {
	rec, err := c.peer.QueryAccount(ctx, u, q)
	if err != nil {
		return nil, err
	}
	c.fetches++
	if rec.Receipt != nil {
		rec.Receipt.Partition = c.partition
	}
	return rec, nil
}

// TestFetch_TakesThePartitionFromTheReceipt — #4308.
//
// Pending.Partition used to be the PULLER's partition. The block a receipt
// names is the SERVING partition's, and block numbers collide across
// partitions -- at a one-second cadence the Directory and a BVN are at the same
// number at the same second -- so settling a foreign account's block against
// this node's partition asks the Directory for a root it never anchored.
// Latent while every account a join pulls is its own partition's, wrong the
// moment a block names one that is not.
func TestFetch_TakesThePartitionFromTheReceipt(t *testing.T) {
	src, u, _ := alice(t)
	local := newObservedDB(t)
	batch := local.Begin(true)
	defer batch.Discard()

	served := &counting{peer: &peer{dbSource: &dbSource{db: src}}, partition: "BVN1"}
	p, err := Fetch(context.Background(), served, batch, u, Options{
		Mode: ModeStateOnly,
		// The puller is the Directory; the peer that answered is BVN1.
		Partition: protocol.DnUrl(),
	}, true)
	require.NoError(t, err)
	defer p.Discard()

	require.Equal(t, protocol.PartitionUrl("BVN1"), p.Partition,
		"the block was attributed to the puller's partition, not the one that served it")
	require.Equal(t, uint64(servedBlock), p.Block)
}

// TestFetch_KeepsThePullersPartitionWhenTheReceiptNamesNone — a peer that names
// no partition leaves the caller's answer standing, so an older peer does not
// make every block unattributable.
func TestFetch_KeepsThePullersPartitionWhenTheReceiptNamesNone(t *testing.T) {
	src, u, _ := alice(t)
	local := newObservedDB(t)
	batch := local.Begin(true)
	defer batch.Discard()

	served := &counting{peer: &peer{dbSource: &dbSource{db: src}}}
	p, err := Fetch(context.Background(), served, batch, u, Options{
		Mode:      ModeStateOnly,
		Partition: protocol.DnUrl(),
	}, true)
	require.NoError(t, err)
	defer p.Discard()
	require.Equal(t, protocol.DnUrl(), p.Partition)
}

// TestFetchFrom_AsksTheNextSourceWhenOneCannotServe — a peer that cannot answer
// is that peer's condition, not an answer about the account. The join addresses
// named peers precisely so it has a next one to ask (#4303).
func TestFetchFrom_AsksTheNextSourceWhenOneCannotServe(t *testing.T) {
	src, u, _ := alice(t)
	local := newObservedDB(t)
	batch := local.Begin(true)
	defer batch.Discard()

	empty := &counting{peer: &peer{dbSource: &dbSource{db: src}, empty: true}}
	good := &counting{peer: &peer{dbSource: &dbSource{db: src}}}

	p, i, err := FetchFrom(context.Background(), []Source{empty, good}, batch, u, Options{
		Mode:      ModeStateOnly,
		Partition: protocol.DnUrl(),
	})
	require.NoError(t, err)
	require.Equal(t, 1, i, "the second source answered")
	require.NoError(t, p.Keep())

	_, _, err = FetchFrom(context.Background(), nil, batch, u, Options{Mode: ModeStateOnly})
	require.Error(t, err, "a fetch with no source is a caller error, not an empty answer")
}
