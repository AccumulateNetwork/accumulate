// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// TestRepeatedAnswersAreDroppedUnread is #4610: under load, suppression
// fails and most peers answer the same request. A joining follower handled
// every copy -- a median of 8 per request, up to 100 certificates each --
// and its own answers waited behind them for 24 s at the median. A request
// answered in full is done; an answer that reports certificates missing
// leaves it open for the next.
func TestRepeatedAnswersAreDroppedUnread(t *testing.T) {
	me, other := []byte("this node's key"), []byte("another node's key")
	config := CertSyncerConfig{PublicKey: me, Partition: "t4610"}
	config.applyDefaults()
	s := NewCertSyncer(config, dag.NewDAG(10), nil, NewPendingCertificates(100))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.ctx = ctx

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cert := func(round types.Round) *types.Certificate {
		h := types.NewHeader(pub, round, 0, nil, nil)
		require.NoError(t, h.Sign(priv))
		return types.NewCertificate(h, [][]byte{h.Signature}, []uint16{0})
	}
	full, partial, last := cert(1), cert(2), cert(3)

	responses := make(chan *gossip.CertSyncResponse, 32)
	// Eight peers answer our request 1 in full, and eight answer another
	// node's request 1 in full.
	for range 8 {
		responses <- &gossip.CertSyncResponse{Certificates: []*types.Certificate{full}, RequestID: 1, Requester: me}
		responses <- &gossip.CertSyncResponse{Certificates: []*types.Certificate{full}, RequestID: 1, Requester: other}
	}
	// One peer answers our request 2 in part, a second in full.
	responses <- &gossip.CertSyncResponse{Certificates: []*types.Certificate{partial}, RequestID: 2, Requester: me, Missing: []types.CertificateDigest{{9}}}
	responses <- &gossip.CertSyncResponse{Certificates: []*types.Certificate{partial}, RequestID: 2, Requester: me}
	responses <- &gossip.CertSyncResponse{Certificates: []*types.Certificate{last}, RequestID: 3, Requester: me}

	handled := map[types.CertificateDigest]int{}
	done := make(chan struct{})
	s.SetCertReceivedCallback(func(c *types.Certificate) CertOutcome {
		handled[c.Digest()]++
		if c.Digest() == last.Digest() {
			close(done)
		}
		return CertDuplicate
	})
	go s.readSyncResponses(responses)

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the responses were never handled")
	}
	require.Equal(t, 2, handled[full.Digest()], "each request answered in full is handled once, not once per answer")
	require.Equal(t, 2, handled[partial.Digest()], "an answer reporting certificates missing leaves its request open")
	require.Equal(t, 14, s.window.take().repeated, "the copies after the first of each full answer")
}
