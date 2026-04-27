// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package trustbundle

import (
	"strings"
	"testing"
)

func mkVal(b byte) ValidatorEntry {
	var kh [32]byte
	kh[0] = b
	return ValidatorEntry{PublicKeyHash: kh, PublicKey: []byte{b}}
}

func mkSig(b byte) ValidatorSignature {
	var kh [32]byte
	kh[0] = b
	return ValidatorSignature{PublicKeyHash: kh, Signature: []byte("stub")}
}

func TestBundle_Verify_DefaultThreshold(t *testing.T) {
	// 4 validators, default threshold = ceil(2*4/3) = 3.
	b := &Bundle{
		ValidatorSet: []ValidatorEntry{mkVal(1), mkVal(2), mkVal(3), mkVal(4)},
		Signatures:   []ValidatorSignature{mkSig(1), mkSig(2), mkSig(3)},
	}
	if err := b.Verify(VerifyOptions{}); err != nil {
		t.Fatalf("Verify with 3/4 sigs: %v", err)
	}

	// 2/4 should be insufficient.
	b.Signatures = []ValidatorSignature{mkSig(1), mkSig(2)}
	if err := b.Verify(VerifyOptions{}); err == nil {
		t.Fatal("Verify with 2/4 sigs should fail")
	}
}

func TestBundle_Verify_CustomThreshold(t *testing.T) {
	b := &Bundle{
		ValidatorSet: []ValidatorEntry{mkVal(1), mkVal(2), mkVal(3)},
		Signatures:   []ValidatorSignature{mkSig(1)},
	}
	if err := b.Verify(VerifyOptions{MinSignatures: 1}); err != nil {
		t.Fatalf("Verify with custom threshold 1: %v", err)
	}
	if err := b.Verify(VerifyOptions{MinSignatures: 2}); err == nil {
		t.Fatal("Verify with custom threshold 2 and 1 sig should fail")
	}
}

func TestBundle_Verify_DuplicateSigsCountOnce(t *testing.T) {
	b := &Bundle{
		ValidatorSet: []ValidatorEntry{mkVal(1), mkVal(2), mkVal(3), mkVal(4)},
		Signatures:   []ValidatorSignature{mkSig(1), mkSig(1), mkSig(1)},
	}
	if err := b.Verify(VerifyOptions{}); err == nil {
		t.Fatal("duplicate signatures from same validator should not satisfy threshold")
	}
}

func TestBundle_Verify_NonValidatorSigsIgnored(t *testing.T) {
	// Signatures from non-validators are silently ignored; threshold
	// only counts validator signatures.
	b := &Bundle{
		ValidatorSet: []ValidatorEntry{mkVal(1), mkVal(2), mkVal(3)},
		Signatures:   []ValidatorSignature{mkSig(1), mkSig(99) /* not a validator */},
	}
	err := b.Verify(VerifyOptions{MinSignatures: 1})
	if err != nil {
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
	b := &Bundle{ValidatorSet: []ValidatorEntry{mkVal(1)}}
	err := b.Verify(VerifyOptions{})
	if err == nil || !strings.Contains(err.Error(), "no signatures") {
		t.Fatalf("expected 'no signatures' error, got %v", err)
	}
}
