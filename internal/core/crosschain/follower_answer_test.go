// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	nodeconfig "gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// withAStraySignature answers as the wrapped sequencer does, plus one
// signature per record by a key the requester's partition does not accept —
// what a follower, a validator removed from the committee, or a peer with a
// stale definition hands back.
type withAStraySignature struct {
	private.SequenceRanger
	key ed25519.PrivateKey
	t   *testing.T
}

func (w withAStraySignature) SequenceRange(ctx context.Context, src, dst *url.URL, start, end uint64, opts private.SequenceOptions) ([]*api.MessageRecord[messaging.Message], error) {
	records, err := w.SequenceRanger.SequenceRange(ctx, src, dst, start, end, opts)
	if err != nil {
		return nil, err
	}
	for _, r := range records {
		h := r.Sequence.Hash()
		sig, err := new(signing.Builder).
			SetType(protocol.SignatureTypeED25519).
			SetPrivateKey(w.key).
			SetUrl(r.Sequence.Source.JoinPath(protocol.Network)).
			SetVersion(1).
			SetTimestampToNow().
			Sign(h[:])
		require.NoError(w.t, err)
		sm := &messaging.SignatureMessage{Signature: sig, TxID: r.ID}
		r.Signatures.Records = append(r.Signatures.Records, &api.SignatureSetRecord{
			Account:    &protocol.UnknownAccount{Url: sig.GetSigner()},
			Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{Total: 1, Records: []*api.MessageRecord[messaging.Message]{{ID: sm.ID(), Message: sm}}},
		})
		r.Signatures.Total++
	}
	return records, nil
}

// An answer carrying one signature by a key that is not active on the
// source partition yields a heal envelope the destination accepts: the
// requester keeps only committee signatures (#4424). Before, every signature
// became a BlockAnchor in one envelope and the destination's submit
// validation refused the whole envelope — the quorum's good copies with the
// stray one — "key is not an active validator for Directory" (run
// 20260924T093936Z, 141 refusals).
//
// Production wiring throughout: a simulator network with a follower in its
// definition, the BVN's own conductor building the envelope from a real
// Directory validator's answer, and the BVN's executor validating it as the
// submit path does. Only the stray signature is added by hand; it is the
// input under test.
func TestRequesterKeepsOnlyCommitteeSignatures(t *testing.T) {
	name := t.Name()
	net := simulator.NewSimpleNetwork(name, 1, 3)
	follower := acctesting.GenerateKey(name, "follower")
	net.Bvns[0].Nodes = append(net.Bvns[0].Nodes, &accumulated.NodeInit{
		DnnType:    nodeconfig.Follower,
		BvnnType:   nodeconfig.Follower,
		PrivValKey: follower,
		DnNodeKey:  acctesting.GenerateKey(name, "follower", "dn"),
		BvnNodeKey: acctesting.GenerateKey(name, "follower", "bvn"),
	})
	sim := NewSim(t, simulator.WithNetwork(net), simulator.Genesis(GenesisTime))
	sim.StepN(50)

	var bvn string
	for _, p := range sim.S.Partitions() {
		if p.Type == protocol.PartitionTypeBlockValidator {
			bvn = p.ID
		}
	}
	dn := sim.S.Partition(protocol.Directory)
	val := -1
	for i := 0; i < dn.NodeCount(); i++ {
		if !bytes.Equal(dn.NodeConductor(i).ValidatorKey, follower) {
			val = i
			break
		}
	}
	require.GreaterOrEqual(t, val, 0)
	ranger, ok := dn.NodePrivate(val).(private.SequenceRanger)
	require.True(t, ok)
	stray := withAStraySignature{SequenceRanger: ranger, key: follower, t: t}

	// Find an anchor the validator still holds.
	src := protocol.DnUrl().JoinPath(protocol.AnchorPool)
	dst := protocol.PartitionUrl(bvn)
	var num uint64
	for n := uint64(1); n < 40; n++ {
		if rs, err := ranger.SequenceRange(context.Background(), src, dst, n, n, private.SequenceOptions{}); err == nil && len(rs) == 1 {
			num = n
			break
		}
	}
	require.NotZero(t, num, "the validator holds no anchor for %s", bvn)

	var envelopes []*messaging.Envelope
	_, served, err := sim.S.Partition(bvn).NodeConductor(0).HealAnchorSpan(context.Background(), stray, protocol.DnUrl(), num, num, func(env *messaging.Envelope) {
		envelopes = append(envelopes, env)
	})
	require.NoError(t, err)
	require.Equal(t, num, served)
	require.Len(t, envelopes, 1)

	statuses, err := sim.S.Partition(bvn).NodeExecutor(0).Validate(envelopes[0], false)
	require.NoError(t, err)
	for _, st := range statuses {
		require.NoError(t, st.AsError(), "the heal envelope was refused")
	}

	followerPub := follower.Public().(ed25519.PublicKey)
	for _, m := range envelopes[0].Messages {
		a := m.(*messaging.BlockAnchor)
		require.False(t, bytes.Equal(a.Signature.GetPublicKey(), followerPub), "the envelope carries the stray signature")
	}
}

// A follower that holds a synthetic entry serves it, and the bundle the
// requester builds from its answer is accepted at the destination: the
// collection proof is the authorization whatever the signer
// (msg_synthetic.go, #4056), so only an ANCHOR answer needs a committee
// signature (#4424, review note_3897460300). The wire still requires a
// signature — an unsigned copy is refused "missing signature" — so the
// follower's answer carries its own.
//
// Production wiring: a simulator network whose first BVN has a follower,
// a real deposit from that BVN to another, the follower's own sequencer
// answering, the destination's conductor building the bundle
// (requestSpanTo), and the destination's executor validating it.
func TestAFollowerServesASyntheticItHolds(t *testing.T) {
	name := t.Name()
	net := simulator.NewSimpleNetwork(name, 2, 3)
	follower := acctesting.GenerateKey(name, "follower")
	net.Bvns[0].Nodes = append(net.Bvns[0].Nodes, &accumulated.NodeInit{
		DnnType:    nodeconfig.Follower,
		BvnnType:   nodeconfig.Follower,
		PrivValKey: follower,
		DnNodeKey:  acctesting.GenerateKey(name, "follower", "dn"),
		BvnNodeKey: acctesting.GenerateKey(name, "follower", "bvn"),
	})
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = protocol.ExecutorVersionLatest
	sim := NewSim(t, simulator.WithNetwork(net), simulator.GenesisWith(GenesisTime, globals))

	srcID := net.Bvns[0].Id
	src := sim.S.Partition(srcID)
	fol := -1
	for i := 0; i < src.NodeCount(); i++ {
		if bytes.Equal(src.NodeConductor(i).ValidatorKey, follower) {
			fol = i
		}
	}
	require.GreaterOrEqual(t, fol, 0, "no node of %s holds the follower's key", srcID)

	// Alice on the follower's BVN, Bob on the other.
	var alice ed25519.PrivateKey
	var aliceUrl, bobUrl *url.URL
	for i := 0; ; i++ {
		alice = acctesting.GenerateKey("Alice", i)
		aliceUrl = acctesting.AcmeLiteAddressStdPriv(alice)
		if p, err := sim.Router().RouteAccount(aliceUrl); err == nil && p == srcID {
			break
		}
	}
	var dstID string
	for i := 0; ; i++ {
		bobUrl = acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("Bob", i))
		p, err := sim.Router().RouteAccount(bobUrl)
		require.NoError(t, err)
		if p != srcID && p != protocol.Directory {
			dstID = p
			break
		}
	}
	helpers.MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], protocol.AcmeUrl())
	var timestamp uint64
	st := sim.SubmitTxnSuccessfully(helpers.MustBuild(t,
		build.Transaction().For(aliceUrl).
			SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
			SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
	sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())

	// Ask the follower, as the transport may, until its answer is out of
	// the in-flight window.
	ranger, ok := src.NodePrivate(fol).(private.SequenceRanger)
	require.True(t, ok)
	dst := sim.S.Partition(dstID)
	var envelopes []*messaging.Envelope
	var err error
	for i := 0; i < 200; i++ {
		envelopes = nil
		_, _, err = dst.NodeConductor(0).HealSpan(context.Background(), ranger, protocol.PartitionUrl(srcID), 1, 1, func(env *messaging.Envelope) {
			envelopes = append(envelopes, env)
		})
		if err == nil {
			break
		}
		sim.Step()
	}
	require.NoError(t, err, "the follower did not serve the synthetic it holds")
	require.Len(t, envelopes, 1)

	var syn *messaging.SyntheticMessage
	for _, m := range envelopes[0].Messages {
		if s, ok := m.(*messaging.SyntheticMessage); ok {
			syn = s
		}
	}
	require.NotNil(t, syn, "the bundle carries no synthetic")
	require.Equal(t, []byte(follower.Public().(ed25519.PublicKey)), syn.Signature.GetPublicKey(), "the answer is the follower's")

	validate := func(env *messaging.Envelope) error {
		statuses, err := dst.NodeExecutor(0).Validate(env, false)
		require.NoError(t, err)
		for _, st := range statuses {
			if err := st.AsError(); err != nil {
				return err
			}
		}
		return nil
	}
	require.NoError(t, validate(envelopes[0]), "the follower's proven synthetic was refused")

	// Control: the same bundle unsigned is refused. "Unsigned but proven"
	// does not exist on the wire.
	syn.Signature = nil
	require.ErrorContains(t, validate(envelopes[0]), "missing signature")
}
