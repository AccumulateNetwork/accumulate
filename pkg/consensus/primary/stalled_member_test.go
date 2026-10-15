// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// TestAStalledMemberWhoseHeaderIsCertifiedIsStillHeard pins #4581: a live
// member at a round that does not complete must keep sending something its
// peers count (RoundStatus.Authors, HeardWithin). Once its own header at the
// round is certified it has no uncertified header to rebroadcast, and when it
// cannot author the next round it sent nothing a peer counts, so a partition
// that lost two of four reported 1 of 4 producing, not 2. While the round is
// stalled it sends its certified header again; while the round is healthy it
// does not.
func TestAStalledMemberWhoseHeaderIsCertifiedIsStillHeard(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	newPrimary := func(v *testValidator) *Primary {
		d := newTestDAG()
		createGenesisCertificates(t, validators, committee, d)
		p := New(Config{Partition: "test", KeyPair: v.priv}, committee, nil, d, nil)
		p.SetRound(1)
		return p
	}
	a, b := newPrimary(validators[0]), newPrimary(validators[1])

	// a's header at round 1 is certified; the round cannot complete.
	header := types.NewHeader(validators[0].pub, 1, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))
	a.pendingMu.Lock()
	a.ourCerts[1] = types.NewCertificate(header, nil, nil)
	a.pendingMu.Unlock()

	stalledFor := func(d time.Duration) {
		a.roundMu.Lock()
		a.lastRoundAdvance = time.Now().Add(-d)
		a.roundMu.Unlock()
	}

	stalledFor(time.Second)
	require.Empty(t, a.headersToRebroadcast(), "a healthy round sends no certified header again")

	stalledFor(2 * roundStallAfter)
	sent := a.headersToRebroadcast()
	require.Len(t, sent, 1, "a stalled member sends its certified header again")
	require.Equal(t, header.Digest(), sent[0].Digest(), "the same header, not a second one for the round")

	b.OnHeaderReceived(sent[0])
	st := b.RoundStatus()
	require.Equal(t, 2, st.Authors, "the peer counts the stalled member: %+v", st)
}
