// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package trustbundle

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"strings"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type testValidator struct {
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
}

func mkValidators(t *testing.T, n int) []testValidator {
	t.Helper()
	out := make([]testValidator, n)
	for i := range out {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate ed25519 keypair: %v", err)
		}
		out[i] = testValidator{pub: pub, priv: priv}
	}
	return out
}

func validatorEntries(vs []testValidator) []ValidatorEntry {
	out := make([]ValidatorEntry, len(vs))
	for i, v := range vs {
		out[i].PublicKeyHash = sha256.Sum256(v.pub)
		out[i].PublicKey = v.pub
		out[i].Type = protocol.SignatureTypeED25519
	}
	return out
}

func signWith(t *testing.T, b *Bundle, v testValidator) {
	t.Helper()
	signer := SignerFor(v.priv, v.pub)
	if _, err := b.AddSignature(signer); err != nil {
		t.Fatalf("AddSignature: %v", err)
	}
}

func TestBundle_Verify_DefaultThreshold(t *testing.T) {
	vs := mkValidators(t, 4)
	b := &Bundle{
		Network:      "testnet",
		Partition:    "Directory",
		ValidatorSet: validatorEntries(vs),
	}
	signWith(t, b, vs[0])
	signWith(t, b, vs[1])
	signWith(t, b, vs[2])
	if err := b.Verify(VerifyOptions{}); err != nil {
		t.Fatalf("Verify with 3/4 sigs: %v", err)
	}

	b.Signatures = b.Signatures[:2]
	if err := b.Verify(VerifyOptions{}); err == nil {
		t.Fatal("Verify with 2/4 sigs should fail")
	}
}

func TestBundle_Verify_CustomThreshold(t *testing.T) {
	vs := mkValidators(t, 3)
	b := &Bundle{ValidatorSet: validatorEntries(vs)}
	signWith(t, b, vs[0])
	if err := b.Verify(VerifyOptions{MinSignatures: 1}); err != nil {
		t.Fatalf("Verify with custom threshold 1: %v", err)
	}
	if err := b.Verify(VerifyOptions{MinSignatures: 2}); err == nil {
		t.Fatal("Verify with custom threshold 2 and 1 sig should fail")
	}
}

func TestBundle_Verify_DuplicateSigsCountOnce(t *testing.T) {
	vs := mkValidators(t, 4)
	b := &Bundle{ValidatorSet: validatorEntries(vs)}
	signWith(t, b, vs[0])
	signWith(t, b, vs[0])
	signWith(t, b, vs[0])
	if err := b.Verify(VerifyOptions{}); err == nil {
		t.Fatal("duplicate signatures from same validator should not satisfy threshold")
	}
}

func TestBundle_Verify_NonValidatorSigsIgnored(t *testing.T) {
	vs := mkValidators(t, 3)
	intruder := mkValidators(t, 1)[0]
	b := &Bundle{ValidatorSet: validatorEntries(vs)}
	signWith(t, b, vs[0])
	signWith(t, b, intruder) // not in validator set
	if err := b.Verify(VerifyOptions{MinSignatures: 1}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestBundle_Verify_StructuralErrors(t *testing.T) {
	if err := (*Bundle)(nil).Verify(VerifyOptions{}); err == nil {
		t.Fatal("nil bundle should fail")
	}
	if err := (&Bundle{}).Verify(VerifyOptions{}); err == nil {
		t.Fatal("empty bundle should fail")
	}
	vs := mkValidators(t, 1)
	b := &Bundle{ValidatorSet: validatorEntries(vs)}
	err := b.Verify(VerifyOptions{})
	if err == nil || !strings.Contains(err.Error(), "no signatures") {
		t.Fatalf("expected 'no signatures' error, got %v", err)
	}
}

// TestBundle_RoundTrip exercises the full produce-and-verify flow.
func TestBundle_RoundTrip(t *testing.T) {
	vs := mkValidators(t, 5)
	b := &Bundle{
		Network:            "mainnet",
		Partition:          "Directory",
		MajorBlockIndex:    100,
		MinorBlockIndex:    12345,
		MajorBlockTimeUnix: 1700000000,
		ValidatorSet:       validatorEntries(vs),
	}

	// Threshold for 5 validators: ceil(10/3) = 4.
	for i := 0; i < 3; i++ {
		signWith(t, b, vs[i])
		if err := b.Verify(VerifyOptions{}); err == nil {
			t.Fatalf("Verify should fail with %d/5 sigs", i+1)
		}
	}
	signWith(t, b, vs[3])
	if err := b.Verify(VerifyOptions{}); err != nil {
		t.Fatalf("Verify with 4/5 should pass: %v", err)
	}

	// Tamper a payload field — verification should fail.
	b.MajorBlockIndex = 999
	if err := b.Verify(VerifyOptions{}); err == nil {
		t.Fatal("Verify should fail after tampering MajorBlockIndex")
	}
}

// TestBundle_BinaryRoundTrip ensures Marshal/UnmarshalBinary preserve
// every persisted field including signatures (issue #3983).
func TestBundle_BinaryRoundTrip(t *testing.T) {
	vs := mkValidators(t, 3)
	original := &Bundle{
		Version:            1,
		Network:            "testnet",
		Partition:          "Directory",
		MajorBlockIndex:    100,
		MinorBlockIndex:    1234,
		MajorBlockTimeUnix: 1700000000,
		PerPartitionAnchors: []PartitionAnchorEntry{
			{Partition: "Directory", RootChainAnchor: [32]byte{0x11}, StateTreeAnchor: [32]byte{0x22}},
			{Partition: "Apollo", RootChainAnchor: [32]byte{0x33}, StateTreeAnchor: [32]byte{0x44}},
		},
		ValidatorSet: validatorEntries(vs),
	}
	for _, v := range vs {
		signWith(t, original, v)
	}

	wire, err := original.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}

	var got Bundle
	if err := got.UnmarshalBinary(wire); err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}

	if got.CanonicalHash() != original.CanonicalHash() {
		t.Error("canonical hash differs after round-trip")
	}
	if len(got.Signatures) != len(original.Signatures) {
		t.Errorf("signature count = %d, want %d", len(got.Signatures), len(original.Signatures))
	}
	if got.MajorBlockIndex != original.MajorBlockIndex {
		t.Error("MajorBlockIndex lost in round-trip")
	}

	// Verify still works after round-trip.
	if err := got.Verify(VerifyOptions{}); err != nil {
		t.Errorf("Verify after round-trip: %v", err)
	}
}

func TestBundle_UnmarshalBinary_TruncatedFails(t *testing.T) {
	b := &Bundle{Version: 1, Network: "x", Partition: "y"}
	wire, _ := b.MarshalBinary()
	var got Bundle
	if err := got.UnmarshalBinary(wire[:len(wire)/2]); err == nil {
		t.Fatal("expected truncation error")
	} else if !strings.Contains(err.Error(), "truncated") {
		t.Errorf("error = %v, want 'truncated'", err)
	}
}

func TestCanonicalHash_OrderInsensitive(t *testing.T) {
	vs := mkValidators(t, 3)
	a := &Bundle{
		Network:      "x",
		Partition:    "Directory",
		ValidatorSet: validatorEntries(vs),
	}
	rev := append([]ValidatorEntry(nil), a.ValidatorSet...)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	b := &Bundle{
		Network:      "x",
		Partition:    "Directory",
		ValidatorSet: rev,
	}
	if a.CanonicalHash() != b.CanonicalHash() {
		t.Fatal("canonical hash should be invariant to validator-set order")
	}
}
