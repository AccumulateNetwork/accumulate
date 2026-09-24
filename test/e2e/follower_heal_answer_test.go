// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// #4424. A follower answered a healing pull for an anchor
// with a record signed by its own key, which no destination accepts; the
// requester turned every signature in the answer into a BlockAnchor in one
// envelope (requestAnchorSpan), and the executor refused the whole envelope
// on the first bad message — the quorum's good signatures with it.
//
// Run 20260924T093936Z: 141 refusals `key is not an active validator for
// Directory|BVN3`, 140 of them within 3 s of the refusing container's own
// `Requested missing anchors` from that same source; 0 for BVN1/BVN2.
//
// This test failed on issue-4205-lead d52ec1e00: the follower's answer
// carried its own signature, and the requester's envelope was refused. A node
// in no committee now signs no answer (sequencer_cache.go, signsAnswers); the
// requester's own filter is pinned in crosschain's
// TestRequesterKeepsOnlyCommitteeSignatures.
func TestAFollowerAnswersNoAnchorWithItsOwnKey(t *testing.T) {
	net, key := networkWithAFollower(t.Name(), 1, 3)
	sim := NewSim(t,
		simulator.WithNetwork(net),
		simulator.Genesis(GenesisTime),
	)
	sim.StepN(50)

	var bvn string
	for _, p := range sim.S.Partitions() {
		if p.Type == protocol.PartitionTypeBlockValidator {
			bvn = p.ID
		}
	}
	dn := sim.S.Partition(protocol.Directory)
	fol := -1
	for i := 0; i < dn.NodeCount(); i++ {
		if bytes.Equal(dn.NodeConductor(i).ValidatorKey, key) {
			fol = i
		}
	}
	require.GreaterOrEqual(t, fol, 0, "no Directory node holds the follower's key")

	// The premise: the follower's key is in no committee.
	def := sim.NetworkStatus(api.NetworkStatusOptions{Partition: protocol.Directory}).Network
	_, info, ok := def.ValidatorByKey(key[32:])
	require.True(t, ok)
	require.False(t, info.IsActiveOn(protocol.Directory))

	// Ask the follower's Directory node for DN anchor #1 to the BVN, as
	// anchorAnswers does when the transport (or its per-peer loop) reaches it.
	src := protocol.DnUrl().JoinPath(protocol.AnchorPool)
	dst := protocol.PartitionUrl(bvn)
	r, err := dn.NodePrivate(fol).Sequence(context.Background(), src, dst, 1, private.SequenceOptions{})
	require.NoError(t, err, "the follower refused to answer — premise changed")

	// What requestAnchorSpan builds: one BlockAnchor per signature, one
	// envelope.
	env := new(messaging.Envelope)
	var signers [][]byte
	for _, set := range r.Signatures.Records {
		for _, s := range set.Signatures.Records {
			ks := s.Message.(*messaging.SignatureMessage).Signature.(protocol.KeySignature)
			signers = append(signers, ks.GetPublicKey())
			env.Messages = append(env.Messages, &messaging.BlockAnchor{Anchor: r.Sequence, Signature: ks})
		}
	}
	t.Logf("follower's answer carries %d signatures", len(signers))

	signedByFollower := false
	for _, k := range signers {
		if bytes.Equal(k, key.Public().(ed25519.PublicKey)) {
			signedByFollower = true
		}
	}

	// The destination's submit-time validation, as ExecutorBridge does it:
	// any status with an error refuses the envelope.
	firstError := func(env *messaging.Envelope) error {
		statuses, err := sim.S.Partition(bvn).NodeExecutor(0).Validate(env, false)
		require.NoError(t, err)
		for _, st := range statuses {
			if st.Error != nil {
				return st.Error
			}
		}
		return nil
	}

	// Control: the same envelope minus the follower's copy is accepted, so
	// the follower's signature is the whole of the refusal.
	control := new(messaging.Envelope)
	for _, m := range env.Messages {
		if !bytes.Equal(m.(*messaging.BlockAnchor).Signature.GetPublicKey(), key.Public().(ed25519.PublicKey)) {
			control.Messages = append(control.Messages, m)
		}
	}
	require.NotEmpty(t, control.Messages, "the answer carries no validator signature — nothing to compare")
	require.NoError(t, firstError(control), "the quorum's own copies are refused — a different defect")

	refused := firstError(env)
	t.Logf("signed by the follower: %v; envelope refused: %v", signedByFollower, refused)
	require.False(t, signedByFollower,
		"the follower signed a healing answer with a key no destination accepts")
	require.NoError(t, refused,
		"the heal envelope built from the follower's answer was refused whole")
}
