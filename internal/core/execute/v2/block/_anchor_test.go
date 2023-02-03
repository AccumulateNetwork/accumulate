// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
)

func TestValidatorSignature(t *testing.T) {
	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	dn := sim.Partition("Directory")
	bvn0 := sim.Partition("BVN0")
	bvn1 := sim.Partition("BVN1")
	bvn2 := sim.Partition("BVN2")

	helpers.View(t, dn, func(batch *database.Batch) {
		globals := dn.Globals()
		globals.Network.Version = 1
		signer := globals.AsSigner(protocol.Directory)
		sigset, err := batch.Transaction(make([]byte, 32)).SignaturesForSigner(signer)
		require.NoError(t, err)
		count, err := sigset.Add(0, &protocol.ED25519Signature{PublicKey: bvn0.Executor.Key[32:], Signer: signer.GetUrl(), SignerVersion: signer.GetVersion()})
		require.NoError(t, err)
		require.Equal(t, count, 1)
		count, err = sigset.Add(0, &protocol.ED25519Signature{PublicKey: bvn1.Executor.Key[32:], Signer: signer.GetUrl(), SignerVersion: signer.GetVersion()})
		require.NoError(t, err)
		require.Equal(t, count, 2)

		globals.Network.Version++
		signer = globals.AsSigner(protocol.Directory)
		sigset, err = batch.Transaction(make([]byte, 32)).SignaturesForSigner(signer)
		require.NoError(t, err)
		count, err = sigset.Add(0, &protocol.ED25519Signature{PublicKey: bvn1.Executor.Key[32:], Signer: signer.GetUrl(), SignerVersion: signer.GetVersion()})
		require.NoError(t, err)
		require.Equal(t, count, 2)
		count, err = sigset.Add(0, &protocol.ED25519Signature{PublicKey: bvn2.Executor.Key[32:], Signer: signer.GetUrl(), SignerVersion: signer.GetVersion()})
		require.NoError(t, err)
		require.Equal(t, count, 3)
	})
}
