// Copyright 2025 The Accumulate Authors
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

// A peer holding two round-0 certificates, with the verifying callback
// production wires in: a certificate by a known author verifies and is
// inserted, anything else is invalid.
func newSuppressionPeer(t *testing.T) (*CertSyncer, []*types.Certificate) {
	t.Helper()
	d := dag.NewDAG(10)
	var certs []*types.Certificate
	known := map[types.CertificateDigest]bool{}
	for i := 0; i < 2; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		h := types.NewHeader(pub, 0, 0, nil, nil)
		require.NoError(t, h.Sign(priv))
		c := types.NewCertificate(h, [][]byte{h.Signature}, []uint16{0})
		require.NoError(t, d.InsertGenesis(c))
		certs = append(certs, c)
		known[c.Digest()] = true
	}
	config := CertSyncerConfig{PublicKey: make([]byte, 32), Partition: "t4597"}
	config.applyDefaults()
	peer := NewCertSyncer(config, d, nil, NewPendingCertificates(100))
	peer.SetCertReceivedCallback(func(c *types.Certificate) CertOutcome {
		if known[c.Digest()] {
			return CertDuplicate
		}
		return CertInvalid
	})
	return peer, certs
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
		peer, certs := newSuppressionPeer(t)
		junk := types.NewCertificate(types.NewHeader(make([]byte, 32), 0, 0, nil, nil), nil, nil)
		peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: []*types.Certificate{junk}, Requester: requester, RequestID: id})
		peer.window.take()

		resp := peer.answer(&gossip.CertSyncRequest{Digests: digestsOf(certs), Requester: requester, RequestID: id})
		require.NotNil(t, resp, "the peer holding the certificates did not answer")
		require.Equal(t, digestsOf(certs), digestsOf(resp.Certificates))
		require.Equal(t, 0, peer.window.take().suppressed)
	})

	t.Run("partial answer suppresses its part", func(t *testing.T) {
		peer, certs := newSuppressionPeer(t)
		peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: certs[:1], Requester: requester, RequestID: id})

		resp := peer.answer(&gossip.CertSyncRequest{Digests: digestsOf(certs), Requester: requester, RequestID: id})
		require.NotNil(t, resp, "the certificate the response did not carry was not answered")
		require.Equal(t, digestsOf(certs[1:]), digestsOf(resp.Certificates))
	})

	t.Run("complete answer suppresses the request", func(t *testing.T) {
		peer, certs := newSuppressionPeer(t)
		peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: certs, Requester: requester, RequestID: id})
		peer.window.take()

		require.Nil(t, peer.answer(&gossip.CertSyncRequest{Digests: digestsOf(certs), Requester: requester, RequestID: id}))
		require.Equal(t, 1, peer.window.take().suppressed)
	})

	t.Run("round request answers what was not delivered", func(t *testing.T) {
		peer, certs := newSuppressionPeer(t)
		peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: certs[1:], Requester: requester, RequestID: id})

		resp := peer.answer(&gossip.CertSyncRequest{Rounds: []types.Round{0}, Requester: requester, RequestID: id})
		require.NotNil(t, resp)
		require.Equal(t, digestsOf(certs[:1]), digestsOf(resp.Certificates))
	})
}
