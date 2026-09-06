// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// #4218: a lite identity derived from an ECDSA (P-256) key could validate and
// submit an AddCredits and have nothing happen. The ECDSA signature record is
// 261 bytes, one byte past the fee schedule's 256-byte chunk, so it carried
// an oversize surcharge of 0.01 credits that the purchase did not waive; an
// identity with no credits failed at the debit, after validation had said
// ok. Now the purchase waives the whole signature fee (Kourou), and before
// that a signer that cannot pay the surcharge is refused at validation.
func TestLiteIdentityEcdsaSigner(t *testing.T) {
	ecdsaLite := func(t *testing.T) (*url.URL, *ecdsa.PrivateKey) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		pkix, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		require.NoError(t, err)
		return protocol.LiteAuthorityForKey(pkix, protocol.SignatureTypeEcdsaSha256), key
	}

	t.Run("ed25519 control", func(t *testing.T) {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		lite := protocol.LiteAuthorityForKey(pub, protocol.SignatureTypeED25519)
		sim, st := liteBuysCredits(t, ExecutorVersionLatest, lite, priv)
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
		requireCredits(t, sim, lite)
	})

	t.Run("ecdsa p256 buys its first credits", func(t *testing.T) {
		lite, key := ecdsaLite(t)
		sim, st := liteBuysCredits(t, ExecutorVersionLatest, lite, key)
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
		requireCredits(t, sim, lite)
	})

	t.Run("before Kourou the refusal is at validation", func(t *testing.T) {
		lite, key := ecdsaLite(t)
		sim, _ := liteBuysCredits(t, ExecutorVersionV2Jiuquan, lite, key)
		_ = sim
	})
}

func liteBuysCredits(t *testing.T, version ExecutorVersion, lite *url.URL, key any) (*Sim, *protocol.TransactionStatus) {
	liteAcme := lite.JoinPath(AcmeUrl().ShortString())
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = version
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, globals),
	)

	// What the faucet leaves behind: the identity with no credits, the token
	// account with ACME
	MakeAccount(t, sim.DatabaseFor(lite), &protocol.LiteIdentity{Url: lite})
	MakeAccount(t, sim.DatabaseFor(lite), &protocol.LiteTokenAccount{Url: liteAcme, TokenUrl: AcmeUrl(), Balance: *big.NewInt(5e9)})

	env := MustBuild(t, build.Transaction().For(liteAcme).
		AddCredits().Spend(10).To(lite).WithOracle(InitialAcmeOracle).
		SignWith(lite).Version(1).Timestamp(1).PrivateKey(key))
	if version.V2KourouEnabled() {
		return sim, sim.SubmitTxnSuccessfully(env)
	}

	// The signature is refused up front, with the reason
	sts := sim.Submit(env)
	var refused bool
	for _, st := range sts {
		if st.Error != nil {
			require.Equal(t, errors.InsufficientCredits, st.Code, "%v", st.Error)
			require.Contains(t, st.Error.Message, "oversize signature")
			refused = true
		}
	}
	require.True(t, refused, "an unfunded ECDSA lite identity must be refused at validation, not dropped at execution")
	return sim, nil
}

func requireCredits(t *testing.T, sim *Sim, lite *url.URL) {
	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		var li *protocol.LiteIdentity
		require.NoError(t, batch.Account(lite).Main().GetAs(&li))
		require.NotZero(t, li.CreditBalance, "the lite identity received no credits")
	})
}
