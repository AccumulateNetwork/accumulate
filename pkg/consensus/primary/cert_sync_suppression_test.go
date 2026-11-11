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
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// A peer holding n round-0 certificates, its sync responses handled by the
// real receiveCertificate, as production wires it: a copy of a certificate
// the DAG holds is a duplicate before anything is verified, anything else
// verifies against the committee of four or is invalid. The first is the
// committee's genesis certificate; the rest are by keys outside the
// committee, held so there can be more than one answer carries.
func newSuppressionPeer(t *testing.T, n int) (*CertSyncer, []*types.Certificate) {
	t.Helper()
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	p := New(Config{Partition: "t4597", KeyPair: validators[0].priv}, committee, nil, d, nil)
	h := types.NewHeader(validators[0].pub, 0, committee.Epoch, nil, nil)
	require.NoError(t, h.Sign(validators[0].priv))
	sigs := make([][]byte, len(validators))
	idx := make([]uint16, len(validators))
	for i, v := range validators {
		vote := types.NewVote(h.Digest(), 0, committee.Epoch, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs[i], idx[i] = vote.Signature, uint16(i)
	}
	genesis := types.NewCertificate(h, sigs, idx)
	require.NoError(t, d.InsertGenesis(genesis))
	certs := []*types.Certificate{genesis}
	for len(certs) < n {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		h := types.NewHeader(pub, 0, 0, nil, nil)
		require.NoError(t, h.Sign(priv))
		c := types.NewCertificate(h, [][]byte{h.Signature}, []uint16{0})
		require.NoError(t, d.InsertGenesis(c))
		certs = append(certs, c)
	}

	config := CertSyncerConfig{PublicKey: validators[0].pub, Partition: "t4597"}
	peer := NewCertSyncer(config, d, nil, p.pendingCerts)
	peer.SetCertReceivedCallback(func(c *types.Certificate) CertOutcome { return p.receiveCertificate(c, false) })
	p.certSyncer = peer
	return peer, certs
}

// junkCertificate is one that checks nowhere: no signatures, an author
// outside any committee.
func junkCertificate() *types.Certificate {
	return types.NewCertificate(types.NewHeader(make([]byte, 32), 0, 0, nil, nil), nil, nil)
}

func digestsOf(certs []*types.Certificate) []types.CertificateDigest {
	var ds []types.CertificateDigest
	for _, c := range certs {
		ds = append(ds, c.Digest())
	}
	return ds
}

// One forged response must not silence the honest answers to a request
// (#4597): a response suppresses only the certificates it delivered that
// verified, so junk suppresses nothing and a partial answer suppresses only
// its part.
func TestSuppressionIsPerVerifiedCertificate(t *testing.T) {
	requester := []byte("the requester's key")
	const id = 77

	t.Run("junk suppresses nothing", func(t *testing.T) {
		peer, certs := newSuppressionPeer(t, 2)
		peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: []*types.Certificate{junkCertificate()}, Requester: requester, RequestID: id})
		peer.window.take()

		resp := peer.answer(&gossip.CertSyncRequest{Digests: digestsOf(certs), Requester: requester, RequestID: id})
		require.NotNil(t, resp, "the peer holding the certificates did not answer")
		require.Equal(t, digestsOf(certs), digestsOf(resp.Certificates))
		require.Equal(t, 0, peer.window.take().suppressed)
	})

	t.Run("partial answer suppresses its part", func(t *testing.T) {
		peer, certs := newSuppressionPeer(t, 2)
		peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: certs[:1], Missing: digestsOf(certs[1:]), Requester: requester, RequestID: id})

		resp := peer.answer(&gossip.CertSyncRequest{Digests: digestsOf(certs), Requester: requester, RequestID: id})
		require.NotNil(t, resp, "the certificate the response did not carry was not answered")
		require.Equal(t, digestsOf(certs[1:]), digestsOf(resp.Certificates))
	})

	t.Run("complete answer suppresses the request", func(t *testing.T) {
		peer, certs := newSuppressionPeer(t, 2)
		peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: certs, Requester: requester, RequestID: id})
		peer.window.take()

		require.Nil(t, peer.answer(&gossip.CertSyncRequest{Digests: digestsOf(certs), Requester: requester, RequestID: id}))
		require.Equal(t, 1, peer.window.take().suppressed)
	})

	t.Run("a complete round answer suppresses the request", func(t *testing.T) {
		// A round request names no digests, so an answer in which every
		// certificate checks reports nothing missing and completes the
		// request, whatever the peer holds beyond it.
		peer, certs := newSuppressionPeer(t, 2)
		require.True(t, peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: certs[1:], Requester: requester, RequestID: id}))
		peer.window.take()

		require.Nil(t, peer.answer(&gossip.CertSyncRequest{Rounds: []types.Round{0}, Requester: requester, RequestID: id}))
		require.Equal(t, 1, peer.window.take().suppressed)
	})

	t.Run("a mixed round answer suppresses only what checked", func(t *testing.T) {
		peer, certs := newSuppressionPeer(t, 3)
		mixed := append(append([]*types.Certificate{}, certs[1:]...), junkCertificate())
		require.False(t, peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: mixed, Requester: requester, RequestID: id}))

		resp := peer.answer(&gossip.CertSyncRequest{Rounds: []types.Round{0}, Requester: requester, RequestID: id})
		require.NotNil(t, resp, "a response that did not complete the request suppressed it")
		require.Equal(t, digestsOf(certs[:1]), digestsOf(resp.Certificates))
	})

	// A response completes a request once every certificate in it checks
	// and it reports nothing missing, whatever it carried: that is when
	// the requester stops reading answers. Whole dominates, in either
	// order. The peer holds a certificate the complete answer did not
	// carry, which a partial answer would leave it to send.
	complete := func(certs []*types.Certificate) *gossip.CertSyncResponse {
		return &gossip.CertSyncResponse{Certificates: certs[1:], Requester: requester, RequestID: id}
	}
	partial := func(certs []*types.Certificate) *gossip.CertSyncResponse {
		return &gossip.CertSyncResponse{Certificates: certs[1:2], Missing: digestsOf(certs[:1]), Requester: requester, RequestID: id}
	}
	round := &gossip.CertSyncRequest{Rounds: []types.Round{0}, Requester: requester, RequestID: id}

	t.Run("complete then partial stays suppressed", func(t *testing.T) {
		peer, certs := newSuppressionPeer(t, 3)
		require.True(t, peer.handleSyncResponse(complete(certs)))
		peer.handleSyncResponse(partial(certs))
		peer.window.take()

		require.Nil(t, peer.answer(round), "a partial answer after a complete one reopened the request")
		require.Equal(t, 1, peer.window.take().suppressed)
	})

	t.Run("partial then complete becomes suppressed", func(t *testing.T) {
		peer, certs := newSuppressionPeer(t, 3)
		peer.handleSyncResponse(partial(certs))
		require.True(t, peer.handleSyncResponse(complete(certs)))
		peer.window.take()

		require.Nil(t, peer.answer(round), "a complete answer after a partial one did not suppress the request")
		require.Equal(t, 1, peer.window.take().suppressed)
	})
}

// #4597, #4057: a round request names no digests, so an honest answer to
// it reports nothing missing, and the first one in which every
// certificate checks completes the request: the requester drops every
// later answer unread (readSyncResponses). The peers must stop there too,
// or each of them broadcasts up to a hundred certificates the requester
// discards, the flood #4057 added suppression against. The rounds asked
// for hold more certificates than one answer carries; the next second's
// round request pulls the next hundred.
func TestACompleteRoundAnswerSuppressesTheLaterPeers(t *testing.T) {
	requester := []byte("the joining node's key")
	const id = 4057
	peer, _ := newSuppressionPeer(t, 250)
	req := &gossip.CertSyncRequest{Rounds: []types.Round{0}, Requester: requester, RequestID: id}

	// What the first peer sends: capped, every certificate checks, nothing
	// missing. It completes the request at the requester.
	first := peer.answer(req)
	require.NotNil(t, first)
	require.Len(t, first.Certificates, gossip.MaxSyncCertificates)
	require.True(t, peer.handleSyncResponse(first))
	require.Empty(t, first.Missing)
	peer.window.take()

	// Peers 2 to 12, having seen it, send nothing.
	for i := 2; i <= 12; i++ {
		require.Nil(t, peer.answer(req), "peer %d answered a request the first answer completed", i)
	}
	require.Equal(t, 11, peer.window.take().suppressed)
}

// #4597, through the real receive path: a certificate's digest covers only
// its header, less the header's signature, and receiveCertificate reports a
// copy of a certificate the DAG holds as a duplicate before verifying it. A
// forged response carrying a genuine header with no signatures, or the
// genuine certificate under a junk header signature, must therefore not
// count as having delivered the certificate, or one message silences every
// honest answer. A copy whose wire encoding is the certificate's does count.
func TestDefect_ForgedSyncResponseSuppressesHonestAnswer(t *testing.T) {
	requester := []byte("the requester's key")
	const id = 4597
	peer, certs := newSuppressionPeer(t, 1)
	genuine := certs[0]
	request := &gossip.CertSyncRequest{Digests: digestsOf(certs), Requester: requester, RequestID: id}
	wireCopy := func() *types.Certificate {
		data, err := genuine.Marshal()
		require.NoError(t, err)
		c, err := types.UnmarshalCertificate(data)
		require.NoError(t, err)
		return c
	}

	forged := types.NewCertificate(genuine.Header, nil, nil)
	peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: []*types.Certificate{forged}, Requester: requester, RequestID: id})
	require.NotNil(t, peer.answer(request), "a header with no signatures suppressed the honest answer")

	badHeaderSig := wireCopy()
	badHeaderSig.Header.Signature = make([]byte, len(genuine.Header.Signature))
	require.Equal(t, genuine.Digest(), badHeaderSig.Digest(), "the digest does not cover the header signature")
	peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: []*types.Certificate{badHeaderSig}, Requester: requester, RequestID: id})
	require.NotNil(t, peer.answer(request), "a copy under a junk header signature suppressed the honest answer")

	peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: []*types.Certificate{wireCopy()}, Requester: requester, RequestID: id})
	require.Nil(t, peer.answer(request), "an identical copy of the certificate did not suppress the answer")
}

// #4597: responses are handled on one goroutine and requests answered on
// others, so what a response delivered is read while the next response to
// the same request is recorded. A concurrent map read and write kills the
// process.
func TestSuppressionMapIsRaceFree(t *testing.T) {
	requester := []byte("the requester's key")
	const id = 11
	peer, certs := newSuppressionPeer(t, 8)
	const rounds = 2000

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < rounds; i++ {
			c := certs[i%len(certs)]
			peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: []*types.Certificate{c}, Requester: requester, RequestID: id})
		}
	}()
	for i := 0; i < rounds; i++ {
		peer.answer(&gossip.CertSyncRequest{Rounds: []types.Round{0}, Requester: requester, RequestID: id})
	}
	<-done
}

// #4597: what a response delivered is taken out before the answer is cut to
// what one response carries, or the certificates past the cap are never
// served. The peer holds one certificate more than a response carries, and
// a response that did not complete the request delivered all but one of
// them.
func TestFilteringPrecedesTheCap(t *testing.T) {
	requester := []byte("the requester's key")
	peer, certs := newSuppressionPeer(t, gossip.MaxSyncCertificates+1)
	withheld, delivered := certs[len(certs)-1], certs[:len(certs)-1]

	// By digest, the withheld certificate named last: the digest loop
	// used to stop at the cap before reaching it. The response reported
	// the withheld one missing, so it did not complete the request.
	const byDigest = 101
	peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: delivered, Missing: digestsOf([]*types.Certificate{withheld}), Requester: requester, RequestID: byDigest})
	resp := peer.answer(&gossip.CertSyncRequest{Digests: digestsOf(certs), Requester: requester, RequestID: byDigest})
	require.NotNil(t, resp, "the certificate no response delivered was not answered")
	require.Equal(t, digestsOf([]*types.Certificate{withheld}), digestsOf(resp.Certificates))

	// By round, the same. A round answer reports nothing missing, so one
	// that did not complete the request carried a certificate that did
	// not check.
	const byRound = 102
	mixed := append(append([]*types.Certificate{}, delivered...), junkCertificate())
	require.False(t, peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: mixed, Requester: requester, RequestID: byRound}))
	resp = peer.answer(&gossip.CertSyncRequest{Rounds: []types.Round{0}, Requester: requester, RequestID: byRound})
	require.NotNil(t, resp, "the certificate no response delivered was not answered")
	require.Equal(t, digestsOf([]*types.Certificate{withheld}), digestsOf(resp.Certificates))
}

// #4597: what is remembered per request is bounded by what one response
// carries, so one requester and ID cannot be filled with the whole DAG.
// Delivering one certificate more than that under one key, in a response
// that does not complete the request, leaves at least one of them
// unsuppressed.
func TestDeliveredDigestsAreCappedPerRequest(t *testing.T) {
	requester := []byte("the requester's key")
	const id = 5
	peer, certs := newSuppressionPeer(t, gossip.MaxSyncCertificates+1)
	mixed := append(append([]*types.Certificate{}, certs...), junkCertificate())
	require.False(t, peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: mixed, Requester: requester, RequestID: id}))

	resp := peer.answer(&gossip.CertSyncRequest{Rounds: []types.Round{0}, Requester: requester, RequestID: id})
	require.NotNil(t, resp, "one request remembered more than a response carries")
	require.Len(t, resp.Certificates, 1)
}

// #4597: suppression ages out a generation at a time, never all at once.
// After one request more than a generation holds, the oldest request and
// the ones recorded just before the rotation are all still suppressed.
func TestSuppressionIsNeverWipedAllAtOnce(t *testing.T) {
	peer, certs := newSuppressionPeer(t, 1)
	requester := []byte("the requester's key")
	deliver := func(id uint64) {
		peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: certs, Requester: requester, RequestID: id})
	}
	suppressed := func(id uint64) bool {
		return peer.answer(&gossip.CertSyncRequest{Digests: digestsOf(certs), Requester: requester, RequestID: id}) == nil
	}

	const generation = 4096
	for id := uint64(0); id <= generation+1; id++ {
		deliver(id)
	}
	for _, id := range []uint64{0, 1, generation, generation + 1} {
		require.True(t, suppressed(id), "request %d was forgotten when the generation rotated", id)
	}
}
