// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package headerwalk

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type tv struct {
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
}

func mkValidators(t *testing.T, n int) []tv {
	t.Helper()
	out := make([]tv, n)
	for i := range out {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = tv{pub: pub, priv: priv}
	}
	return out
}

func validatorSet(vs []tv) ValidatorSet {
	out := ValidatorSet{Validators: make([]Validator, len(vs))}
	for i, v := range vs {
		out.Validators[i] = Validator{
			PublicKeyHash: sha256.Sum256(v.pub),
			PublicKey:     v.pub,
			Type:          protocol.SignatureTypeED25519,
		}
	}
	return out
}

func signHeader(h *Header, v tv) HeaderSignature {
	canonical := h.CanonicalHash()
	return HeaderSignature{
		PublicKeyHash: sha256.Sum256(v.pub),
		Signature:     ed25519.Sign(v.priv, canonical[:]),
	}
}

// fakeSource serves a hardcoded sequence of headers with hardcoded
// signature sets and zero deltas. Plenty for exercising Walk's
// per-step verification without needing a live network.
type fakeSource struct {
	headers map[uint64]*Header
	sigs    map[uint64][]HeaderSignature
	deltas  map[uint64][]OperatorsDelta
}

func (f *fakeSource) Header(_ context.Context, h uint64) (*Header, error) {
	if hd, ok := f.headers[h]; ok {
		return hd, nil
	}
	return nil, ErrNoSuchHeight
}

func (f *fakeSource) Signatures(_ context.Context, h uint64) ([]HeaderSignature, error) {
	return f.sigs[h], nil
}

func (f *fakeSource) OperatorsDeltaAt(_ context.Context, h uint64) ([]OperatorsDelta, error) {
	return f.deltas[h], nil
}

func TestVerifyQuorum_DefaultThreshold(t *testing.T) {
	vs := mkValidators(t, 4)
	set := validatorSet(vs)
	h := &Header{Height: 100, Time: time.Unix(1700000000, 0), StateTreeRoot: [32]byte{0xab}}

	// 3-of-4 — meets ceil(2*4/3)=3.
	sigs := []HeaderSignature{
		signHeader(h, vs[0]),
		signHeader(h, vs[1]),
		signHeader(h, vs[2]),
	}
	if err := VerifyQuorum(h, set, sigs, QuorumOptions{}); err != nil {
		t.Errorf("3-of-4 should pass default quorum: %v", err)
	}

	// 2-of-4 — below threshold.
	if err := VerifyQuorum(h, set, sigs[:2], QuorumOptions{}); err == nil {
		t.Error("2-of-4 should fail default quorum")
	} else if !errors.Is(err, ErrInsufficientQuorum) {
		t.Errorf("err = %v, want ErrInsufficientQuorum", err)
	}
}

func TestVerifyQuorum_DuplicatesCountOnce(t *testing.T) {
	vs := mkValidators(t, 4)
	set := validatorSet(vs)
	h := &Header{Height: 1, StateTreeRoot: [32]byte{0xee}}

	sig := signHeader(h, vs[0])
	sigs := []HeaderSignature{sig, sig, sig, sig} // same validator x4

	if err := VerifyQuorum(h, set, sigs, QuorumOptions{}); err == nil {
		t.Error("duplicate sigs from one validator should not reach 3-of-4 threshold")
	}
}

func TestVerifyQuorum_NonValidatorIgnored(t *testing.T) {
	vs := mkValidators(t, 4)
	stranger := mkValidators(t, 1)[0]
	set := validatorSet(vs)
	h := &Header{Height: 1, StateTreeRoot: [32]byte{0x11}}

	sigs := []HeaderSignature{
		signHeader(h, vs[0]),
		signHeader(h, vs[1]),
		signHeader(h, vs[2]),
		signHeader(h, stranger), // not in the set; ignored
	}
	if err := VerifyQuorum(h, set, sigs, QuorumOptions{}); err != nil {
		t.Errorf("3 valid + 1 stranger should pass: %v", err)
	}
}

func TestVerifyQuorum_ForgedSignatureIgnored(t *testing.T) {
	vs := mkValidators(t, 4)
	set := validatorSet(vs)
	h := &Header{Height: 1, StateTreeRoot: [32]byte{0x22}}

	// One valid sig, two forged sigs from validators (signed against
	// a different hash), one valid sig.
	other := &Header{Height: 99, StateTreeRoot: [32]byte{0x99}}
	otherCanonical := other.CanonicalHash()
	forge := func(v tv) HeaderSignature {
		return HeaderSignature{
			PublicKeyHash: sha256.Sum256(v.pub),
			Signature:     ed25519.Sign(v.priv, otherCanonical[:]),
		}
	}
	sigs := []HeaderSignature{
		signHeader(h, vs[0]),
		forge(vs[1]),
		forge(vs[2]),
		signHeader(h, vs[3]),
	}
	if err := VerifyQuorum(h, set, sigs, QuorumOptions{}); err == nil {
		t.Error("only 2 valid sigs of 4 — should fail quorum")
	}
}

func TestWalk_NoDeltas_AllBlocksVerify(t *testing.T) {
	vs := mkValidators(t, 4)
	set := validatorSet(vs)

	src := &fakeSource{
		headers: make(map[uint64]*Header),
		sigs:    make(map[uint64][]HeaderSignature),
		deltas:  make(map[uint64][]OperatorsDelta),
	}
	for h := uint64(1); h <= 5; h++ {
		hdr := &Header{
			Height:        h,
			Time:          time.Unix(1700000000+int64(h)*60, 0),
			StateTreeRoot: [32]byte{byte(h)},
		}
		src.headers[h] = hdr
		src.sigs[h] = []HeaderSignature{
			signHeader(hdr, vs[0]),
			signHeader(hdr, vs[1]),
			signHeader(hdr, vs[2]),
		}
	}

	last, err := Walk(context.Background(), src, 1, 5, set, QuorumOptions{}, nil)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if last == nil {
		t.Fatal("expected non-nil terminal Step")
	}
	if last.Header.Height != 5 {
		t.Errorf("terminal height = %d, want 5", last.Header.Height)
	}
	// No deltas → before == after.
	if len(last.ValidatorSetBefore.Validators) != len(last.ValidatorSetAfter.Validators) {
		t.Error("no-op delta should not change validator set size")
	}
}

func TestWalk_FailsAtFirstBadBlock(t *testing.T) {
	vs := mkValidators(t, 4)
	set := validatorSet(vs)

	src := &fakeSource{
		headers: make(map[uint64]*Header),
		sigs:    make(map[uint64][]HeaderSignature),
		deltas:  make(map[uint64][]OperatorsDelta),
	}
	for h := uint64(1); h <= 5; h++ {
		hdr := &Header{Height: h, StateTreeRoot: [32]byte{byte(h)}}
		src.headers[h] = hdr
		switch h {
		case 3:
			// Only one validator signs at height 3 — below quorum.
			src.sigs[h] = []HeaderSignature{signHeader(hdr, vs[0])}
		default:
			src.sigs[h] = []HeaderSignature{
				signHeader(hdr, vs[0]),
				signHeader(hdr, vs[1]),
				signHeader(hdr, vs[2]),
			}
		}
	}

	last, err := Walk(context.Background(), src, 1, 5, set, QuorumOptions{}, nil)
	if !errors.Is(err, ErrInsufficientQuorum) {
		t.Fatalf("err = %v, want ErrInsufficientQuorum", err)
	}
	// Should have made it through height 2 successfully.
	if last == nil || last.Header.Height != 2 {
		t.Errorf("expected last successful step at height 2, got %+v", last)
	}
}

// TestWalk_DeltasShrinkSetMidWalk simulates an operators-keybook
// rotation that drops a validator partway through the walk. The
// applyDelta callback removes vs[3] starting at height 3; subsequent
// blocks must verify against the smaller set.
func TestWalk_DeltasShrinkSetMidWalk(t *testing.T) {
	vs := mkValidators(t, 4)
	initial := validatorSet(vs)

	src := &fakeSource{
		headers: make(map[uint64]*Header),
		sigs:    make(map[uint64][]HeaderSignature),
		deltas:  make(map[uint64][]OperatorsDelta),
	}
	for h := uint64(1); h <= 5; h++ {
		hdr := &Header{Height: h, StateTreeRoot: [32]byte{byte(h)}}
		src.headers[h] = hdr
	}
	// Heights 1,2: signed by 3 of 4 (above 4-set threshold of 3).
	src.sigs[1] = []HeaderSignature{signHeader(src.headers[1], vs[0]), signHeader(src.headers[1], vs[1]), signHeader(src.headers[1], vs[2])}
	src.sigs[2] = []HeaderSignature{signHeader(src.headers[2], vs[0]), signHeader(src.headers[2], vs[1]), signHeader(src.headers[2], vs[2])}
	// Block 2 carries the rotation delta: drop vs[3].
	src.deltas[2] = []OperatorsDelta{{Kind: "drop:3"}}
	// Heights 3..5: signed by 2 of remaining 3 (above 3-set threshold of 2).
	for _, h := range []uint64{3, 4, 5} {
		src.sigs[h] = []HeaderSignature{signHeader(src.headers[h], vs[0]), signHeader(src.headers[h], vs[1])}
	}

	apply := func(set ValidatorSet, deltas []OperatorsDelta) (ValidatorSet, error) {
		next := ValidatorSet{Validators: append([]Validator(nil), set.Validators...)}
		for _, d := range deltas {
			if d.Kind == "drop:3" && len(next.Validators) > 0 {
				next.Validators = next.Validators[:len(next.Validators)-1]
			}
		}
		return next, nil
	}

	last, err := Walk(context.Background(), src, 1, 5, initial, QuorumOptions{}, apply)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if last.Header.Height != 5 {
		t.Errorf("terminal height = %d, want 5", last.Header.Height)
	}
	if got, want := len(last.ValidatorSetAfter.Validators), 3; got != want {
		t.Errorf("terminal validator set size = %d, want %d", got, want)
	}
}
