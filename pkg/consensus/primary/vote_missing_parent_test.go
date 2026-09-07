// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// #4182: a header whose parent certificate this node does not hold must be
// ASKED for, not merely deferred.
//
// The author rebroadcasts its header about once a second, and each rebroadcast
// re-tests the same condition. A certificate that was missed once is never
// re-sent on its own, so the condition can only become true if somebody asks.
// Without the ask, every validator of a partition sits on the round's headers
// refusing to vote, no header reaches quorum, the round cannot advance, and
// the partition stops while its peers run on — all four validators frozen at
// one round with votes still arriving and nothing going out.
//
// The batch gate immediately below this one in the vote path has always
// fetched what it lacks, for the same reason. This is that gate's twin.
func TestVote_MissingParentIsRequestedNotJustDeferred(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, nil)

	// A parent this node has never seen.
	absent := types.CertificateDigest{9, 9, 9}
	require.Nil(t, d.GetByDigest(absent), "the parent must be absent for the test to mean anything")

	// A header from another validator that names it.
	header := types.NewHeader(validators[1].pub, 1, 1, nil, []types.CertificateDigest{absent})
	require.NoError(t, header.Sign(validators[1].priv))

	// Record what the vote path asks for.
	var asked []types.CertificateDigest
	p.requestParents = func(d []types.CertificateDigest) { asked = append(asked, d...) }

	p.OnHeaderReceived(header)

	// It must not vote — it cannot certify a header whose history it lacks.
	p.pendingMu.Lock()
	_, voted := p.sentVotes[header.Digest()]
	p.pendingMu.Unlock()
	require.False(t, voted, "must not vote on a header whose parent is missing")

	// And it must have asked for the parent.
	require.Equal(t, []types.CertificateDigest{absent}, asked,
		"a missing parent must be requested, or the round can never advance")
}
