// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// Producing a block writes nothing on the certificates of its group. They are
// the DAG's own, and gossip marshals them on other goroutines while the block
// is produced -- a stalled round's re-share, a catch-up answer. The block's
// state hash used to be stamped on each one, a write the race detector caught
// beside those marshals in the libp2p join test (#4610), on a field the
// consensus spec says nothing reads and is zero on every certificate the
// network forms. The hash is the node's record for the round.
func TestProducingABlockWritesNothingOnItsCertificates(t *testing.T) {
	svc, _, author := newCommitService(t, 1)
	w := svc.node.Workers()[0]

	b := types.NewBatch([][]byte{[]byte("tx")})
	require.NoError(t, w.StoreBatch(b))
	cert := commitCert(author, 2, time.Unix(100, 0), []types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})
	before, err := cert.Marshal()
	require.NoError(t, err)

	_, err = svc.processCommittedGroup(group(cert))
	require.NoError(t, err)

	after, err := cert.Marshal()
	require.NoError(t, err)
	require.Equal(t, before, after, "producing the block wrote on a certificate the DAG holds")
	hash, ok := svc.stateHashTracker.GetLocalHash(2)
	require.True(t, ok, "the block's state hash was not recorded for its round")
	require.Equal(t, types.StateHash{0xAA}, hash)
}
