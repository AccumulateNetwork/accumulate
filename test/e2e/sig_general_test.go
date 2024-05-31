// Copyright 2024 The Accumulate Authors
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
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestSignatureMemo(t *testing.T) {
	// Tests AIP-006
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

	// Submit
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "book", "1").
			BurnCredits(1).
			SignWith(alice, "book", "1").
			Version(1).Timestamp(1).
			Memo("foo").
			Metadata("bar").
			PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st[0].TxID).Succeeds())

	// Verify the signature
	sig := sim.QueryMessage(st[1].TxID, nil).Message.(*messaging.SignatureMessage).Signature.(*protocol.ED25519Signature)
	require.Equal(t, "foo", sig.Memo)
	require.Equal(t, "bar", string(sig.Data))
}

func generateTestPkiCertificates() (string, string, string, error) {
	certTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "example.com",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Generate RSA private key
	rsaPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate RSA key: %v", err)
	}
	rsaCertBytes, err := x509.CreateCertificate(rand.Reader, &certTemplate, &certTemplate, &rsaPrivKey.PublicKey, rsaPrivKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create RSA certificate: %v", err)
	}
	rsaCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rsaCertBytes})
	rsaPrivKeyPEM, err := x509.MarshalPKCS8PrivateKey(rsaPrivKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to marshal RSA private key: %v", err)
	}
	rsaPrivKeyPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: rsaPrivKeyPEM})

	// Generate ECDSA private key
	ecdsaPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate ECDSA key: %v", err)
	}
	ecdsaCertBytes, err := x509.CreateCertificate(rand.Reader, &certTemplate, &certTemplate, &ecdsaPrivKey.PublicKey, ecdsaPrivKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create ECDSA certificate: %v", err)
	}
	ecdsaCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ecdsaCertBytes})
	ecdsaPrivKeyPEM, err := x509.MarshalPKCS8PrivateKey(ecdsaPrivKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to marshal ECDSA private key: %v", err)
	}
	ecdsaPrivKeyPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecdsaPrivKeyPEM})

	// Generate Ed25519 private key
	_, ed25519PrivKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate Ed25519 key: %v", err)
	}
	ed25519CertBytes, err := x509.CreateCertificate(rand.Reader, &certTemplate, &certTemplate, ed25519PrivKey.Public().(ed25519.PublicKey), ed25519PrivKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create Ed25519 certificate: %v", err)
	}
	ed25519CertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ed25519CertBytes})
	ed25519PrivKeyPEM, err := x509.MarshalPKCS8PrivateKey(ed25519PrivKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to marshal Ed25519 private key: %v", err)
	}
	ed25519PrivKeyPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ed25519PrivKeyPEM})

	return string(rsaCertPEM) + string(rsaPrivKeyPEMBytes), string(ecdsaCertPEM) + string(ecdsaPrivKeyPEMBytes), string(ed25519CertPEM) + string(ed25519PrivKeyPEMBytes), nil
}

func TestSigTypeFromCerts(t *testing.T) {
	rsaCert, ecdsaCert, ed25519Cert, err := generateTestPkiCertificates()
	if err != nil {
		t.Fatalf("Failed to generate certificates: %v\n", err)
	}

	alice := AccountUrl("alice")
	alicePubKey := &address.PublicKey{}

	cases := map[string][]struct {
		Version ExecutorVersion
		Ok      bool
	}{
		rsaCert: {
			{ExecutorVersionV2Baikonur, false},
			{ExecutorVersionLatest, true}},
		ecdsaCert: {
			{ExecutorVersionV2Baikonur, false},
			{ExecutorVersionLatest, true}},
		ed25519Cert: {
			{ExecutorVersionV2Baikonur, true},
			{ExecutorVersionLatest, true}},
	}

	for k, c := range cases {
		block, rest := pem.Decode([]byte(k))
		if block.Type == "CERTIFICATE" {
			block, _ = pem.Decode(rest)
		}
		require.Equal(t, block.Type, "PRIVATE KEY")
		aliceKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		require.NoError(t, err)
		switch pk := aliceKey.(type) {
		case *rsa.PrivateKey:
			alicePubKey = address.FromRSAPublicKey(&pk.PublicKey)
		case *ecdsa.PrivateKey:
			alicePubKey = address.FromEcdsaPublicKeyAsPKIX(&pk.PublicKey)
		case ed25519.PrivateKey:
			alicePubKey = address.FromED25519PublicKey(pk.Public().(ed25519.PublicKey))
		}

		for _, ca := range c {
			t.Run(ca.Version.String(), func(t *testing.T) {
				// Initialize
				sim := NewSim(t,
					simulator.SimpleNetwork(t.Name(), 3, 1),
					simulator.GenesisWithVersion(GenesisTime, ca.Version),
				)

				MakeIdentity(t, sim.DatabaseFor(alice), alice, alicePubKey.Key)
				CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

				// Submit
				env := build.Transaction().For(alice, "book", "1").
					BurnCredits(1).
					SignWith(alice, "book", "1").
					Version(1).Timestamp(1).
					Memo("foo").
					Metadata("bar").
					Type(alicePubKey.GetType()).
					PrivateKey(aliceKey)

				if ca.Ok {
					st := sim.BuildAndSubmitSuccessfully(env)
					sim.StepUntil(
						Txn(st[0].TxID).Succeeds())
				} else {
					st := sim.BuildAndSubmit(env)
					require.Error(t, st[1].AsError())
					require.ErrorContains(t, st[1].Error, "unsupported signature type")
				}
			})
		}
	}
}

func TestVoteTypes(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	bob := AccountUrl("bob")
	bobKey := acctesting.GenerateKey(bob)

	t.Run("Pre-Vandenberg", func(t *testing.T) {
		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 3, 1),
			simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2Baikonur),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

		MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
		CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

		// Submit
		st := sim.BuildAndSubmit(
			build.Transaction().For(alice, "book", "1").
				BurnCredits(1).
				SignWith(alice, "book", "1").
				Suggest().
				Version(1).Timestamp(1).
				PrivateKey(aliceKey))
		require.Error(t, st[1].AsError())
		require.EqualError(t, st[1].Error, "initial signature cannot be a suggest vote")
	})

	t.Run("Suggestathon pre-Vandenberg", func(t *testing.T) {
		// Pre-Vandenberg, what happens if everyone suggests?

		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 3, 1),
			simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2Baikonur),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

		MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
		CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

		// Initiate with bob, acts like a suggestion
		st := sim.BuildAndSubmit(
			build.Transaction().For(alice, "book", "1").
				BurnCredits(1).
				SignWith(bob, "book", "1").
				Version(1).Timestamp(1).
				PrivateKey(bobKey))

		sim.StepUntil(
			Sig(st[1].TxID).CreditPayment().Completes(),
			Txn(st[0].TxID).IsPending())

		// Suggest with Alice
		st = sim.BuildAndSubmitSuccessfully(
			build.SignatureForTxID(st[0].TxID).
				Url(alice, "book", "1").
				Suggest().
				Version(1).Timestamp(1).
				PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st[0].TxID).Fails().WithError(errors.Rejected))
	})

	t.Run("Bob suggests", func(t *testing.T) {
		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 1),
			simulator.Genesis(GenesisTime),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

		MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
		CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

		// Suggest
		st := sim.BuildAndSubmitSuccessfully(
			build.Transaction().For(alice, "book", "1").
				BurnCredits(1).
				SignWith(bob, "book", "1").
				Suggest().
				Version(1).Timestamp(1).
				PrivateKey(bobKey))

		sim.StepUntil(
			Sig(st[1].TxID).CreditPayment().Completes(),
			Txn(st[0].TxID).IsPending())

		// Sign
		sim.BuildAndSubmitSuccessfully(
			build.SignatureForTxID(st[0].TxID).
				Url(alice, "book", "1").
				Accept().
				Version(1).Timestamp(1).
				PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st[0].TxID).Succeeds())
	})

	t.Run("Suggesting does not execute", func(t *testing.T) {
		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 1),
			simulator.Genesis(GenesisTime),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

		// Suggest
		st := sim.BuildAndSubmitSuccessfully(
			build.Transaction().For(alice, "book", "1").
				BurnCredits(1).
				SignWith(alice, "book", "1").
				Suggest().
				Version(1).Timestamp(1).
				PrivateKey(aliceKey))

		sim.StepUntil(
			Sig(st[1].TxID).CreditPayment().Completes(),
			Txn(st[0].TxID).IsPending())

		// Verify the signature set is empty
		r := sim.QueryTransaction(st[0].TxID, nil)
		for _, sig := range r.Signatures.Records {
			for _, sig := range sig.Signatures.Records {
				_, ok := sig.Message.(*messaging.SignatureMessage)
				require.False(t, ok && !sig.Historical, "Should not have any active signatures")
			}
		}

	})

	t.Run("No secondary suggestions", func(t *testing.T) {
		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 1),
			simulator.Genesis(GenesisTime),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

		MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
		CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

		// Suggest with Bob
		st := sim.BuildAndSubmitSuccessfully(
			build.Transaction().For(alice, "book", "1").
				BurnCredits(1).
				SignWith(bob, "book", "1").
				Suggest().
				Version(1).Timestamp(1).
				PrivateKey(bobKey))

		sim.StepUntil(
			Sig(st[1].TxID).CreditPayment().Completes(),
			Txn(st[0].TxID).IsPending())

		// Suggest with Alice
		st = sim.BuildAndSubmit(
			build.SignatureForTxID(st[0].TxID).
				Url(alice, "book", "1").
				Suggest().
				Version(1).Timestamp(1).
				PrivateKey(aliceKey))
		require.Error(t, st[1].AsError())
		require.EqualError(t, st[1].Error, "suggestions cannot be secondary signatures")
	})

	t.Run("No merkle suggestions", func(t *testing.T) {
		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 1),
			simulator.Genesis(GenesisTime),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

		MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
		CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

		// Suggest
		bld := build.Transaction().For(alice, "book", "1").
			BurnCredits(1).
			SignWith(bob, "book", "1").
			Suggest().
			Version(1).Timestamp(1).
			PrivateKey(bobKey)
		bld.InitMerkle = true //nolint:staticcheck // This is explicitly testing the deprecated hash method
		st := sim.BuildAndSubmit(bld)
		require.Error(t, st[1].AsError())
		require.EqualError(t, st[1].Error, "suggestions cannot use Merkle initiator hashes")
	})

	t.Run("Cannot initiate with rejection", func(t *testing.T) {
		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 1),
			simulator.Genesis(GenesisTime),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

		// Initiate with rejection
		st := sim.BuildAndSubmit(
			build.Transaction().For(alice, "book", "1").
				BurnCredits(1).
				SignWith(alice, "book", "1").
				Reject().
				Version(1).Timestamp(1).
				PrivateKey(aliceKey))
		require.Error(t, st[1].AsError())
		require.EqualError(t, st[1].Error, "initial signature cannot be a reject vote")
	})
}

func TestSignatureErrors(t *testing.T) {
	// Tests https://gitlab.com/accumulatenetwork/accumulate/-/issues/3578

	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
		simulator.UseABCI(),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])

	// Submit
	_, err := sim.SubmitRaw(MustBuild(t,
		build.Transaction().For(alice, "book", "1").
			SendTokens(1, 0).To("foo").
			SignWith(alice, "book", "1").
			Version(1).Timestamp(1).
			Memo("foo").
			Metadata("bar").
			PrivateKey(aliceKey)))
	require.Error(t, err)
	require.ErrorContains(t, err, "insufficient credits: have 0.00, want 0.01")
}
