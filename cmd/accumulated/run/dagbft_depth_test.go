// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A DAG shallower than the minimum is refused at configuration: below it
// Bullshark stops ordering on a healthy node (#4595).
func TestDAGBFT_RefusesADAGBelowTheMinimumDepth(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	svc := &DAGBFTService{
		Partition:    &protocol.PartitionInfo{ID: protocol.Directory, Type: protocol.PartitionTypeDirectory},
		ValidatorKey: &RawPrivateKey{Address: address.FromED25519PrivateKey(key).String()},
	}
	require.NoError(t, svc.Verify())

	depth := int64(types.MinDAGGCDepth - 1)
	svc.DAGGCDepth = &depth
	require.Error(t, svc.Verify())

	depth++
	require.NoError(t, svc.Verify())
}
