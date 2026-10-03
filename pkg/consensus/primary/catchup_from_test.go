// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// A restarted node's round is the last it authored, which can be rounds past
// the certificates its checkpoint restored. Catch-up must ask from where its
// DAG ends, not from its round: asked from its round, the rounds between are
// never requested, and every certificate that arrives waits on parents in
// them (#4515).
func TestCatchUpAsksFromWhereTheDAGEndsNotFromTheAuthoredRound(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, nil)

	// The checkpoint restored full rounds 0..10.
	var prev []*types.Certificate
	for i := range validators {
		cert := buildCert(t, validators, i, 0, nil)
		require.NoError(t, d.InsertGenesis(cert))
		prev = append(prev, cert)
	}
	for r := types.Round(1); r <= 10; r++ {
		var certs []*types.Certificate
		for i := range validators {
			cert := buildCert(t, validators, i, r, prev)
			require.NoError(t, d.Insert(cert))
			certs = append(certs, cert)
		}
		prev = certs
	}
	// Round 11 holds one certificate: no quorum.
	require.NoError(t, d.Insert(buildCert(t, validators, 1, 11, prev)))

	// Before it stopped the node authored a header at round 30.
	h := types.NewHeader(validators[0].pub, 30, 1, nil, nil)
	require.NoError(t, h.Sign(validators[0].priv))
	p.RestoreAuthored(h)
	require.Equal(t, types.Round(30), p.CurrentRound(), "precondition: the restored round is the authored one")

	require.Equal(t, types.Round(10), p.catchUpFrom(p.CurrentRound()),
		"catch-up must start at the highest round the DAG holds a quorum of, not at the round before the authored one")

	// A node whose DAG is whole to the round before its own asks from there,
	// as it always did.
	require.Equal(t, types.Round(10), p.catchUpFrom(11))
}
