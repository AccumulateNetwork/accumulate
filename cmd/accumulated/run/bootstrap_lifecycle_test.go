// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bootpersist"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/headerwalk"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pinned"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pipeline"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// fakeHeaderSource emits a single quorum-signed header at every
// height in the configured map. Sufficient for exercising the
// lifecycle.
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

// dualPullSource serves DN-prefixed URLs from one ref DB, BVN-
// prefixed URLs from another. Same shape as the pipeline test's.
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

func newObservedDB(t *testing.T) *database.Database {
	t.Helper()
	db := database.OpenInMemory(nil)
	db.SetObserver(execute.NewDatabaseObserver())
	return db
}

func populateReference(t *testing.T, db *database.Database, urls []*url.URL) [32]byte {
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

// TestLifecycle_BootstrapToRunResume is the v2 capstone covering
// the full corrected (DN, BVN)-pair flow:
//
//  1. Reference DN + BVN databases populated to known BPT roots.
//  2. fakeHeaderSource emits major-block headers signed by a
//     2-validator set, with major-block 1's StateTreeAnchor
//     matching the binary pin.
//  3. pipeline.Bootstrap runs Phase A (trust) + Phase B (DN data
//     + DN convergence) + Phase C (BVN data + BVN convergence)
//     against fake sources.
//  4. New-shape bootpersist.Artifact saved with both DN and BVN
//     anchors.
//  5. Fresh Instance runs detectBootstrapState; nodestate.Machine
//     restored in ACTIVE with the DN anchor in the advertisement.
//
// Regressions across phases 1-6 surface here as a single failure.
func TestLifecycle_BootstrapToRunResume(t *testing.T) {
	const network = "test-lifecycle"

	dnURLs := []*url.URL{
		protocol.DnUrl().JoinPath("alice"),
		protocol.DnUrl().JoinPath("bob"),
	}
	bvnURLs := []*url.URL{
		protocol.PartitionUrl("Apollo").JoinPath("carol"),
	}

	dnRef := newObservedDB(t)
	dnRoot := populateReference(t, dnRef, dnURLs)
	dnRefRO := dnRef.Begin(false)
	defer dnRefRO.Discard()

	bvnRef := newObservedDB(t)
	bvnRoot := populateReference(t, bvnRef, bvnURLs)
	bvnRefRO := bvnRef.Begin(false)
	defer bvnRefRO.Discard()

	// Header source: 3 major blocks, all committing dnRoot, all
	// signed by both validators. major-block 1 doubles as the
	// genesis-pin source.
	vs := mkValidators(t, 2)
	set := mkValidatorSet(vs)
	hsrc := &fakeHeaderSource{
		headers: make(map[uint64]*headerwalk.Header),
		sigs:    make(map[uint64][]headerwalk.HeaderSignature),
	}
	for h := uint64(1); h <= 3; h++ {
		hdr := &headerwalk.Header{
			Height:        h,
			Time:          time.Unix(1700000000+int64(h)*60, 0),
			StateTreeRoot: dnRoot,
		}
		hsrc.headers[h] = hdr
		hsrc.sigs[h] = []headerwalk.HeaderSignature{
			signTestHeader(hdr, vs[0]),
			signTestHeader(hdr, vs[1]),
		}
	}

	// Run the two-phase pipeline.
	dnTgt := newObservedDB(t)
	bvnTgt := newObservedDB(t)
	res, err := pipeline.Bootstrap(context.Background(), pipeline.Options{
		HeaderSource:           hsrc,
		ToMajorBlock:           3,
		InitialValidatorSet:    set,
		QuorumOpts:             headerwalk.QuorumOptions{MinSignatures: 1},
		GenesisStateTreeAnchor: dnRoot,
		PullSource:             &dualPullSource{dn: pull.NewDBSource(dnRefRO), bvn: pull.NewDBSource(bvnRefRO)},
		DNAccounts:             dnURLs,
		DNDatabase:             dnTgt,
		BVN:                    "Apollo",
		BVNAccounts:            bvnURLs,
		BVNDatabase:            bvnTgt,
		BVNAnchorFromDN: func(_ context.Context, _ *database.Database, _ string) ([32]byte, uint64, error) {
			return bvnRoot, 7, nil
		},
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	// Persist artifact with the new shape.
	dir := t.TempDir()
	t.Cleanup(pinned.RegisterForTest(network, pinned.Pin{
		DNGenesisStateTreeAnchor: dnRoot,
	}))

	now := time.Now().UTC()
	art := &bootpersist.Artifact{
		Network:                  network,
		BVN:                      "Apollo",
		DNGenesisStateTreeAnchor: dnRoot,
		DNVerifiedAnchor:         res.DNVerifiedAnchor,
		DNVerifiedMajorBlock:     res.DNVerifiedMajorBlock,
		BVNVerifiedAnchor:        res.BVNVerifiedAnchor,
		BVNVerifiedMajorBlock:    res.BVNVerifiedMajorBlock,
		State: bootpersist.StateRecord{
			Current:        "ACTIVE",
			EnteredBooting: now,
			EnteredActive:  now,
		},
	}
	if err := bootpersist.Save(dir, art); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Simulate restart.
	inst := instanceForTest(t, dir)
	if err := inst.detectBootstrapState(); err != nil {
		t.Fatalf("detectBootstrapState: %v", err)
	}
	if inst.BootMachine() == nil {
		t.Fatal("expected non-nil BootMachine after restart")
	}
	if got := inst.BootMachine().State(); got != nodestate.StateActive {
		t.Errorf("State after restart = %v, want ACTIVE", got)
	}

	// Advertisement carries the DN-side anchor (network-shared
	// across all BVNs that bootstrapped against this DN state).
	ad := advertisementFromMachine(inst.BootMachine())
	if ad.VerifiedAnchor != res.DNVerifiedAnchor {
		t.Errorf("Advertisement VerifiedAnchor = %x, want DN anchor %x",
			ad.VerifiedAnchor, res.DNVerifiedAnchor)
	}
	if ad.SinceBlock != res.DNVerifiedMajorBlock {
		t.Errorf("Advertisement SinceBlock = %d, want DNVerifiedMajorBlock %d",
			ad.SinceBlock, res.DNVerifiedMajorBlock)
	}

	// Cross-check the persisted BVN fields too.
	if inst.BootArtifact().BVNVerifiedAnchor != res.BVNVerifiedAnchor {
		t.Errorf("Artifact BVNVerifiedAnchor not preserved")
	}
}

// TestLifecycle_GenesisPinMismatchFails closes the loop on the
// "fail closed when chain isn't the expected network" guarantee.
// Header source serves a major-block-1 with a wrong StateTreeRoot
// versus the binary pin; Bootstrap aborts with ErrGenesisMismatch
// before any state is committed to either DB.
func TestLifecycle_GenesisPinMismatchFails(t *testing.T) {
	dnRef := newObservedDB(t)
	dnURLs := []*url.URL{protocol.DnUrl().JoinPath("alice")}
	dnRoot := populateReference(t, dnRef, dnURLs)
	dnRefRO := dnRef.Begin(false)
	defer dnRefRO.Discard()

	vs := mkValidators(t, 1)
	set := mkValidatorSet(vs)
	hdr := &headerwalk.Header{Height: 1, StateTreeRoot: dnRoot}
	hsrc := &fakeHeaderSource{
		headers: map[uint64]*headerwalk.Header{1: hdr},
		sigs:    map[uint64][]headerwalk.HeaderSignature{1: {signTestHeader(hdr, vs[0])}},
	}

	_, err := pipeline.Bootstrap(context.Background(), pipeline.Options{
		HeaderSource:           hsrc,
		ToMajorBlock:           1,
		InitialValidatorSet:    set,
		QuorumOpts:             headerwalk.QuorumOptions{MinSignatures: 1},
		GenesisStateTreeAnchor: [32]byte{0xde, 0xad, 0xbe, 0xef}, // doesn't match
		PullSource:             pull.NewDBSource(dnRefRO),
		DNAccounts:             dnURLs,
		DNDatabase:             newObservedDB(t),
		BVN:                    "Apollo",
		BVNAccounts:            nil,
		BVNDatabase:            newObservedDB(t),
		BVNAnchorFromDN: func(_ context.Context, _ *database.Database, _ string) ([32]byte, uint64, error) {
			return [32]byte{}, 0, errors.New("should not be called")
		},
	})
	if err == nil {
		t.Fatal("Bootstrap should have failed on genesis pin mismatch")
	}
	if !strings.Contains(err.Error(), "genesis") {
		t.Errorf("err = %v, want substring 'genesis'", err)
	}
}

// --- Test crypto helpers ----------------------------------------

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

func mkValidatorSet(vs []tv) headerwalk.ValidatorSet {
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

func signTestHeader(h *headerwalk.Header, v tv) headerwalk.HeaderSignature {
	canonical := h.CanonicalHash()
	return headerwalk.HeaderSignature{
		PublicKeyHash: sha256.Sum256(v.pub),
		Signature:     ed25519.Sign(v.priv, canonical[:]),
	}
}
