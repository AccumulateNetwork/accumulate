// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// A vote deferred for what a header names is cast when the last of it lands,
// not on the author's next rebroadcast (#4611; consensus.md invariant 11).
// The primary runs its loop with a round tick of an hour, so no rebroadcast
// and no round advance happens during the test: the vote can only come from
// the arrival itself. Waiting for the rebroadcast cost every header after a
// restart a second or more after its batches were in, and a partition went
// 7-20 s without a block (TestRestartAfterTwoNodesStopFirst).
func TestDeferredVoteIsCastWhenWhatItAskedForLands(t *testing.T) {
	t.Run("batch", func(t *testing.T) {
		validators := make([]*testValidator, 4)
		for i := range validators {
			validators[i] = newTestValidator(t)
		}
		committee := newTestCommittee(validators, 1)
		d := newTestDAG()
		var parents []types.CertificateDigest
		for _, c := range createGenesisCertificates(t, validators, committee, d) {
			parents = append(parents, c.Digest())
		}
		w := worker.New(worker.Config{ID: 0, Partition: "test"}, nil)
		p := startForRetest(t, validators[0], committee, d, w)

		batch := types.NewBatch([][]byte{[]byte("fetched-later")})
		header := types.NewHeader(validators[1].pub, 1, 1,
			[]types.PayloadEntry{{Digest: batch.Digest(), Worker: 0}}, parents)
		require.NoError(t, header.Sign(validators[1].priv))

		p.OnHeaderReceived(header)
		require.False(t, votedOn(p, header), "must not vote before the batch is held")

		require.NoError(t, w.StoreBatch(batch))
		require.Eventually(t, func() bool { return votedOn(p, header) }, 2*time.Second, 10*time.Millisecond,
			"the vote must go out when the batch is stored, without a rebroadcast")
	})

	t.Run("parent", func(t *testing.T) {
		validators := make([]*testValidator, 4)
		for i := range validators {
			validators[i] = newTestValidator(t)
		}
		committee := newTestCommittee(validators, 1)
		d := newTestDAG()
		// Every genesis certificate but the last is in the DAG; the last
		// arrives after the header that names it.
		var parents []types.CertificateDigest
		var late *types.Certificate
		for i, v := range validators {
			h := types.NewHeader(v.pub, 0, 1, nil, nil)
			require.NoError(t, h.Sign(v.priv))
			sigs := make([][]byte, len(validators))
			authors := make([]uint16, len(validators))
			for j, voter := range validators {
				vote := types.NewVote(h.Digest(), 0, 1, voter.pub)
				require.NoError(t, vote.Sign(voter.priv))
				sigs[j], authors[j] = vote.Signature, uint16(j)
			}
			cert := types.NewCertificate(h, sigs, authors)
			parents = append(parents, cert.Digest())
			if i == len(validators)-1 {
				late = cert
				continue
			}
			require.NoError(t, d.InsertGenesis(cert))
		}
		p := startForRetest(t, validators[0], committee, d, worker.New(worker.Config{ID: 0, Partition: "test"}, nil))

		header := types.NewHeader(validators[1].pub, 1, 1, nil, parents)
		require.NoError(t, header.Sign(validators[1].priv))

		p.OnHeaderReceived(header)
		require.False(t, votedOn(p, header), "must not vote before the parent is held")

		p.OnCertificateReceived(late)
		require.NotNil(t, d.GetByDigest(late.Digest()), "the parent must be inserted for the test to mean anything")
		require.Eventually(t, func() bool { return votedOn(p, header) }, 2*time.Second, 10*time.Millisecond,
			"the vote must go out when the parent is inserted, without a rebroadcast")
	})
}

// startForRetest starts a primary at round 1 whose round tick never fires.
func startForRetest(t *testing.T, v *testValidator, committee *types.Committee, d *dag.DAG, w *worker.Worker) *Primary {
	t.Helper()
	p := New(Config{Partition: "test", KeyPair: v.priv, RoundAdvanceInterval: time.Hour, NewCertsChannelSize: 16},
		committee, nil, d, []*worker.Worker{w})
	p.SetRound(1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = p.Start(ctx) }()
	t.Cleanup(func() { cancel(); <-done; p.Stop() })
	return p
}

func votedOn(p *Primary, h *types.Header) bool {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	_, ok := p.votedHeaders[h.Digest()]
	return ok
}
