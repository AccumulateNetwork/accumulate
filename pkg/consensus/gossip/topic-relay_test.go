// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	bhost "github.com/libp2p/go-libp2p/p2p/host/blank"
	swarmt "github.com/libp2p/go-libp2p/p2p/net/swarm/testing"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// newCommittee returns a committee of n validators of equal stake and their keys.
func newCommittee(t *testing.T, n int) (*types.Committee, []ed25519.PrivateKey) {
	t.Helper()
	var keys []ed25519.PrivateKey
	infos := make([]types.ValidatorInfo, n)
	for i := range infos {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		keys = append(keys, priv)
		infos[i] = types.ValidatorInfo{PublicKey: pub, Stake: 1}
	}
	return types.NewCommittee(infos, 0), keys
}

func signedHeader(t *testing.T, key ed25519.PrivateKey, round types.Round, epoch uint64) *types.Header {
	t.Helper()
	// A header above round 0 names parents (#4591); which ones is the
	// DAG's concern, not the router's.
	var parents []types.CertificateDigest
	if round > 0 {
		parents = []types.CertificateDigest{{1}}
	}
	h := types.NewHeader(key.Public().(ed25519.PublicKey), round, epoch, nil, parents)
	require.NoError(t, h.Sign(key))
	return h
}

func signedVote(t *testing.T, key ed25519.PrivateKey, h *types.Header) *types.Vote {
	t.Helper()
	v := types.NewVote(h.Digest(), h.Round, h.Epoch, key.Public().(ed25519.PublicKey))
	require.NoError(t, v.Sign(key))
	return v
}

// quorumCert certifies h with a vote from every key.
func quorumCert(t *testing.T, c *types.Committee, keys []ed25519.PrivateKey, h *types.Header) *types.Certificate {
	t.Helper()
	var sigs [][]byte
	var idx []uint16
	for _, key := range keys {
		i, ok := c.FindValidator(key.Public().(ed25519.PublicKey))
		require.True(t, ok)
		sigs = append(sigs, signedVote(t, key, h).Signature)
		idx = append(idx, i)
	}
	return types.NewCertificate(h, sigs, idx)
}

func startLayer(ctx context.Context, t *testing.T, h host.Host, ps *pubsub.PubSub, c *types.Committee) *gossip.GossipLayer {
	t.Helper()
	g, err := gossip.NewGossipLayer(h, ps, "test")
	require.NoError(t, err)
	g.SetCommittee(c)
	require.NoError(t, g.Start(ctx))
	t.Cleanup(func() { g.Close() })
	return g
}

// A peer that holds no committee key is connected to validator A only, and
// validator C to A only, so what the peer publishes reaches C only if A's
// router relays it. A header, vote or certificate authored by a key outside
// the committee must reach neither; one authored by committee members must
// reach both, whoever publishes it.
func TestTopicValidation_StrangerMessagesAreNotRelayed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	committee, keys := newCommittee(t, 2)

	newHost := func() (host.Host, *pubsub.PubSub) {
		h := bhost.NewBlankHost(swarmt.GenSwarm(t))
		t.Cleanup(func() { h.Close() })
		ps, err := pubsub.NewGossipSub(ctx, h)
		require.NoError(t, err)
		return h, ps
	}
	hA, psA := newHost()
	hC, psC := newHost()
	hS, psS := newHost()
	for _, h := range []host.Host{hS, hC} {
		require.NoError(t, h.Connect(ctx, peer.AddrInfo{ID: hA.ID(), Addrs: hA.Addrs()}))
	}

	layers := make(chan *gossip.GossipLayer, 2)
	for _, n := range []struct {
		h  host.Host
		ps *pubsub.PubSub
	}{{hA, psA}, {hC, psC}} {
		n := n
		go func() { layers <- startLayer(ctx, t, n.h, n.ps, committee) }()
	}
	validators := []*gossip.GossipLayer{<-layers, <-layers}
	require.Empty(t, hC.Network().ConnsToPeer(hS.ID()), "C must not be connected to the stranger")

	// The stranger publishes on the raw topics and runs no validators.
	topics := map[string]*pubsub.Topic{}
	for _, pattern := range []string{gossip.TopicHeaders, gossip.TopicVotes, gossip.TopicCerts} {
		topic, err := psS.Join(fmt.Sprintf(pattern, "test"))
		require.NoError(t, err)
		topics[pattern] = topic
	}
	publish := func(pattern string, m interface{ Marshal() ([]byte, error) }) {
		data, err := m.Marshal()
		require.NoError(t, err)
		require.NoError(t, topics[pattern].Publish(ctx, data))
	}

	_, strangerKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	badHeader := signedHeader(t, strangerKey, 7, committee.Epoch)
	badVote := signedVote(t, strangerKey, badHeader)
	badCert := quorumCert(t, committee, keys, signedHeader(t, strangerKey, 8, committee.Epoch))

	goodHeader := signedHeader(t, keys[0], 7, committee.Epoch)
	goodVote := signedVote(t, keys[1], goodHeader)
	goodCert := quorumCert(t, committee, keys, goodHeader)

	var bad, good [2][3]int
	reached := func() bool {
		for i := range good {
			for _, n := range good[i] {
				if n == 0 {
					return false
				}
			}
		}
		return true
	}
	deadline := time.After(30 * time.Second)
	for !reached() {
		publish(gossip.TopicHeaders, badHeader)
		publish(gossip.TopicVotes, badVote)
		publish(gossip.TopicCerts, badCert)
		publish(gossip.TopicHeaders, goodHeader)
		publish(gossip.TopicVotes, goodVote)
		publish(gossip.TopicCerts, goodCert)

		round := time.After(300 * time.Millisecond)
	collect:
		for {
			for i, g := range validators {
				select {
				case h := <-g.SubscribeHeaders():
					if h.Digest() == goodHeader.Digest() {
						good[i][0]++
					} else {
						bad[i][0]++
					}
					continue collect
				case v := <-g.SubscribeVotes():
					if v.VoteDigest() == goodVote.VoteDigest() {
						good[i][1]++
					} else {
						bad[i][1]++
					}
					continue collect
				case c := <-g.SubscribeCertificates():
					if c.Digest() == goodCert.Digest() {
						good[i][2]++
					} else {
						bad[i][2]++
					}
					continue collect
				default:
				}
			}
			select {
			case <-round:
				break collect
			case <-deadline:
				t.Fatalf("committee messages (header, vote, certificate) did not reach A and C: %v", good)
			case <-time.After(10 * time.Millisecond):
			}
		}
	}

	require.Equal(t, [2][3]int{}, bad, "stranger messages (header, vote, certificate) delivered to A and C")
}
