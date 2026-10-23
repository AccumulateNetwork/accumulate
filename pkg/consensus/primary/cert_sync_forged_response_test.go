// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// TestForgedSyncResponseIsNotAnAnswer is #4597: sync responses are unsigned
// and anyone can send one, so a response counts as an answer only once the
// certificates it carries check. Before this, a response carrying one junk
// certificate under the requester's key and request ID was recorded as an
// answer by every peer, which then suppressed its own, and the requester
// dropped the digest from its in-flight set, never to retry it.
//
// The forgery is a genuine header, which is public, with junk signatures:
// a certificate's digest covers only its header, and the primary reports a
// certificate it holds before verifying it, so the forgery comes back as a
// duplicate at a peer that holds the certificate and as invalid at one that
// does not. The certificates go through the primary's receiveCertificate,
// as in production.
func TestForgedSyncResponseIsNotAnAnswer(t *testing.T) {
	v := newTestValidator(t)
	committee := newTestCommittee([]*testValidator{v}, 0)

	h := types.NewHeader(v.pub, 0, 0, nil, nil)
	require.NoError(t, h.Sign(v.priv))
	vote := types.NewVote(h.Digest(), h.Round, h.Epoch, v.pub)
	require.NoError(t, vote.Sign(v.priv))
	genuine := types.NewCertificate(h, [][]byte{vote.Signature}, []uint16{0})
	require.NoError(t, genuine.Verify(committee))
	forged := types.NewCertificate(h, [][]byte{make([]byte, ed25519.SignatureSize)}, []uint16{0})
	require.Error(t, forged.Verify(committee))
	require.Equal(t, genuine.Digest(), forged.Digest())

	// A primary and its syncer, wired as New wires them when gossip is set.
	newSyncer := func(partition string, key ed25519.PrivateKey) (*Primary, *CertSyncer) {
		p := New(Config{Partition: partition, KeyPair: key}, committee, nil, newTestDAG(), nil)
		config := CertSyncerConfig{PublicKey: key.Public().(ed25519.PublicKey), Partition: partition, RetryTimeout: time.Millisecond, DeduplicationInterval: time.Millisecond}
		config.applyDefaults()
		s := NewCertSyncer(config, p.dag, nil, p.pendingCerts)
		s.SetCertReceivedCallback(p.receiveCertificate)
		return p, s
	}
	requester := newTestValidator(t)
	const k = 11

	t.Run("a peer still answers the request", func(t *testing.T) {
		p, peer := newSyncer("t4597a", v.priv)
		require.NoError(t, p.dag.InsertGenesis(genuine))

		// The forgery arrives first, under the requester's key and ID.
		peer.handleSyncResponse(&gossip.CertSyncResponse{
			Certificates: []*types.Certificate{forged},
			RequestID:    k,
			Requester:    requester.pub,
		})
		peer.window.take()

		// Then the genuine request, for a certificate this peer holds.
		peer.handleSyncRequest(&gossip.CertSyncRequest{
			Digests:   []types.CertificateDigest{genuine.Digest()},
			Requester: requester.pub,
			RequestID: k,
		})
		require.Equal(t, 0, peer.window.take().suppressed, "the request was suppressed by the forged response")
		peer.answeredMu.Lock()
		_, answered := peer.answered[keyOf(requester.pub, k)]
		peer.answeredMu.Unlock()
		require.False(t, answered, "the forged response was recorded as an answer")

		// A genuine copy is an answer.
		peer.handleSyncResponse(&gossip.CertSyncResponse{
			Certificates: []*types.Certificate{genuine},
			RequestID:    k,
			Requester:    requester.pub,
		})
		peer.handleSyncRequest(&gossip.CertSyncRequest{
			Digests:   []types.CertificateDigest{genuine.Digest()},
			Requester: requester.pub,
			RequestID: k,
		})
		require.Equal(t, 1, peer.window.take().suppressed, "a genuine answer did not suppress the request")
	})

	t.Run("the requester keeps asking", func(t *testing.T) {
		_, s := newSyncer("t4597b", requester.priv)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		s.ctx = ctx

		// The digest is in flight under request k.
		s.inFlightMu.Lock()
		s.inFlight[genuine.Digest()] = &inFlightRequest{digest: genuine.Digest(), requestID: k, sentAt: time.Now().Add(-time.Second)}
		s.inFlightMu.Unlock()

		handled := make(chan types.CertificateDigest, 4)
		responses := make(chan *gossip.CertSyncResponse, 4)
		go func() {
			for resp := range responses {
				s.handleSyncResponse(resp)
				handled <- resp.Certificates[0].Digest()
			}
		}()

		// The forgery answers request k with the digest asked for.
		responses <- &gossip.CertSyncResponse{Certificates: []*types.Certificate{forged}, RequestID: k, Requester: requester.pub}
		<-handled
		require.False(t, s.dag.Contains(genuine.Digest()), "the forgery was inserted")

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
		close(responses)
	})

	t.Run("the request stays open for the genuine answer", func(t *testing.T) {
		_, s := newSyncer("t4597c", requester.priv)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		s.ctx = ctx

		responses := make(chan *gossip.CertSyncResponse, 4)
		go s.readSyncResponses(responses)
		responses <- &gossip.CertSyncResponse{Certificates: []*types.Certificate{forged}, RequestID: k, Requester: requester.pub}
		responses <- &gossip.CertSyncResponse{Certificates: []*types.Certificate{genuine}, RequestID: k, Requester: requester.pub}
		require.Eventually(t, func() bool { return s.dag.Contains(genuine.Digest()) }, 5*time.Second, 10*time.Millisecond,
			"the genuine answer was dropped: the forged response completed the request")
	})
}

// TestACopyIsADuplicateOnlyByteForByte: the primary reports a certificate
// whose digest is in the DAG without verifying it, and the digest covers
// only the header. So a copy is a duplicate only if its signatures and
// signers are the stored certificate's, byte for byte; any other copy is
// invalid (#4597). One rule, in the primary, for every receive path.
func TestACopyIsADuplicateOnlyByteForByte(t *testing.T) {
	v := newTestValidator(t)
	committee := newTestCommittee([]*testValidator{v}, 0)
	p := New(Config{Partition: "t4597d", KeyPair: v.priv}, committee, nil, newTestDAG(), nil)

	h := types.NewHeader(v.pub, 0, 0, nil, nil)
	require.NoError(t, h.Sign(v.priv))
	vote := types.NewVote(h.Digest(), h.Round, h.Epoch, v.pub)
	require.NoError(t, vote.Sign(v.priv))
	genuine := types.NewCertificate(h, [][]byte{vote.Signature}, []uint16{0})
	require.NoError(t, p.dag.InsertGenesis(genuine))

	copied := types.NewCertificate(h, [][]byte{append([]byte(nil), vote.Signature...)}, []uint16{0})
	require.Equal(t, CertDuplicate, p.receiveCertificate(copied), "a byte-for-byte copy")

	junk := types.NewCertificate(h, [][]byte{make([]byte, ed25519.SignatureSize)}, []uint16{0})
	require.Equal(t, CertInvalid, p.receiveCertificate(junk), "a copy with other signatures")
	otherSigner := types.NewCertificate(h, [][]byte{vote.Signature}, []uint16{1})
	require.Equal(t, CertInvalid, p.receiveCertificate(otherSigner), "a copy with other signers")
}

// TestAPartialAnswerSuppressesNothing: a response that reports some of the
// request missing leaves the request open for the next answer, and does
// not stop this peer from answering it either. Only a verified answer that
// covers the request counts (#4597).
func TestAPartialAnswerSuppressesNothing(t *testing.T) {
	v := newTestValidator(t)
	committee := newTestCommittee([]*testValidator{v}, 0)
	p := New(Config{Partition: "t4597e", KeyPair: v.priv}, committee, nil, newTestDAG(), nil)
	config := CertSyncerConfig{PublicKey: v.pub, Partition: "t4597e"}
	config.applyDefaults()
	peer := NewCertSyncer(config, p.dag, nil, p.pendingCerts)
	peer.SetCertReceivedCallback(p.receiveCertificate)

	h := types.NewHeader(v.pub, 0, 0, nil, nil)
	require.NoError(t, h.Sign(v.priv))
	vote := types.NewVote(h.Digest(), h.Round, h.Epoch, v.pub)
	require.NoError(t, vote.Sign(v.priv))
	genuine := types.NewCertificate(h, [][]byte{vote.Signature}, []uint16{0})
	require.NoError(t, p.dag.InsertGenesis(genuine))

	requester := []byte("the requester's key")
	const k = 12
	other := types.CertificateDigest{9}
	peer.handleSyncResponse(&gossip.CertSyncResponse{
		Certificates: []*types.Certificate{genuine},
		Missing:      []types.CertificateDigest{other},
		RequestID:    k,
		Requester:    requester,
	})
	peer.window.take()
	peer.handleSyncRequest(&gossip.CertSyncRequest{
		Digests:   []types.CertificateDigest{genuine.Digest(), other},
		Requester: requester,
		RequestID: k,
	})
	require.Equal(t, 0, peer.window.take().suppressed, "a partial answer suppressed the request")
}
