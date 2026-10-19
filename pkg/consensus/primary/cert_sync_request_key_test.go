// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// TestSuppressionNamesTheRequester is #4470: a peer that saw node A's
// request k answered must still answer node B's request k. Every node
// numbers its own requests, so the two IDs collide; suppression keyed on the
// ID alone skipped B's request.
func TestSuppressionNamesTheRequester(t *testing.T) {
	config := CertSyncerConfig{PublicKey: make([]byte, 32), Partition: "t4470"}
	config.applyDefaults()
	d := dag.NewDAG(10)
	peer := NewCertSyncer(config, d, nil, NewPendingCertificates(100))
	// The answer this peer saw verified (#4597): what did not verify
	// suppresses nothing.
	peer.SetCertReceivedCallback(func(*types.Certificate) CertOutcome { return CertDuplicate })

	nodeA, nodeB := []byte("node A's key"), []byte("node B's key")
	const k = 7

	// This peer holds the certificate both nodes ask for.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	h := types.NewHeader(pub, 0, 0, nil, nil)
	require.NoError(t, h.Sign(priv))
	cert := types.NewCertificate(h, [][]byte{h.Signature}, []uint16{0})
	require.NoError(t, d.InsertGenesis(cert))
	want := []types.CertificateDigest{cert.Digest()}

	// Another peer answered node A's request k, and this peer saw it.
	peer.handleSyncResponse(&gossip.CertSyncResponse{
		Certificates: []*types.Certificate{cert},
		RequestID:    k,
		Requester:    nodeA,
	})
	peer.window.take()

	// Node B's request k is answered; node A's own request k, re-broadcast,
	// is suppressed.
	require.NotNil(t, peer.answer(&gossip.CertSyncRequest{Digests: want, Requester: nodeB, RequestID: k}),
		"node B's request %d was skipped because node A's request %d was answered", k, k)
	require.Equal(t, 0, peer.window.take().suppressed)

	require.Nil(t, peer.answer(&gossip.CertSyncRequest{Digests: want, Requester: nodeA, RequestID: k}))
	require.Equal(t, 1, peer.window.take().suppressed)
}

// TestRequestIDsDoNotStartAtOne is #4470: a node restarted under the same key
// numbered its requests from 1 again, the IDs its peers had seen answered
// before it stopped, so they skipped them.
func TestRequestIDsDoNotStartAtOne(t *testing.T) {
	config := CertSyncerConfig{PublicKey: make([]byte, 32), Partition: "t4470"}
	before := NewCertSyncer(config, dag.NewDAG(10), nil, NewPendingCertificates(100))
	after := NewCertSyncer(config, dag.NewDAG(10), nil, NewPendingCertificates(100))
	require.NotEqual(t, before.nextReqID.Add(1), after.nextReqID.Add(1),
		"two syncers under the same key issued the same first request ID")
}
