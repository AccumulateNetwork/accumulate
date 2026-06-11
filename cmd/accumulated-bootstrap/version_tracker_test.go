// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"testing"
)

// present builds a probeResult for a live, voting validator at version v.
func present(version string) probeResult {
	return probeResult{version: version, votingPower: 10, alive: true}
}

func TestComputeConsensus_AllPresentAndAgree(t *testing.T) {
	roster := []string{"aaa", "bbb", "ccc"}
	probes := map[string]probeResult{
		"aaa": present("v1.0"),
		"bbb": present("v1.0"),
		"ccc": present("v1.0"),
	}

	r := computeConsensus("Cyclops", roster, probes)
	if !r.ValidatorConsensus {
		t.Fatalf("expected consensus=true, got false: %+v", r)
	}
	if r.AgreedVersion != "v1.0" {
		t.Fatalf("expected agreedVersion v1.0, got %q", r.AgreedVersion)
	}
	if len(r.Versions["v1.0"]) != 3 {
		t.Fatalf("expected 3 hashes under v1.0, got %v", r.Versions)
	}
	if r.Pending {
		t.Fatalf("expected pending=false")
	}
	for _, v := range r.Validators {
		if !v.Present {
			t.Fatalf("expected validator %s present", v.KeyHash)
		}
	}
}

func TestComputeConsensus_MissingValidator(t *testing.T) {
	roster := []string{"aaa", "bbb", "ccc"}
	probes := map[string]probeResult{
		"aaa": present("v1.0"),
		"bbb": present("v1.0"),
		// ccc missing
	}

	r := computeConsensus("Cyclops", roster, probes)
	if r.ValidatorConsensus {
		t.Fatalf("expected consensus=false when a validator is missing")
	}
	if r.AgreedVersion != "" {
		t.Fatalf("expected empty agreedVersion, got %q", r.AgreedVersion)
	}
	// ccc must be reported present=false.
	var ccc *ValidatorVersion
	for i := range r.Validators {
		if r.Validators[i].KeyHash == "ccc" {
			ccc = &r.Validators[i]
		}
	}
	if ccc == nil || ccc.Present {
		t.Fatalf("expected ccc present=false, got %+v", ccc)
	}
}

func TestComputeConsensus_VersionSplit(t *testing.T) {
	roster := []string{"aaa", "bbb"}
	probes := map[string]probeResult{
		"aaa": present("v1.0"),
		"bbb": present("v2.0"),
	}

	r := computeConsensus("Cyclops", roster, probes)
	if r.ValidatorConsensus {
		t.Fatalf("expected consensus=false on version split")
	}
	if r.AgreedVersion != "" {
		t.Fatalf("expected empty agreedVersion on split, got %q", r.AgreedVersion)
	}
	if len(r.Versions) != 2 {
		t.Fatalf("expected 2 distinct versions, got %v", r.Versions)
	}
}

func TestComputeConsensus_NonRosterPeerIgnored(t *testing.T) {
	roster := []string{"aaa", "bbb"}
	probes := map[string]probeResult{
		"aaa": present("v1.0"),
		"bbb": present("v1.0"),
		// zzz is a live peer not in the roster — must be ignored.
		"zzz": present("v9.9"),
	}

	r := computeConsensus("Cyclops", roster, probes)
	if !r.ValidatorConsensus {
		t.Fatalf("expected consensus=true; non-roster peer should be ignored: %+v", r)
	}
	if r.AgreedVersion != "v1.0" {
		t.Fatalf("expected agreedVersion v1.0, got %q", r.AgreedVersion)
	}
	if _, ok := r.Versions["v9.9"]; ok {
		t.Fatalf("non-roster version v9.9 should not appear: %v", r.Versions)
	}
	if len(r.Validators) != 2 {
		t.Fatalf("expected only 2 roster validators reported, got %d", len(r.Validators))
	}
}

func TestComputeConsensus_CatchingUpCountedButFlagged(t *testing.T) {
	roster := []string{"aaa", "bbb"}
	pr := present("v1.0")
	pr.catchingUp = true
	probes := map[string]probeResult{
		"aaa": present("v1.0"),
		"bbb": pr,
	}

	r := computeConsensus("Cyclops", roster, probes)
	// Catching-up validator is counted by version, so consensus holds.
	if !r.ValidatorConsensus {
		t.Fatalf("expected consensus=true; catching-up node still counts by version: %+v", r)
	}
	// But it must be flagged.
	var bbb *ValidatorVersion
	for i := range r.Validators {
		if r.Validators[i].KeyHash == "bbb" {
			bbb = &r.Validators[i]
		}
	}
	if bbb == nil || !bbb.Present || !bbb.CatchingUp {
		t.Fatalf("expected bbb present and catchingUp, got %+v", bbb)
	}
}

func TestComputeConsensus_NoVotingPowerNotPresent(t *testing.T) {
	roster := []string{"aaa"}
	probes := map[string]probeResult{
		// alive but voting power 0 (e.g. demoted) => not present.
		"aaa": {version: "v1.0", votingPower: 0, alive: true},
	}

	r := computeConsensus("Cyclops", roster, probes)
	if r.ValidatorConsensus {
		t.Fatalf("expected consensus=false; zero voting power is not present")
	}
}

func TestComputeConsensus_EmptyRosterPending(t *testing.T) {
	r := computeConsensus("Cyclops", nil, map[string]probeResult{})
	if !r.Pending {
		t.Fatalf("expected pending=true for empty roster")
	}
	if r.ValidatorConsensus {
		t.Fatalf("expected consensus=false for empty roster")
	}
}
