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

// TestForgedSyncResponseIsNotAnAnswer is #4597: sync responses are unsigned
// and anyone can send one, so a response counts as an answer only once the
// certificates it carries verify. Before this, a response carrying one junk
// certificate under the requester's key and request ID was recorded as an
// answer by every peer, which then suppressed its own, and the requester
// dropped the digest from its in-flight set, never to retry it.
func TestForgedSyncResponseIsNotAnAnswer(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	committee := types.NewCommittee([]types.ValidatorInfo{{PublicKey: pub, Stake: 1}}, 0)

	// The genuine certificate: the member's header, signed, with the
	// member's vote.
	h := types.NewHeader(pub, 0, 0, nil, nil)
	require.NoError(t, h.Sign(priv))
	v := types.NewVote(h.Digest(), h.Round, h.Epoch, pub)
	require.NoError(t, v.Sign(priv))
	genuine := types.NewCertificate(h, [][]byte{v.Signature}, []uint16{0})
	require.NoError(t, genuine.Verify(committee))

	// The forgeries name the member as author and carry no signature worth
	// the name. Their author is in the committee, so the topic validator
	// relays them (consensus.md, "Who may send consensus messages"). One
	// invents a header; the other copies the genuine header, which is
	// public, so its digest is the one requested.
	forgedHeader := types.NewHeader(pub, 5, 0, nil, nil)
	forgedHeader.Signature = make([]byte, ed25519.SignatureSize)
	forged := types.NewCertificate(forgedHeader, [][]byte{make([]byte, ed25519.SignatureSize)}, []uint16{0})
	require.Error(t, forged.Verify(committee))
	copied := types.NewCertificate(h, [][]byte{make([]byte, ed25519.SignatureSize)}, []uint16{0})
	require.Error(t, copied.Verify(committee))
	require.Equal(t, genuine.Digest(), copied.Digest())

	// What the primary does with a received certificate, reduced to the
	// verification (receiveCertificate).
	verify := func(c *types.Certificate) CertOutcome {
		if err := c.Verify(committee); err != nil {
			return CertInvalid
		}
		return CertInserted
	}

	requester := []byte("the requester's key")
	const k = 11

	t.Run("a peer still answers the request", func(t *testing.T) {
		config := CertSyncerConfig{PublicKey: make([]byte, 32), Partition: "t4597a"}
		config.applyDefaults()
		d := dag.NewDAG(10)
		require.NoError(t, d.InsertGenesis(genuine))
		peer := NewCertSyncer(config, d, nil, NewPendingCertificates(100))
		peer.SetCertReceivedCallback(verify)

		// The forgery arrives first, under the requester's key and ID.
		peer.handleSyncResponse(&gossip.CertSyncResponse{
			Certificates: []*types.Certificate{forged},
			RequestID:    k,
			Requester:    requester,
		})
		peer.window.take()

		// Then the genuine request, for a certificate this peer holds.
		peer.handleSyncRequest(&gossip.CertSyncRequest{
			Digests:   []types.CertificateDigest{genuine.Digest()},
			Requester: requester,
			RequestID: k,
		})
		require.Equal(t, 0, peer.window.take().suppressed, "the request was suppressed by the forged response")
		peer.answeredMu.Lock()
		_, answered := peer.answered[keyOf(requester, k)]
		peer.answeredMu.Unlock()
		require.False(t, answered, "the forged response was recorded as an answer")
	})

	t.Run("the requester keeps asking", func(t *testing.T) {
		config := CertSyncerConfig{PublicKey: requester, Partition: "t4597b", RetryTimeout: time.Millisecond, DeduplicationInterval: time.Millisecond}
		config.applyDefaults()
		s := NewCertSyncer(config, dag.NewDAG(10), nil, NewPendingCertificates(100))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		s.ctx = ctx

		// The digest is in flight under request k.
		s.inFlightMu.Lock()
		s.inFlight[genuine.Digest()] = &inFlightRequest{digest: genuine.Digest(), requestID: k, sentAt: time.Now().Add(-time.Second)}
		s.inFlightMu.Unlock()

		outcomes := make(chan CertOutcome, 4)
		s.SetCertReceivedCallback(func(c *types.Certificate) CertOutcome {
			o := verify(c)
			outcomes <- o
			return o
		})
		responses := make(chan *gossip.CertSyncResponse, 4)
		go s.readSyncResponses(responses)

		// The copy answers request k with the digest asked for.
		responses <- &gossip.CertSyncResponse{Certificates: []*types.Certificate{copied}, RequestID: k, Requester: requester}
		require.Equal(t, CertInvalid, <-outcomes)

		s.inFlightMu.Lock()
		_, inFlight := s.inFlight[genuine.Digest()]
		s.inFlightMu.Unlock()
		require.True(t, inFlight, "the forged response took the digest out of the in-flight set")

		// It is retried: the retry check queues it again.
		s.checkRetries()
		s.batchQueueMu.Lock()
		queued := append([]types.CertificateDigest(nil), s.batchQueue...)
		s.batchQueueMu.Unlock()
		require.Contains(t, queued, genuine.Digest(), "the digest was not retried")

		// And the genuine answer to request k is still read.
		responses <- &gossip.CertSyncResponse{Certificates: []*types.Certificate{genuine}, RequestID: k, Requester: requester}
		select {
		case o := <-outcomes:
			require.Equal(t, CertInserted, o)
		case <-time.After(5 * time.Second):
			t.Fatal("the genuine answer was dropped: the forged response completed the request")
		}
	})
}
