// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestClassifyRemoteError pins the breaker semantics fixed after run
// 20260820T073651Z: notFound ("reached the end of the chain") and notReady
// ("anchor N commits below entry M") are a HEALTHY remote giving a correct
// answer — the requested entry is not servable yet. Counting them as remote
// failures opened the shared per-remote breaker, and synthetic-heal notReady
// answers then blocked ANCHOR healing to the same remote (which would have
// succeeded) — BVN1 anchor delivery trickled at 2 per 20 minutes.
func TestClassifyRemoteError(t *testing.T) {
	t.Run("deterministic cannot-serve-yet answers never open the circuit", func(t *testing.T) {
		c := new(Conductor)
		notFound := errors.NotFound.WithFormat("locate anchor index chain entry for block 111: reached the end of the chain")
		notReady := errors.NotReady.WithFormat("anchor 55 commits to root chain entry 327, which is below 1756")

		for i := 0; i < breakerThreshold*10; i++ {
			c.classifyRemoteError("remote", notFound)
			c.classifyRemoteError("remote", notReady)
		}
		require.True(t, c.remoteAllowed("remote"),
			"a remote that answers, even with cannot-serve-yet, is healthy — the circuit must stay closed")
	})

	t.Run("wrapped cannot-serve-yet answers are still recognized", func(t *testing.T) {
		c := new(Conductor)
		wrapped := errors.UnknownError.WithFormat("request synthetic %d: %w", 151,
			errors.NotFound.WithFormat("reached the end of the chain"))
		for i := 0; i < breakerThreshold*3; i++ {
			c.classifyRemoteError("remote", wrapped)
		}
		require.True(t, c.remoteAllowed("remote"))
	})

	t.Run("transport-class errors open the circuit at the threshold", func(t *testing.T) {
		c := new(Conductor)
		dial := errors.NoPeer.WithFormat("no live peers for query:bvn2")
		for i := 0; i < breakerThreshold-1; i++ {
			c.classifyRemoteError("remote", dial)
			require.True(t, c.remoteAllowed("remote"), "below the threshold the circuit stays closed")
		}
		c.classifyRemoteError("remote", dial)
		require.False(t, c.remoteAllowed("remote"), "at the threshold the circuit opens")
	})

	t.Run("a cannot-serve-yet answer closes a partly-charged circuit", func(t *testing.T) {
		c := new(Conductor)
		dial := errors.NoPeer.WithFormat("no live peers")
		c.classifyRemoteError("remote", dial)
		c.classifyRemoteError("remote", dial)
		// The remote answers — it is alive; the failure streak resets.
		c.classifyRemoteError("remote", errors.NotReady.With("not servable yet"))
		c.classifyRemoteError("remote", dial)
		c.classifyRemoteError("remote", dial)
		require.True(t, c.remoteAllowed("remote"),
			"an answering remote must reset the consecutive-failure count")
	})
}

// TestAnchorRecoverySourceAllowed pins the recovery scope fixed after run
// 20260820T100912Z: anchor streams exist BVN<->DN only. A BVN conductor
// "recovering anchors from" another BVN pulled that BVN's ->dn anchors and
// submitted them into its OWN partition's DAG, where they committed as
// wrong-partition noise.
func TestAnchorRecoverySourceAllowed(t *testing.T) {
	bvn := &Conductor{Partition: &protocol.PartitionInfo{
		ID: "BVN3", Type: protocol.PartitionTypeBlockValidator}}
	require.True(t, bvn.anchorRecoverySourceAllowed(protocol.Directory),
		"a BVN's only anchor source is the Directory")
	require.False(t, bvn.anchorRecoverySourceAllowed("BVN1"),
		"there is no BVN->BVN anchor stream to recover")
	require.False(t, bvn.anchorRecoverySourceAllowed("BVN3"),
		"never recover from self")

	dn := &Conductor{Partition: &protocol.PartitionInfo{
		ID: protocol.Directory, Type: protocol.PartitionTypeDirectory}}
	require.True(t, dn.anchorRecoverySourceAllowed("BVN1"))
	require.True(t, dn.anchorRecoverySourceAllowed("BVN2"))
	require.False(t, dn.anchorRecoverySourceAllowed(protocol.Directory),
		"never recover from self")
}
