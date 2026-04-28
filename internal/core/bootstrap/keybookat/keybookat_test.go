// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package keybookat

import (
	"strings"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/headerwalk"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func mkSet(hashes ...byte) headerwalk.ValidatorSet {
	out := headerwalk.ValidatorSet{}
	for _, b := range hashes {
		var pkh [32]byte
		pkh[0] = b
		out.Validators = append(out.Validators, headerwalk.Validator{
			PublicKeyHash: pkh,
			Type:          protocol.SignatureTypeED25519,
		})
	}
	return out
}

func keyHash(b byte) []byte {
	out := make([]byte, 32)
	out[0] = b
	return out
}

func contains(set headerwalk.ValidatorSet, b byte) bool {
	var pkh [32]byte
	pkh[0] = b
	for _, v := range set.Validators {
		if v.PublicKeyHash == pkh {
			return true
		}
	}
	return false
}

func TestApplyOperation_AddKey(t *testing.T) {
	set := mkSet(0x01, 0x02)
	out, err := ApplyOperation(set, &protocol.AddKeyOperation{
		Entry: protocol.KeySpecParams{KeyHash: keyHash(0x03)},
	})
	if err != nil {
		t.Fatalf("AddKey: %v", err)
	}
	if len(out.Validators) != 3 {
		t.Errorf("len = %d, want 3", len(out.Validators))
	}
	if !contains(out, 0x03) {
		t.Error("new key not in set")
	}
	if !contains(out, 0x01) || !contains(out, 0x02) {
		t.Error("existing keys lost")
	}
}

func TestApplyOperation_AddKey_DuplicateRejected(t *testing.T) {
	set := mkSet(0x01)
	_, err := ApplyOperation(set, &protocol.AddKeyOperation{
		Entry: protocol.KeySpecParams{KeyHash: keyHash(0x01)},
	})
	if err == nil {
		t.Fatal("expected error adding duplicate")
	}
	if !strings.Contains(err.Error(), "already in set") {
		t.Errorf("err = %q, want substring 'already in set'", err)
	}
}

func TestApplyOperation_RemoveKey(t *testing.T) {
	set := mkSet(0x01, 0x02, 0x03)
	out, err := ApplyOperation(set, &protocol.RemoveKeyOperation{
		Entry: protocol.KeySpecParams{KeyHash: keyHash(0x02)},
	})
	if err != nil {
		t.Fatalf("RemoveKey: %v", err)
	}
	if len(out.Validators) != 2 {
		t.Errorf("len = %d, want 2", len(out.Validators))
	}
	if contains(out, 0x02) {
		t.Error("removed key still in set")
	}
}

func TestApplyOperation_RemoveKey_NotFoundRejected(t *testing.T) {
	set := mkSet(0x01)
	_, err := ApplyOperation(set, &protocol.RemoveKeyOperation{
		Entry: protocol.KeySpecParams{KeyHash: keyHash(0x99)},
	})
	if err == nil {
		t.Fatal("expected error removing nonexistent")
	}
}

func TestApplyOperation_UpdateKey(t *testing.T) {
	set := mkSet(0x01, 0x02)
	out, err := ApplyOperation(set, &protocol.UpdateKeyOperation{
		OldEntry: protocol.KeySpecParams{KeyHash: keyHash(0x02)},
		NewEntry: protocol.KeySpecParams{KeyHash: keyHash(0x42)},
	})
	if err != nil {
		t.Fatalf("UpdateKey: %v", err)
	}
	if len(out.Validators) != 2 {
		t.Errorf("len = %d, want 2 (size preserved on update)", len(out.Validators))
	}
	if contains(out, 0x02) {
		t.Error("old key still present after update")
	}
	if !contains(out, 0x42) {
		t.Error("new key absent after update")
	}
}

func TestApplyOperation_UpdateKey_NotFoundRejected(t *testing.T) {
	set := mkSet(0x01)
	_, err := ApplyOperation(set, &protocol.UpdateKeyOperation{
		OldEntry: protocol.KeySpecParams{KeyHash: keyHash(0x99)},
		NewEntry: protocol.KeySpecParams{KeyHash: keyHash(0x42)},
	})
	if err == nil {
		t.Fatal("expected error updating nonexistent")
	}
}

func TestApplyOperation_NonMembershipOpsAreNoOps(t *testing.T) {
	set := mkSet(0x01, 0x02)
	cases := []protocol.KeyPageOperation{
		&protocol.SetThresholdKeyPageOperation{Threshold: 5},
		&protocol.UpdateAllowedKeyPageOperation{},
	}
	for _, op := range cases {
		out, err := ApplyOperation(set, op)
		if err != nil {
			t.Errorf("%T: unexpected error: %v", op, err)
			continue
		}
		if len(out.Validators) != 2 || !contains(out, 0x01) || !contains(out, 0x02) {
			t.Errorf("%T: set should be unchanged, got %+v", op, out.Validators)
		}
	}
}

func TestApplyDelta_RoundTripsAddRemove(t *testing.T) {
	set := mkSet(0x01)

	addOp := &protocol.AddKeyOperation{Entry: protocol.KeySpecParams{KeyHash: keyHash(0x02)}}
	removeOp := &protocol.RemoveKeyOperation{Entry: protocol.KeySpecParams{KeyHash: keyHash(0x01)}}

	addDelta, err := EncodeOperation(addOp)
	if err != nil {
		t.Fatal(err)
	}
	removeDelta, err := EncodeOperation(removeOp)
	if err != nil {
		t.Fatal(err)
	}

	out, err := ApplyDelta(set, []headerwalk.OperatorsDelta{addDelta, removeDelta})
	if err != nil {
		t.Fatalf("ApplyDelta: %v", err)
	}
	if len(out.Validators) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Validators))
	}
	if !contains(out, 0x02) {
		t.Error("added key missing")
	}
	if contains(out, 0x01) {
		t.Error("removed key still present")
	}
}

func TestApplyDelta_StopsAtFirstError(t *testing.T) {
	set := mkSet(0x01)
	// Second op is invalid (remove nonexistent), first succeeds.
	addOp := &protocol.AddKeyOperation{Entry: protocol.KeySpecParams{KeyHash: keyHash(0x02)}}
	badOp := &protocol.RemoveKeyOperation{Entry: protocol.KeySpecParams{KeyHash: keyHash(0x99)}}

	addDelta, _ := EncodeOperation(addOp)
	badDelta, _ := EncodeOperation(badOp)

	out, err := ApplyDelta(set, []headerwalk.OperatorsDelta{addDelta, badDelta})
	if err == nil {
		t.Fatal("expected error from invalid second delta")
	}
	// Returned set should be the partial result through op 0 — i.e.,
	// 0x01 + 0x02. Walker can use this to record what was successfully
	// applied before failure.
	if !contains(out, 0x02) {
		t.Error("partial result should include successfully-applied 0x02")
	}
}

func TestApplyDelta_EmptyIsIdentity(t *testing.T) {
	set := mkSet(0x01, 0x02, 0x03)
	out, err := ApplyDelta(set, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Validators) != 3 {
		t.Errorf("len = %d, want 3 (identity)", len(out.Validators))
	}
}

// TestApplyDelta_PluggableInWalker pins the contract that ApplyDelta
// has the exact signature headerwalk.Walk's applyDelta callback
// requires. If walk's signature changes, this fails.
func TestApplyDelta_PluggableInWalker(t *testing.T) {
	var _ func(headerwalk.ValidatorSet, []headerwalk.OperatorsDelta) (headerwalk.ValidatorSet, error) = ApplyDelta
}

func TestEncodeOperation_RejectsNil(t *testing.T) {
	_, err := EncodeOperation(nil)
	if err == nil {
		t.Error("expected error for nil op")
	}
}
