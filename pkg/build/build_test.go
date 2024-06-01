// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package build_test

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestBuild64Byte(t *testing.T) {
	var timestamp uint64
	key1 := acctesting.GenerateKey(1)
	key2 := acctesting.GenerateKey(2)

	env := MustBuild(t,
		build.Transaction().For("staking.acme", "registered").
			WriteData("foo").
			SignWith("staking.acme", "book", "2").Version(1).Timestamp(&timestamp).PrivateKey(key1).
			SignWith("staking.acme", "book", "2").Version(1).Timestamp(&timestamp).PrivateKey(key2))

	// Verify there's one transaction and two signatures
	require.Len(t, env.Transaction, 1)
	require.Len(t, env.Signatures, 2)

	// Verify the header has not been zero-padded
	b, err := env.Transaction[0].Header.MarshalBinary()
	require.NoError(t, err)
	require.Len(t, b, 64)
	// require.Equal(t, byte(0), b[64])

	// Verify the signatures match the transaction
	for i, sig := range env.Signatures {
		assert.Equal(t, env.Transaction[0].ID().Hash(), sig.GetTransactionHash(), "Signature %d should match", i)
	}
}

func TestLoad(t *testing.T) {
	t.Skip("Manual")

	seed := record.NewKey("ci").Hash()
	key := ed25519.NewKeyFromSeed(seed[:])
	lite, err := protocol.LiteTokenAddress(key[32:], "ACME", protocol.SignatureTypeED25519)
	require.NoError(t, err)

	var envs []*messaging.Envelope
	ts := uint64(time.Now().UTC().UnixMilli())
	for i := 0; i < 100; i++ {
		env, err := build.Transaction().For(lite).
			SendTokens(1, 0).To(lite).
			SignWith(lite).Version(1).Timestamp(&ts).PrivateKey(key).
			Done()
		require.NoError(t, err)
		envs = append(envs, env)
	}

	c := jsonrpc.NewClient("http://127.0.1.2:26760/v3")
	for _, env := range envs {
		subs, err := c.Submit(context.Background(), env, api.SubmitOptions{})
		require.NoError(t, err)
		for _, sub := range subs {
			require.NoError(t, sub.Status.AsError(), sub.Message)
		}
	}
}
