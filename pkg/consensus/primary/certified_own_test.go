// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// certifiedOwnFixture is one validator, one worker holding one batch, and a
// DAG with genesis in it — the least that lets a certificate of ours be
// counted the way tryCreateCertificateLocked counts one.
func certifiedOwnFixture(t *testing.T, partition string, txns [][]byte) (*Primary, *types.Batch, []*testValidator, *types.Committee) {
	t.Helper()

	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	createGenesisCertificates(t, validators, committee, d)

	w := worker.New(worker.Config{ID: 0, Partition: partition}, nil)
	batch := types.NewBatch(txns)
	require.NoError(t, w.StoreBatch(batch))

	p := New(Config{Partition: partition, KeyPair: v.priv}, committee, nil, d, []*worker.Worker{w})
	return p, batch, validators, committee
}

// ourCertificate builds a certificate of ours at a round, naming one batch,
// and puts it in the DAG — which is where countCertifiedOwn requires it.
func ourCertificate(t *testing.T, p *Primary, validators []*testValidator, committee *types.Committee, round types.Round, digest types.BatchDigest) *types.Certificate {
	t.Helper()
	v := validators[0]

	header := types.NewHeader(v.pub, round, committee.Epoch,
		[]types.PayloadEntry{{Digest: digest, Worker: 0}}, nil)
	require.NoError(t, header.Sign(v.priv))

	vote := types.NewVote(header.Digest(), round, committee.Epoch, v.pub)
	require.NoError(t, vote.Sign(v.priv))

	cert := types.NewCertificate(header, [][]byte{vote.Signature}, []uint16{0})
	require.NoError(t, p.dag.Insert(cert))
	return cert
}

// TestCertifiedOwn_ARequeuedBatchIsCountedOnce — the dedup window
// certified_own.go argues for at length, which nothing failed on.
//
// A header that never certifies is requeued and its batches are re-proposed,
// so the same batch can reach a second certified header. Counted twice, this
// node reports more certified than it accepted, and the harness reads that
// as an instrument fault on a working node.
func TestCertifiedOwn_ARequeuedBatchIsCountedOnce(t *testing.T) {
	const part = "CertOnce"
	txns := [][]byte{[]byte("one"), []byte("two"), []byte("three")}
	p, batch, vals, committee := certifiedOwnFixture(t, part, txns)

	before := testutil.ToFloat64(metrics.CertifiedOwnTransactionsTotal.WithLabelValues(part))

	first := ourCertificate(t, p, vals, committee, 1, batch.Digest())
	p.countCertifiedOwn(first)
	require.Equal(t, float64(len(txns)),
		testutil.ToFloat64(metrics.CertifiedOwnTransactionsTotal.WithLabelValues(part))-before,
		"the transactions of our certified header, counted once")

	// The same batch re-proposed and certified again, at a later round.
	second := ourCertificate(t, p, vals, committee, 2, batch.Digest())
	require.NotEqual(t, first.Digest(), second.Digest())
	p.countCertifiedOwn(second)
	require.Equal(t, float64(len(txns)),
		testutil.ToFloat64(metrics.CertifiedOwnTransactionsTotal.WithLabelValues(part))-before,
		"a re-proposed batch must be counted at its FIRST certified header and no other")
}

// TestCertifiedOwn_ACertificateNotInTheDagCountsNothing — the "after the
// insert" rule, as a precondition rather than an ordering.
//
// An insert that fails un-claims the round and requeues the header's batches
// (A13a), so a certificate that is not in the DAG is about to be proposed
// again, not certified. Counting it reports as certified what this node is
// still trying to get into a block.
func TestCertifiedOwn_ACertificateNotInTheDagCountsNothing(t *testing.T) {
	const part = "CertNotInDag"
	txns := [][]byte{[]byte("one"), []byte("two")}
	p, batch, vals, committee := certifiedOwnFixture(t, part, txns)

	before := testutil.ToFloat64(metrics.CertifiedOwnTransactionsTotal.WithLabelValues(part))

	// Built exactly as the counted one is, and never inserted.
	v := vals[0]
	header := types.NewHeader(v.pub, 1, committee.Epoch,
		[]types.PayloadEntry{{Digest: batch.Digest(), Worker: 0}}, nil)
	require.NoError(t, header.Sign(v.priv))
	vote := types.NewVote(header.Digest(), 1, committee.Epoch, v.pub)
	require.NoError(t, vote.Sign(v.priv))
	cert := types.NewCertificate(header, [][]byte{vote.Signature}, []uint16{0})
	require.Nil(t, p.dag.GetByDigest(cert.Digest()))

	p.countCertifiedOwn(cert)
	require.Equal(t, float64(0),
		testutil.ToFloat64(metrics.CertifiedOwnTransactionsTotal.WithLabelValues(part))-before,
		"a certificate that is not in the DAG is not certified")

	// And once it is in, it counts.
	require.NoError(t, p.dag.Insert(cert))
	p.countCertifiedOwn(cert)
	require.Equal(t, float64(len(txns)),
		testutil.ToFloat64(metrics.CertifiedOwnTransactionsTotal.WithLabelValues(part))-before)
}
