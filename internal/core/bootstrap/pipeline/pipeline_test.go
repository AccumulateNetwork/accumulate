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

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/headerwalk"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// --- Fakes -------------------------------------------------------

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

// --- Helpers -----------------------------------------------------

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

// --- Tests -------------------------------------------------------

// TestBootstrap_HappyPath exercises Phase A (trust) + Phase B (DN
// data + DN convergence) + Phase C (BVN data + BVN convergence).
//
// The DN headers at major-block 1 (pin) and ToMajorBlock both
// commit the same StateTreeRoot — that's the value the DN data
// phase converges against. The BVN's state is supplied by the
// BVNAnchorFromDN callback; the BVN data phase converges against
// it.
func TestBootstrap_HappyPath(t *testing.T) {
	dnURLs := []*url.URL{
		protocol.DnUrl().JoinPath("alice"),
		protocol.DnUrl().JoinPath("bob"),
	}
	bvnURLs := []*url.URL{
		protocol.PartitionUrl("Apollo").JoinPath("carol"),
	}

	dnRef := newDB(t)
	dnRoot := populateAccounts(t, dnRef, dnURLs)
	dnRefRO := dnRef.Begin(false)
	defer dnRefRO.Discard()

	bvnRef := newDB(t)
	bvnRoot := populateAccounts(t, bvnRef, bvnURLs)
	bvnRefRO := bvnRef.Begin(false)
	defer bvnRefRO.Discard()

	// Validator set: 2 validators, both sign every major block.
	vs := mkValidators(t, 2)
	set := validatorSet(vs)

	mkHeader := func(h uint64) *headerwalk.Header {
		return &headerwalk.Header{
			Height:        h,
			Time:          time.Unix(1700000000+int64(h)*60, 0),
			StateTreeRoot: dnRoot,
		}
	}

	hsrc := &fakeHeaderSource{
		headers: make(map[uint64]*headerwalk.Header),
		sigs:    make(map[uint64][]headerwalk.HeaderSignature),
	}
	for h := uint64(1); h <= 3; h++ {
		hdr := mkHeader(h)
		hsrc.headers[h] = hdr
		hsrc.sigs[h] = []headerwalk.HeaderSignature{
			signHeader(hdr, vs[0]),
			signHeader(hdr, vs[1]),
		}
	}

	// Pull source serves both DN and BVN refs. We reuse the
	// dual-source pattern: a function that picks the right ref
	// based on the URL's root identity.
	dualSrc := &dualPullSource{
		dn:  pull.NewDBSource(dnRefRO),
		bvn: pull.NewDBSource(bvnRefRO),
	}

	dnTgt := newDB(t)
	bvnTgt := newDB(t)

	res, err := Bootstrap(context.Background(), Options{
		HeaderSource:           hsrc,
		ToMajorBlock:           3,
		InitialValidatorSet:    set,
		QuorumOpts:             headerwalk.QuorumOptions{MinSignatures: 1},
		GenesisStateTreeAnchor: dnRoot, // pin matches genesis-major-block 1
		PullSource:             dualSrc,
		DNAccounts:             dnURLs,
		DNDatabase:             dnTgt,
		BVN:                    "Apollo",
		BVNAccounts:            bvnURLs,
		BVNDatabase:            bvnTgt,
		BVNAnchorFromDN: func(_ context.Context, _ *database.Database, bvn string) ([32]byte, uint64, error) {
			if bvn != "Apollo" {
				return [32]byte{}, 0, errors.New("unexpected bvn")
			}
			return bvnRoot, 7, nil
		},
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if res.DNVerifiedAnchor != dnRoot {
		t.Errorf("DNVerifiedAnchor = %x, want %x", res.DNVerifiedAnchor, dnRoot)
	}
	if res.DNLocalBPTRoot != dnRoot {
		t.Errorf("DNLocalBPTRoot = %x, want %x", res.DNLocalBPTRoot, dnRoot)
	}
	if res.DNVerifiedMajorBlock != 3 {
		t.Errorf("DNVerifiedMajorBlock = %d, want 3", res.DNVerifiedMajorBlock)
	}
	if res.DNAccountsPulled != len(dnURLs) {
		t.Errorf("DNAccountsPulled = %d, want %d", res.DNAccountsPulled, len(dnURLs))
	}
	if res.BVNVerifiedAnchor != bvnRoot {
		t.Errorf("BVNVerifiedAnchor = %x, want %x", res.BVNVerifiedAnchor, bvnRoot)
	}
	if res.BVNLocalBPTRoot != bvnRoot {
		t.Errorf("BVNLocalBPTRoot = %x, want %x", res.BVNLocalBPTRoot, bvnRoot)
	}
	if res.BVNVerifiedMajorBlock != 7 {
		t.Errorf("BVNVerifiedMajorBlock = %d, want 7", res.BVNVerifiedMajorBlock)
	}
	if res.BVNAccountsPulled != len(bvnURLs) {
		t.Errorf("BVNAccountsPulled = %d, want %d", res.BVNAccountsPulled, len(bvnURLs))
	}
}

// TestBootstrap_GenesisPinMismatchFails closes the loop on the
// "fail closed when chain isn't the expected network" guarantee.
func TestBootstrap_GenesisPinMismatchFails(t *testing.T) {
	dnRef := newDB(t)
	dnURLs := []*url.URL{protocol.DnUrl().JoinPath("alice")}
	dnRoot := populateAccounts(t, dnRef, dnURLs)
	dnRefRO := dnRef.Begin(false)
	defer dnRefRO.Discard()

	vs := mkValidators(t, 1)
	set := validatorSet(vs)
	hdr := &headerwalk.Header{Height: 1, StateTreeRoot: dnRoot}
	hsrc := &fakeHeaderSource{
		headers: map[uint64]*headerwalk.Header{1: hdr},
		sigs: map[uint64][]headerwalk.HeaderSignature{
			1: {signHeader(hdr, vs[0])},
		},
	}

	_, err := Bootstrap(context.Background(), Options{
		HeaderSource:           hsrc,
		ToMajorBlock:           1,
		InitialValidatorSet:    set,
		QuorumOpts:             headerwalk.QuorumOptions{MinSignatures: 1},
		GenesisStateTreeAnchor: [32]byte{0xde, 0xad, 0xbe, 0xef}, // doesn't match
		PullSource:             pull.NewDBSource(dnRefRO),
		DNAccounts:             dnURLs,
		DNDatabase:             newDB(t),
		BVN:                    "Apollo",
		BVNAccounts:            nil,
		BVNDatabase:            newDB(t),
		BVNAnchorFromDN: func(_ context.Context, _ *database.Database, _ string) ([32]byte, uint64, error) {
			return [32]byte{0x01}, 1, nil
		},
	})
	if !errors.Is(err, ErrGenesisMismatch) {
		t.Errorf("err = %v, want ErrGenesisMismatch chain", err)
	}
}

// TestBootstrap_RejectsMissingInputs — input-validation guards.
func TestBootstrap_RejectsMissingInputs(t *testing.T) {
	// One Options field missing per case; everything else stubbed
	// out enough to reach validate().
	base := func() Options {
		return Options{
			HeaderSource: &fakeHeaderSource{},
			ToMajorBlock: 1,
			PullSource:   pull.NewDBSource(nil),
			DNDatabase:   newDB(t),
			BVN:          "Apollo",
			BVNDatabase:  newDB(t),
			BVNAnchorFromDN: func(_ context.Context, _ *database.Database, _ string) ([32]byte, uint64, error) {
				return [32]byte{}, 0, nil
			},
		}
	}
	cases := []struct {
		name    string
		mutate  func(*Options)
		wantSub string
	}{
		{"no header source", func(o *Options) { o.HeaderSource = nil }, "HeaderSource"},
		{"no pull source", func(o *Options) { o.PullSource = nil }, "PullSource"},
		{"no DN db", func(o *Options) { o.DNDatabase = nil }, "DNDatabase"},
		{"no BVN db", func(o *Options) { o.BVNDatabase = nil }, "BVNDatabase"},
		{"no BVN", func(o *Options) { o.BVN = "" }, "BVN"},
		{"no BVNAnchorFromDN", func(o *Options) { o.BVNAnchorFromDN = nil }, "BVNAnchorFromDN"},
		{"zero ToMajorBlock", func(o *Options) { o.ToMajorBlock = 0 }, "ToMajorBlock"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := base()
			c.mutate(&o)
			_, err := Bootstrap(context.Background(), o)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("err = %q, want substring %q", err.Error(), c.wantSub)
			}
		})
	}
}

// dualPullSource serves DN-prefixed URLs from one ref DB, BVN-
// prefixed URLs from another. Test plumbing for the two-phase
// pipeline.
type dualPullSource struct {
	dn, bvn pull.Source
}

func (d *dualPullSource) Main(ctx context.Context, u *url.URL) (protocol.Account, error) {
	return d.pick(u).Main(ctx, u)
}
func (d *dualPullSource) DirectoryUrls(ctx context.Context, u *url.URL) ([]*url.URL, error) {
	return d.pick(u).DirectoryUrls(ctx, u)
}
func (d *dualPullSource) PendingIDs(ctx context.Context, u *url.URL) ([]*url.TxID, error) {
	return d.pick(u).PendingIDs(ctx, u)
}
func (d *dualPullSource) ChainNames(ctx context.Context, u *url.URL) ([]string, error) {
	return d.pick(u).ChainNames(ctx, u)
}
func (d *dualPullSource) ChainEntries(ctx context.Context, u *url.URL, name string) ([][]byte, error) {
	return d.pick(u).ChainEntries(ctx, u, name)
}
func (d *dualPullSource) pick(u *url.URL) pull.Source {
	if u.RootIdentity().Equal(protocol.DnUrl()) {
		return d.dn
	}
	return d.bvn
}
