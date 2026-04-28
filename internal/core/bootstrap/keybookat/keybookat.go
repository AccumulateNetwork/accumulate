// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package keybookat applies operators-keybook deltas to a v2
// headerwalk.ValidatorSet across the trust phase walk.
//
// Block validator quorum has already attested that any
// operators-keybook update inside a block was protocol-valid, so
// keybookat performs no signature checking — it's a pure data-
// transition function.
//
// Surface:
//
//   - ApplyOperation: typed primitive that takes one
//     protocol.KeyPageOperation and applies it to a ValidatorSet.
//
//   - ApplyDelta: bridge to headerwalk's opaque OperatorsDelta wire
//     format. The Payload of each delta is a binary-marshaled
//     KeyPageOperation; ApplyDelta unmarshals and dispatches.
//
// Operations supported (the only ones that change validator-set
// membership):
//
//   - AddKeyOperation
//   - RemoveKeyOperation
//   - UpdateKeyOperation
//
// Operations that don't affect membership (SetThresholdOperation,
// UpdateAllowedOperation, UpdateAccountAuthOperation) are accepted
// and ignored — applying them is a no-op on the validator set.
//
// The Kind field on OperatorsDelta is decorative; ApplyDelta dispatches
// on the unmarshaled operation type, not the string label.
package keybookat

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/headerwalk"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ApplyOperation produces the next ValidatorSet after applying one
// KeyPageOperation. Returns an error for malformed inputs (zero
// key-hash, RemoveKey/UpdateKey for a non-existent entry).
func ApplyOperation(set headerwalk.ValidatorSet, op protocol.KeyPageOperation) (headerwalk.ValidatorSet, error) {
	switch op := op.(type) {
	case *protocol.AddKeyOperation:
		return addKey(set, op.Entry.KeyHash)

	case *protocol.RemoveKeyOperation:
		return removeKey(set, op.Entry.KeyHash)

	case *protocol.UpdateKeyOperation:
		return updateKey(set, op.OldEntry.KeyHash, op.NewEntry.KeyHash)

	default:
		// Any other operation type doesn't change membership.
		// Threshold updates, transaction blacklist, etc. are
		// accepted as no-ops on the validator set.
		return set, nil
	}
}

// ApplyDelta is the bridge from headerwalk's wire format. Each delta
// carries a binary-marshaled KeyPageOperation in Payload. ApplyDelta
// unmarshals and applies them in order.
//
// This signature matches headerwalk.Walk's applyDelta callback
// argument, so the caller wires it directly:
//
//	headerwalk.Walk(ctx, src, from, to, initial, opts, keybookat.ApplyDelta)
func ApplyDelta(set headerwalk.ValidatorSet, deltas []headerwalk.OperatorsDelta) (headerwalk.ValidatorSet, error) {
	current := set
	for i, d := range deltas {
		op, err := decodeOperation(d.Payload)
		if err != nil {
			return current, fmt.Errorf("delta %d (%q): decode: %w", i, d.Kind, err)
		}
		next, err := ApplyOperation(current, op)
		if err != nil {
			return current, fmt.Errorf("delta %d (%q): apply: %w", i, d.Kind, err)
		}
		current = next
	}
	return current, nil
}

// EncodeOperation produces the OperatorsDelta wire form of a
// KeyPageOperation. Used by sources (live HeaderSource at
// OperatorsDeltaAt) to surface ops to the walker.
func EncodeOperation(op protocol.KeyPageOperation) (headerwalk.OperatorsDelta, error) {
	if op == nil {
		return headerwalk.OperatorsDelta{}, errors.New("keybookat: nil operation")
	}
	data, err := op.MarshalBinary()
	if err != nil {
		return headerwalk.OperatorsDelta{}, fmt.Errorf("marshal %T: %w", op, err)
	}
	return headerwalk.OperatorsDelta{
		Kind:    op.Type().String(),
		Payload: data,
	}, nil
}

// decodeOperation reverses EncodeOperation.
func decodeOperation(payload []byte) (protocol.KeyPageOperation, error) {
	op, err := protocol.UnmarshalKeyPageOperation(payload)
	if err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return op, nil
}

// --- helpers --------------------------------------------------------

func addKey(set headerwalk.ValidatorSet, keyHash []byte) (headerwalk.ValidatorSet, error) {
	pkh, err := keyHashToArr(keyHash)
	if err != nil {
		return set, fmt.Errorf("add: %w", err)
	}
	for _, v := range set.Validators {
		if v.PublicKeyHash == pkh {
			return set, fmt.Errorf("add: validator %x already in set", pkh[:8])
		}
	}
	out := headerwalk.ValidatorSet{
		Validators: append(append([]headerwalk.Validator(nil), set.Validators...),
			headerwalk.Validator{
				PublicKeyHash: pkh,
				Type:          protocol.SignatureTypeED25519,
			}),
	}
	return out, nil
}

func removeKey(set headerwalk.ValidatorSet, keyHash []byte) (headerwalk.ValidatorSet, error) {
	pkh, err := keyHashToArr(keyHash)
	if err != nil {
		return set, fmt.Errorf("remove: %w", err)
	}
	out := headerwalk.ValidatorSet{
		Validators: make([]headerwalk.Validator, 0, len(set.Validators)),
	}
	found := false
	for _, v := range set.Validators {
		if v.PublicKeyHash == pkh {
			found = true
			continue
		}
		out.Validators = append(out.Validators, v)
	}
	if !found {
		return set, fmt.Errorf("remove: validator %x not in set", pkh[:8])
	}
	return out, nil
}

func updateKey(set headerwalk.ValidatorSet, oldHash, newHash []byte) (headerwalk.ValidatorSet, error) {
	oldPkh, err := keyHashToArr(oldHash)
	if err != nil {
		return set, fmt.Errorf("update: old: %w", err)
	}
	newPkh, err := keyHashToArr(newHash)
	if err != nil {
		return set, fmt.Errorf("update: new: %w", err)
	}
	out := headerwalk.ValidatorSet{
		Validators: make([]headerwalk.Validator, len(set.Validators)),
	}
	found := false
	for i, v := range set.Validators {
		if v.PublicKeyHash == oldPkh {
			out.Validators[i] = headerwalk.Validator{
				PublicKeyHash: newPkh,
				Type:          protocol.SignatureTypeED25519,
			}
			found = true
			continue
		}
		out.Validators[i] = v
	}
	if !found {
		return set, fmt.Errorf("update: validator %x not in set", oldPkh[:8])
	}
	return out, nil
}

func keyHashToArr(b []byte) ([32]byte, error) {
	if len(b) != 32 {
		return [32]byte{}, fmt.Errorf("expected 32-byte key hash, got %d", len(b))
	}
	var out [32]byte
	copy(out[:], b)
	return out, nil
}
