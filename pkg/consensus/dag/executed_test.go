// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dag

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// A batch a certified header names is protected in the active store until
// this node executes it (consensus spec, invariant 3). Once executed it is
// cache: a late copy that arrives after retention dropped it must not be
// protected again for the rounds the DAG keeps the certificate, or a peer
// could replay executed batches into a store that never prunes them
// (#4598). A later certificate naming the same batch does not revive the
// protection, and the mark goes when the DAG collects the batch's entry.
func TestExecutedBatchIsNotCertified(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	batch := types.NewBatch([][]byte{[]byte("named, executed, named again")})
	bd := batch.Digest()
	cert := func(round types.Round, payload []types.PayloadEntry, parents []types.CertificateDigest) *types.Certificate {
		h := types.NewHeader(pub, round, 0, payload, parents)
		require.NoError(t, h.Sign(priv))
		return types.NewCertificate(h, nil, nil)
	}
	names := []types.PayloadEntry{{Digest: bd}}

	d := NewDAG(2)
	genesis := cert(0, nil, nil)
	require.NoError(t, d.InsertGenesis(genesis))
	first := cert(1, names, []types.CertificateDigest{genesis.Digest()})
	require.NoError(t, d.Insert(first))
	require.True(t, d.HasCertifiedBatch(bd), "a certified header names it")

	d.MarkExecuted(first)
	require.False(t, d.HasCertifiedBatch(bd), "executed: the certificate no longer protects it")

	later := cert(2, names, []types.CertificateDigest{first.Digest()})
	require.NoError(t, d.Insert(later))
	require.False(t, d.HasCertifiedBatch(bd), "a later certificate naming an executed batch does not protect it again")

	// Collected past both rounds: the entry and its mark go together.
	d.GarbageCollect(10)
	require.False(t, d.HasCertifiedBatch(bd))
	d.mu.RLock()
	defer d.mu.RUnlock()
	require.Empty(t, d.executed, "the mark is collected with the batch's entry")
	require.Empty(t, d.certifiedBatches)
}
