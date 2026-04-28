// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pipeline

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/convergence"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/headerwalk"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// --- Header fake (mirrors headerwalk/walk_test.go's pattern) -------

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

func validatorSet(vs []tv) headerwalk.ValidatorSet {
	out := headerwalk.ValidatorSet{Validators: make([]headerwalk.Validator, len(vs))}
	for i, v := range vs {
		out.Validators[i] = headerwalk.Validator{
			PublicKeyHash: sha256.Sum256(v.pub),
			PublicKey:     v.pub,
			Type:          protocol.SignatureTypeED25519,
		}
	}
	return out
}

func signHeader(h *headerwalk.Header, v tv) headerwalk.HeaderSignature {
	canonical := h.CanonicalHash()
	return headerwalk.HeaderSignature{
		PublicKeyHash: sha256.Sum256(v.pub),
		Signature:     ed25519.Sign(v.priv, canonical[:]),
	}
}

type fakeHeaderSource struct {
	headers map[uint64]*headerwalk.Header
	sigs    map[uint64][]headerwalk.HeaderSignature
}

func (f *fakeHeaderSource) Header(_ context.Context, h uint64) (*headerwalk.Header, error) {
	if hd, ok := f.headers[h]; ok {
		return hd, nil
	}
	return nil, headerwalk.ErrNoSuchHeight
}

func (f *fakeHeaderSource) Signatures(_ context.Context, h uint64) ([]headerwalk.HeaderSignature, error) {
	return f.sigs[h], nil
}

func (f *fakeHeaderSource) OperatorsDeltaAt(_ context.Context, _ uint64) ([]headerwalk.OperatorsDelta, error) {
	return nil, nil
}

// --- Helpers ------------------------------------------------------

func newDB(t *testing.T) *database.Database {
	t.Helper()
	db := database.OpenInMemory(nil)
	db.SetObserver(execute.NewDatabaseObserver())
	return db
}

func populateAccounts(t *testing.T, db *database.Database, urls []*url.URL) [32]byte {
	t.Helper()
	b := db.Begin(true)
	for _, u := range urls {
		if err := b.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
			t.Fatal(err)
		}
		entry := make([]byte, 32)
		entry[0] = byte(len(u.Path))
		if err := b.Account(u).MainChain().Inner().AddEntry(entry, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.UpdateBPT(); err != nil {
		t.Fatal(err)
	}
	root, err := b.GetBptRootHash()
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
	return root
}

// --- Tests --------------------------------------------------------

// TestBootstrap_HappyPath ties trust phase + data phase + convergence
// end-to-end. Reference DB stamps a known BPT root; the fake header
// source signs a header that commits to that root; the puller fills
// a fresh DB from the reference; convergence accepts.
func TestBootstrap_HappyPath(t *testing.T) {
	urls := []*url.URL{
		protocol.DnUrl().JoinPath("alice"),
		protocol.DnUrl().JoinPath("bob"),
	}

	ref := newDB(t)
	expectedRoot := populateAccounts(t, ref, urls)
	refRO := ref.Begin(false)
	defer refRO.Discard()

	// Header source signs a header whose StateTreeRoot equals the
	// reference's root. Two-validator set, both sign — meets default
	// quorum (ceil(2*2/3) = 2).
	vs := mkValidators(t, 2)
	set := validatorSet(vs)
	hdr := &headerwalk.Header{
		Height:        100,
		Time:          time.Unix(1700000000, 0),
		StateTreeRoot: expectedRoot,
	}
	hsrc := &fakeHeaderSource{
		headers: map[uint64]*headerwalk.Header{100: hdr},
		sigs: map[uint64][]headerwalk.HeaderSignature{
			100: {signHeader(hdr, vs[0]), signHeader(hdr, vs[1])},
		},
	}

	// Target DB.
	tgt := newDB(t)

	res, err := Bootstrap(context.Background(), Options{
		HeaderSource:        hsrc,
		StartHeight:         100,
		EndHeight:           100,
		InitialValidatorSet: set,
		PullSource:          pull.NewDBSource(refRO),
		Accounts:            urls,
		Database:            tgt,
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if res.VerifiedAnchor != expectedRoot {
		t.Errorf("VerifiedAnchor mismatch: got %x, want %x", res.VerifiedAnchor, expectedRoot)
	}
	if res.LocalBPTRoot != expectedRoot {
		t.Errorf("LocalBPTRoot mismatch: got %x, want %x", res.LocalBPTRoot, expectedRoot)
	}
	if res.AccountsPulled != len(urls) {
		t.Errorf("AccountsPulled = %d, want %d", res.AccountsPulled, len(urls))
	}
}

// TestBootstrap_TrustPhaseFails — header signature is forged; the
// walk fails before any data is pulled. No partial state should
// leak into the target DB.
func TestBootstrap_TrustPhaseFails(t *testing.T) {
	urls := []*url.URL{protocol.DnUrl().JoinPath("alice")}
	ref := newDB(t)
	expectedRoot := populateAccounts(t, ref, urls)
	refRO := ref.Begin(false)
	defer refRO.Discard()

	vs := mkValidators(t, 4)
	set := validatorSet(vs)
	hdr := &headerwalk.Header{Height: 1, StateTreeRoot: expectedRoot}
	// Only one signer — well below ceil(2*4/3)=3.
	hsrc := &fakeHeaderSource{
		headers: map[uint64]*headerwalk.Header{1: hdr},
		sigs: map[uint64][]headerwalk.HeaderSignature{
			1: {signHeader(hdr, vs[0])},
		},
	}

	tgt := newDB(t)
	_, err := Bootstrap(context.Background(), Options{
		HeaderSource:        hsrc,
		StartHeight:         1,
		EndHeight:           1,
		InitialValidatorSet: set,
		PullSource:          pull.NewDBSource(refRO),
		Accounts:            urls,
		Database:            tgt,
	})
	if err == nil {
		t.Fatal("expected trust-phase failure")
	}
	if !errors.Is(err, headerwalk.ErrInsufficientQuorum) {
		t.Errorf("err = %v, want ErrInsufficientQuorum chain", err)
	}

	// Target DB should be untouched (no partial commit).
	tgtRO := tgt.Begin(false)
	defer tgtRO.Discard()
	root, _ := tgtRO.GetBptRootHash()
	if root != ([32]byte{}) {
		t.Errorf("expected untouched target BPT (zero root), got %x", root)
	}
}

// TestBootstrap_ConvergenceFails — header walker returns a
// StateTreeRoot that differs from what the puller's source actually
// holds. Convergence must catch the divergence.
func TestBootstrap_ConvergenceFails(t *testing.T) {
	urls := []*url.URL{protocol.DnUrl().JoinPath("alice")}
	ref := newDB(t)
	populateAccounts(t, ref, urls)
	refRO := ref.Begin(false)
	defer refRO.Discard()

	// Header source claims a different StateTreeRoot than what's
	// actually in the reference DB. Simulates a peer lying about
	// its BPT, or a divergent fork.
	vs := mkValidators(t, 2)
	set := validatorSet(vs)
	hdr := &headerwalk.Header{Height: 1, StateTreeRoot: [32]byte{0xde, 0xad, 0xbe, 0xef}}
	hsrc := &fakeHeaderSource{
		headers: map[uint64]*headerwalk.Header{1: hdr},
		sigs: map[uint64][]headerwalk.HeaderSignature{
			1: {signHeader(hdr, vs[0]), signHeader(hdr, vs[1])},
		},
	}

	tgt := newDB(t)
	_, err := Bootstrap(context.Background(), Options{
		HeaderSource:        hsrc,
		StartHeight:         1,
		EndHeight:           1,
		InitialValidatorSet: set,
		PullSource:          pull.NewDBSource(refRO),
		Accounts:            urls,
		Database:            tgt,
	})
	if err == nil {
		t.Fatal("expected convergence failure")
	}
	if !errors.Is(err, convergence.ErrMismatch) {
		t.Errorf("err = %v, want convergence.ErrMismatch chain", err)
	}
}

// TestBootstrap_RejectsMissingInputs — guards.
func TestBootstrap_RejectsMissingInputs(t *testing.T) {
	cases := []struct {
		name string
		opts Options
		want string
	}{
		{"no header source", Options{PullSource: pull.NewDBSource(nil), Database: newDB(t)}, "HeaderSource"},
		{"no pull source", Options{HeaderSource: &fakeHeaderSource{}, Database: newDB(t)}, "PullSource"},
		{"no database", Options{HeaderSource: &fakeHeaderSource{}, PullSource: pull.NewDBSource(nil)}, "Database"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Bootstrap(context.Background(), c.opts)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %q, want substring %q", err.Error(), c.want)
			}
		})
	}
}
