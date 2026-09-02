// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

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
