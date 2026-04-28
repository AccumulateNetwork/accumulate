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

// fakeHeaderSource emits a single quorum-signed header at a fixed
// height. Sufficient for exercising the lifecycle.
type fakeHeaderSource struct {
	hdr  *headerwalk.Header
	sigs []headerwalk.HeaderSignature
}

func (f *fakeHeaderSource) Header(_ context.Context, h uint64) (*headerwalk.Header, error) {
	if h == f.hdr.Height {
		return f.hdr, nil
	}
	return nil, headerwalk.ErrNoSuchHeight
}

func (f *fakeHeaderSource) Signatures(_ context.Context, h uint64) ([]headerwalk.HeaderSignature, error) {
	return f.sigs, nil
}

func (f *fakeHeaderSource) OperatorsDeltaAt(_ context.Context, _ uint64) ([]headerwalk.OperatorsDelta, error) {
	return nil, nil
}

// TestLifecycle_BootstrapToRunResume is the v2 capstone. It walks
// the entire stack in a single test:
//
//  1. Reference DB stamps a known BPT root (the "network state")
//  2. Validators sign a header committing that root
//  3. pipeline.Bootstrap runs the trust phase against the fake
//     header source, the data phase against a DBSource over the
//     reference, and convergence — the local BPT root must match
//  4. The artifact is saved via bootpersist.Save
//  5. A fresh Instance simulates restart: detectBootstrapState reads
//     the artifact, restores the nodestate.Machine in ACTIVE state
//  6. advertisementFromMachine projects to the wire format peers
//     would receive in NodeInfo
//
// Regressions in any of phases 1-8 surface here.
func TestLifecycle_BootstrapToRunResume(t *testing.T) {
	const network = "test-lifecycle"

	// Step 1: reference state.
	urls := []*url.URL{
		protocol.DnUrl().JoinPath("alice"),
		protocol.DnUrl().JoinPath("bob"),
	}
	ref := database.OpenInMemory(nil)
	ref.SetObserver(execute.NewDatabaseObserver())
	rb := ref.Begin(true)
	for _, u := range urls {
		if err := rb.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
			t.Fatal(err)
		}
		entry := make([]byte, 32)
		entry[0] = byte(len(u.Path))
		if err := rb.Account(u).MainChain().Inner().AddEntry(entry, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := rb.UpdateBPT(); err != nil {
		t.Fatal(err)
	}
	if err := rb.Commit(); err != nil {
		t.Fatal(err)
	}
	refRO := ref.Begin(false)
	defer refRO.Discard()
	expectedRoot, err := refRO.GetBptRootHash()
	if err != nil {
		t.Fatal(err)
	}

	// Step 2: signed header.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubHash := sha256.Sum256(pub)
	validatorSet := headerwalk.ValidatorSet{
		Validators: []headerwalk.Validator{
			{
				PublicKeyHash: pubHash,
				PublicKey:     pub,
				Type:          protocol.SignatureTypeED25519,
			},
		},
	}
	hdr := &headerwalk.Header{
		Height:        500,
		Time:          time.Unix(1700000000, 0).UTC(),
		StateTreeRoot: expectedRoot,
		// Synthetic header — leave AnchorTxHash zero so CanonicalHash
		// uses the fields-fallback. Validator signs that.
	}
	canonical := hdr.CanonicalHash()
	sigs := []headerwalk.HeaderSignature{{
		PublicKeyHash: pubHash,
		Signature:     ed25519.Sign(priv, canonical[:]),
	}}
	hsrc := &fakeHeaderSource{hdr: hdr, sigs: sigs}

	// Step 3: Bootstrap.
	tgt := database.OpenInMemory(nil)
	tgt.SetObserver(execute.NewDatabaseObserver())
	res, err := pipeline.Bootstrap(context.Background(), pipeline.Options{
		HeaderSource:        hsrc,
		StartHeight:         500,
		EndHeight:           500,
		InitialValidatorSet: validatorSet,
		PullSource:          pull.NewDBSource(refRO),
		Accounts:            urls,
		Database:            tgt,
		QuorumOpts:          headerwalk.QuorumOptions{MinSignatures: 1},
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

	// Step 4: persist artifact.
	dir := t.TempDir()
	pinHash := [32]byte{0xde, 0xad}
	t.Cleanup(pinned.RegisterForTest(network, pinned.Pin{
		ValidatorSetHash: pinHash,
		PinnedHeight:     500,
	}))

	now := time.Now().UTC()
	art := &bootpersist.Artifact{
		Network:                network,
		Partition:              "Directory",
		PinnedValidatorSetHash: pinHash,
		PinnedHeight:           500,
		VerifiedAnchor:         res.VerifiedAnchor,
		VerifiedHeight:         res.TerminalStep.Header.Height,
		State: bootpersist.StateRecord{
			Current:        "ACTIVE",
			EnteredBooting: now,
			EnteredActive:  now,
		},
		Cursors: bootpersist.Cursors{
			WalkLastVerified: res.TerminalStep.Header.Height,
			AccountsPulled:   uint64(res.AccountsPulled),
		},
	}
	if err := bootpersist.Save(dir, art); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Step 5: simulate restart.
	inst := instanceForTest(t, dir)
	if err := inst.detectBootstrapState(); err != nil {
		t.Fatalf("detectBootstrapState: %v", err)
	}
	if inst.BootMachine() == nil {
		t.Fatal("expected non-nil BootMachine after restart")
	}
	if got, want := inst.BootMachine().State(), nodestate.StateActive; got != want {
		t.Errorf("State after restart = %v, want %v", got, want)
	}

	// Step 6: wire-format advertisement.
	ad := advertisementFromMachine(inst.BootMachine())
	if ad == nil {
		t.Fatal("expected non-nil advertisement")
	}
	if ad.VerifiedAnchor != expectedRoot {
		t.Errorf("Advertisement VerifiedAnchor mismatch")
	}
	if ad.SinceBlock != 500 {
		t.Errorf("Advertisement SinceBlock = %d, want 500", ad.SinceBlock)
	}
}

// TestLifecycle_DivergentLocalStateFails closes the loop on the
// proof: if the puller produces state that doesn't match the
// validator-attested anchor, Bootstrap MUST fail and the artifact
// MUST NOT be persisted. This is the "fail closed" guarantee of
// the v2 design.
func TestLifecycle_DivergentLocalStateFails(t *testing.T) {
	u := protocol.DnUrl().JoinPath("alice")

	ref := database.OpenInMemory(nil)
	ref.SetObserver(execute.NewDatabaseObserver())
	rb := ref.Begin(true)
	if err := rb.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
		t.Fatal(err)
	}
	if err := rb.UpdateBPT(); err != nil {
		t.Fatal(err)
	}
	if err := rb.Commit(); err != nil {
		t.Fatal(err)
	}

	// Header signed against a fabricated root — different from what
	// the puller's source actually serves.
	fakeRoot := [32]byte{0xde, 0xad, 0xbe, 0xef}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHash := sha256.Sum256(pub)
	hdr := &headerwalk.Header{Height: 1, StateTreeRoot: fakeRoot}
	canonical := hdr.CanonicalHash()
	sigs := []headerwalk.HeaderSignature{{
		PublicKeyHash: pubHash,
		Signature:     ed25519.Sign(priv, canonical[:]),
	}}
	hsrc := &fakeHeaderSource{hdr: hdr, sigs: sigs}

	tgt := database.OpenInMemory(nil)
	tgt.SetObserver(execute.NewDatabaseObserver())
	refRO := ref.Begin(false)
	defer refRO.Discard()

	_, err := pipeline.Bootstrap(context.Background(), pipeline.Options{
		HeaderSource: hsrc,
		StartHeight:  1,
		EndHeight:    1,
		InitialValidatorSet: headerwalk.ValidatorSet{Validators: []headerwalk.Validator{
			{PublicKeyHash: pubHash, PublicKey: pub, Type: protocol.SignatureTypeED25519},
		}},
		PullSource: pull.NewDBSource(refRO),
		Accounts:   []*url.URL{u},
		Database:   tgt,
		QuorumOpts: headerwalk.QuorumOptions{MinSignatures: 1},
	})
	if err == nil {
		t.Fatal("Bootstrap should fail when validator-attested root != local BPT root")
	}
	if !contains(err.Error(), "convergence") {
		t.Errorf("expected convergence error, got %v", err)
	}

	// Target DB must have nothing committed (fail-closed).
	tgtRO := tgt.Begin(false)
	defer tgtRO.Discard()
	root, err := tgtRO.GetBptRootHash()
	if err != nil {
		t.Fatal(err)
	}
	if root != ([32]byte{}) {
		t.Errorf("target DB BPT root = %x, want zero (no commit on failure)", root)
	}

	_ = errors.New // silence import if not used
}

// contains is a stdlib-free strings.Contains for use in this test
// without pulling another import in.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
