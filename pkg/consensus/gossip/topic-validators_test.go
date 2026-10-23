// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

type testCommittee struct {
	keys      []ed25519.PrivateKey
	committee *types.Committee
}

func newTestCommittee(t *testing.T, n int, epoch uint64) *testCommittee {
	t.Helper()
	tc := &testCommittee{}
	infos := make([]types.ValidatorInfo, n)
	for i := range infos {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		tc.keys = append(tc.keys, priv)
		infos[i] = types.ValidatorInfo{PublicKey: pub, Stake: 1}
	}
	tc.committee = types.NewCommittee(infos, epoch)
	return tc
}

func (tc *testCommittee) header(t *testing.T, key ed25519.PrivateKey, round types.Round) *types.Header {
	t.Helper()
	// A header above round 0 names parents (#4591); which ones is the
	// DAG's concern, not the router's.
	var parents []types.CertificateDigest
	if round > 0 {
		parents = []types.CertificateDigest{{1}}
	}
	h := types.NewHeader(key.Public().(ed25519.PublicKey), round, tc.committee.Epoch, nil, parents)
	require.NoError(t, h.Sign(key))
	return h
}

func (tc *testCommittee) vote(t *testing.T, key ed25519.PrivateKey, h *types.Header) *types.Vote {
	t.Helper()
	v := types.NewVote(h.Digest(), h.Round, h.Epoch, key.Public().(ed25519.PublicKey))
	require.NoError(t, v.Sign(key))
	return v
}

// cert builds a certificate for h signed by every committee member.
func (tc *testCommittee) cert(t *testing.T, h *types.Header) *types.Certificate {
	t.Helper()
	var sigs [][]byte
	var idx []uint16
	for _, key := range tc.keys {
		i, ok := tc.committee.FindValidator(key.Public().(ed25519.PublicKey))
		require.True(t, ok)
		sigs = append(sigs, tc.vote(t, key, h).Signature)
		idx = append(idx, i)
	}
	return types.NewCertificate(h, sigs, idx)
}

func stranger(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return priv
}

func message(data []byte) *pubsub.Message {
	return &pubsub.Message{Message: &pb.Message{Data: data}}
}

func mustMarshal(t *testing.T, m interface{ Marshal() ([]byte, error) }) []byte {
	t.Helper()
	data, err := m.Marshal()
	require.NoError(t, err)
	return data
}

func syncMessage(t *testing.T, kind byte, m interface{ Marshal() ([]byte, error) }) []byte {
	return append([]byte{kind}, mustMarshal(t, m)...)
}

func TestValidateHeader(t *testing.T) {
	tc := newTestCommittee(t, 4, 3)
	g := &GossipLayer{}
	g.SetCommittee(tc.committee)
	ctx := context.Background()

	valid := tc.header(t, tc.keys[0], 5)
	msg := message(mustMarshal(t, valid))
	require.Equal(t, pubsub.ValidationAccept, g.validateHeader(ctx, "", msg))
	require.Equal(t, valid.Digest(), msg.ValidatorData.(*types.Header).Digest())

	notMember := tc.header(t, stranger(t), 5)
	require.Equal(t, pubsub.ValidationReject, g.validateHeader(ctx, "", message(mustMarshal(t, notMember))))

	badSig := tc.header(t, tc.keys[1], 5)
	badSig.Signature[0] ^= 1
	require.Equal(t, pubsub.ValidationReject, g.validateHeader(ctx, "", message(mustMarshal(t, badSig))))

	unknownEpoch := types.NewHeader(tc.keys[0].Public().(ed25519.PublicKey), 5, 9, nil, nil)
	require.NoError(t, unknownEpoch.Sign(tc.keys[0]))
	require.Equal(t, pubsub.ValidationIgnore, g.validateHeader(ctx, "", message(mustMarshal(t, unknownEpoch))))

	require.Equal(t, pubsub.ValidationReject, g.validateHeader(ctx, "", message([]byte{1, 2, 3})))
}

func TestValidateVote(t *testing.T) {
	tc := newTestCommittee(t, 4, 3)
	g := &GossipLayer{}
	g.SetCommittee(tc.committee)
	ctx := context.Background()
	h := tc.header(t, tc.keys[0], 5)

	msg := message(mustMarshal(t, tc.vote(t, tc.keys[1], h)))
	require.Equal(t, pubsub.ValidationAccept, g.validateVote(ctx, "", msg))
	require.IsType(t, &types.Vote{}, msg.ValidatorData)

	require.Equal(t, pubsub.ValidationReject, g.validateVote(ctx, "", message(mustMarshal(t, tc.vote(t, stranger(t), h)))))

	badSig := tc.vote(t, tc.keys[2], h)
	badSig.Signature[0] ^= 1
	require.Equal(t, pubsub.ValidationReject, g.validateVote(ctx, "", message(mustMarshal(t, badSig))))
}

func TestValidateCertificate(t *testing.T) {
	tc := newTestCommittee(t, 4, 3)
	g := &GossipLayer{}
	g.SetCommittee(tc.committee)
	ctx := context.Background()

	msg := message(mustMarshal(t, tc.cert(t, tc.header(t, tc.keys[0], 5))))
	require.Equal(t, pubsub.ValidationAccept, g.validateCertificate(ctx, "", msg))
	require.IsType(t, &types.Certificate{}, msg.ValidatorData)

	// A stranger's header carrying the committee's signatures.
	require.Equal(t, pubsub.ValidationReject, g.validateCertificate(ctx, "", message(mustMarshal(t, tc.cert(t, tc.header(t, stranger(t), 5))))))

	// A committee member's header with one vote signature: no quorum.
	h := tc.header(t, tc.keys[0], 6)
	short := types.NewCertificate(h, [][]byte{tc.vote(t, tc.keys[0], h).Signature}, []uint16{0})
	require.Equal(t, pubsub.ValidationReject, g.validateCertificate(ctx, "", message(mustMarshal(t, short))))

	forged := tc.cert(t, tc.header(t, tc.keys[1], 7))
	forged.Signatures[2][0] ^= 1
	require.Equal(t, pubsub.ValidationReject, g.validateCertificate(ctx, "", message(mustMarshal(t, forged))))
}

func TestValidateBatch(t *testing.T) {
	g := &GossipLayer{}
	ctx := context.Background()

	msg := message(mustMarshal(t, types.NewBatch([][]byte{[]byte("tx")})))
	require.Equal(t, pubsub.ValidationAccept, g.validateBatch(ctx, "", msg))
	require.IsType(t, &types.Batch{}, msg.ValidatorData)

	require.Equal(t, pubsub.ValidationReject, g.validateBatch(ctx, "", message(mustMarshal(t, types.NewBatch(nil)))))
	require.Equal(t, pubsub.ValidationReject, g.validateBatch(ctx, "", message([]byte{0, 0, 0, 2, 0})))
}

func TestValidateCertSync(t *testing.T) {
	tc := newTestCommittee(t, 4, 3)
	g := &GossipLayer{}
	g.SetCommittee(tc.committee)
	ctx := context.Background()

	// A request is anyone's: a joining node is not in the committee.
	req := &CertSyncRequest{Requester: stranger(t).Public().(ed25519.PublicKey), RequestID: 1, Rounds: []types.Round{4}}
	msg := message(syncMessage(t, 0x01, req))
	require.Equal(t, pubsub.ValidationAccept, g.validateCertSync(ctx, "", msg))
	require.IsType(t, &CertSyncRequest{}, msg.ValidatorData)

	resp := &CertSyncResponse{Certificates: []*types.Certificate{tc.cert(t, tc.header(t, tc.keys[0], 5))}, RequestID: 1}
	msg = message(syncMessage(t, 0x02, resp))
	require.Equal(t, pubsub.ValidationAccept, g.validateCertSync(ctx, "", msg))
	require.IsType(t, &CertSyncResponse{}, msg.ValidatorData)

	resp.Certificates = append(resp.Certificates, tc.cert(t, tc.header(t, stranger(t), 5)))
	require.Equal(t, pubsub.ValidationReject, g.validateCertSync(ctx, "", message(syncMessage(t, 0x02, resp))))

	require.Equal(t, pubsub.ValidationReject, g.validateCertSync(ctx, "", message([]byte{0x07})))
	require.Equal(t, pubsub.ValidationReject, g.validateCertSync(ctx, "", message(nil)))
}

// Around a committee switch, messages of the previous epoch are still judged
// by the previous committee; a validator that left is accepted only for it.
func TestValidateAcrossCommitteeSwitch(t *testing.T) {
	old := newTestCommittee(t, 4, 3)
	next := newTestCommittee(t, 4, 4)
	g := &GossipLayer{}
	ctx := context.Background()

	h := old.header(t, old.keys[0], 5)
	require.Equal(t, pubsub.ValidationIgnore, g.validateHeader(ctx, "", message(mustMarshal(t, h))), "no committee yet")

	g.SetCommittee(old.committee)
	g.SetCommittee(next.committee)
	require.Equal(t, pubsub.ValidationAccept, g.validateHeader(ctx, "", message(mustMarshal(t, h))))
	require.Equal(t, pubsub.ValidationAccept, g.validateHeader(ctx, "", message(mustMarshal(t, next.header(t, next.keys[0], 6)))))

	// An old member signing for the new epoch is not a member of it.
	moved := types.NewHeader(old.keys[0].Public().(ed25519.PublicKey), 6, next.committee.Epoch, nil, nil)
	require.NoError(t, moved.Sign(old.keys[0]))
	require.Equal(t, pubsub.ValidationReject, g.validateHeader(ctx, "", message(mustMarshal(t, moved))))

	// Two switches on, the oldest committee is forgotten.
	g.SetCommittee(newTestCommittee(t, 4, 5).committee)
	require.Equal(t, pubsub.ValidationIgnore, g.validateHeader(ctx, "", message(mustMarshal(t, h))))
}

// A copy, byte for byte, of a certificate the node holds is accepted on the
// verification that admitted it. A stalled node re-shares every certificate
// of its round and the one before as new messages, and each copy cost every
// receiver a full verification before the handler found it a duplicate:
// #4057 again, at the router. A copy with other signatures is verified as
// any certificate is.
func TestACopyOfAHeldCertificateIsNotVerifiedAgain(t *testing.T) {
	tc := newTestCommittee(t, 4, 3)
	g := &GossipLayer{}
	g.SetCommittee(tc.committee)
	ctx := context.Background()

	// The held certificate's signatures are nonsense, so only the held-copy
	// rule can accept a message that matches it.
	h := tc.header(t, tc.keys[0], 5)
	sigs := func(last byte) [][]byte {
		return [][]byte{bytes.Repeat([]byte{1}, 64), bytes.Repeat([]byte{2}, 64), bytes.Repeat([]byte{last}, 64)}
	}
	held := types.NewCertificate(h, sigs(3), []uint16{0, 1, 2})
	g.SetHeldCertificate(func(d types.CertificateDigest) *types.Certificate {
		if d == held.Digest() {
			return held
		}
		return nil
	})

	msg := message(mustMarshal(t, held))
	require.Equal(t, pubsub.ValidationAccept, g.validateCertificate(ctx, "", msg))
	require.IsType(t, &types.Certificate{}, msg.ValidatorData)

	// The same header with other signatures is not the held certificate.
	other := types.NewCertificate(h, sigs(4), []uint16{0, 1, 2})
	require.Equal(t, pubsub.ValidationReject, g.validateCertificate(ctx, "", message(mustMarshal(t, other))))

	// Nor is it with the header's signature zeroed or flipped: the digest
	// leaves the header signature out, so only the wire encoding tells.
	resigned := func(sig []byte) *types.Header {
		h2, err := types.UnmarshalHeader(mustMarshal(t, h))
		require.NoError(t, err)
		h2.Signature = sig
		require.Equal(t, h.Digest(), h2.Digest())
		return h2
	}
	unsigned := types.NewCertificate(resigned(make([]byte, ed25519.SignatureSize)), sigs(3), []uint16{0, 1, 2})
	require.Equal(t, pubsub.ValidationReject, g.validateCertificate(ctx, "", message(mustMarshal(t, unsigned))), "the header signature zeroed")
	flipped := append([]byte(nil), h.Signature...)
	flipped[0] ^= 1
	require.Equal(t, pubsub.ValidationReject, g.validateCertificate(ctx, "", message(mustMarshal(t, types.NewCertificate(resigned(flipped), sigs(3), []uint16{0, 1, 2})))), "the header signature flipped")

	// A copy that lacks the state hash the holder set after execution is
	// still the held certificate.
	held.SetStateHash(types.StateHash{7})
	bare := types.NewCertificate(h, sigs(3), []uint16{0, 1, 2})
	require.Equal(t, pubsub.ValidationAccept, g.validateCertificate(ctx, "", message(mustMarshal(t, bare))), "a copy without the state hash")

	// With nothing held the copy is verified, and its signatures fail.
	g.SetHeldCertificate(nil)
	require.Equal(t, pubsub.ValidationReject, g.validateCertificate(ctx, "", message(mustMarshal(t, held))))
}
