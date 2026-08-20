// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bullshark

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// These tests pin the commit-completeness invariant that run 20260820T090939Z
// proved broken: every certificate in the DAG must eventually be emitted by
// the commit path, exactly once — including certificates from a validator
// whose certs consistently miss the round-1 parent window and are only picked
// up later via weak links, by which time their round is below the commit
// frontier and a higher-round certificate from the same author has already
// committed. Before the digest-set dedup and rescue window, 46% of all
// certificates (and every transaction in their batches) were silently
// discarded (#4111).

// TestCommitCompleteness_SlowAuthor drives twelve rounds in which validator
// 3's certificate is never referenced by the next round's headers — it is
// weak-linked two rounds late — and asserts that every certificate is still
// committed exactly once.
func TestCommitCompleteness_SlowAuthor(t *testing.T) {
	h := newTestHelper(t, 4)
	bs := New(h.committee, h.dag)

	const slow = 3
	const lastRound = types.Round(12)

	outputs := map[types.CertificateDigest]int{}
	process := func(certs ...*types.Certificate) {
		for _, c := range certs {
			for _, o := range bs.ProcessCertificate(c) {
				outputs[o.Certificate.Digest()]++
			}
		}
	}

	var tracked []*types.Certificate // everything that must eventually commit
	r0 := h.insertGenesis()

	prevFast := r0 // previous round's fast certs
	slowByRound := map[types.Round]*types.Certificate{}
	for r := types.Round(1); r <= lastRound; r++ {
		// Fast validators reference the previous round's fast certs, plus the
		// slow validator's certificate from two rounds back — the weak link:
		// it "arrived" too late for its own round+1 headers.
		parents := append([]*types.Certificate{}, prevFast...)
		if late, ok := slowByRound[r-3]; ok && r >= 3 {
			parents = append(parents, late)
			delete(slowByRound, r-3)
		}

		var fast []*types.Certificate
		for i := 0; i < len(h.keys); i++ {
			if i == slow {
				continue
			}
			cert := h.createCert(i, r, parents)
			require.NoError(t, h.dag.Insert(cert))
			fast = append(fast, cert)
		}

		// The slow validator keeps up with the DAG (it references the same
		// parents) — its OWN certificate is just never referenced in time.
		slowCert := h.createCert(slow, r, prevFast)
		require.NoError(t, h.dag.Insert(slowCert))
		slowByRound[r] = slowCert

		tracked = append(tracked, fast...)
		tracked = append(tracked, slowCert)
		process(fast...)
		process(slowCert)
		prevFast = fast
	}

	// Flush: a few fully-connected rounds so trailing certificates land in a
	// committed leader's history. Reference every still-unlinked slow cert.
	parents := append([]*types.Certificate{}, prevFast...)
	for _, c := range slowByRound {
		parents = append(parents, c)
	}
	for r := lastRound + 1; r <= lastRound+4; r++ {
		var certs []*types.Certificate
		for i := 0; i < len(h.keys); i++ {
			cert := h.createCert(i, r, parents)
			require.NoError(t, h.dag.Insert(cert))
			certs = append(certs, cert)
		}
		process(certs...)
		parents = certs
	}

	// Exactly-once: nothing is ever emitted twice.
	for d, n := range outputs {
		require.LessOrEqualf(t, n, 1, "certificate %v emitted %d times", d, n)
	}

	// Completeness: every certificate through lastRound committed — the slow
	// validator's included.
	for _, c := range tracked {
		require.Equalf(t, 1, outputs[c.Digest()],
			"certificate author=%x round=%d was not committed",
			c.Author()[:4], c.Round())
	}
}

// TestStragglerRescuedAcrossCommitFrontier pins the exact shape the
// per-author round watermark got wrong: author A's round-2 certificate
// becomes reachable (via a weak link) only AFTER A's round-3 and round-4
// certificates have committed. The old dedup skipped it as "already
// committed" — round 2 <= lastCommitted[A] — and the old orderDag round
// floor excluded it as below the frontier. It must commit, once.
func TestStragglerRescuedAcrossCommitFrontier(t *testing.T) {
	h := newTestHelper(t, 4)
	bs := New(h.committee, h.dag)

	const a = 3 // the straggling author

	outputs := map[types.CertificateDigest]int{}
	process := func(certs ...*types.Certificate) {
		for _, c := range certs {
			for _, o := range bs.ProcessCertificate(c) {
				outputs[o.Certificate.Digest()]++
			}
		}
	}

	r0 := h.insertGenesis()
	r1 := h.insertRound(1, r0)
	process(r1...)

	// Round 2: fast certs reference r1; A's cert exists but no round-3
	// header references it.
	r2fast := h.insertPartialRound(2, r1, []int{0, 1, 2})
	a2 := h.createCert(a, 2, r1)
	require.NoError(t, h.dag.Insert(a2))
	process(r2fast...)
	process(a2)

	// Rounds 3-4: A participates normally (its r3/r4 certs are referenced
	// and will commit ahead of its orphaned r2 cert).
	r3 := h.insertRound(3, r2fast)
	process(r3...)
	r4 := h.insertRound(4, r3)
	process(r4...)

	// Round 5 weak-links A's round-2 straggler.
	parents5 := append(append([]*types.Certificate{}, r4...), a2)
	var r5 []*types.Certificate
	for i := range h.keys {
		cert := h.createCert(i, 5, parents5)
		require.NoError(t, h.dag.Insert(cert))
		r5 = append(r5, cert)
	}
	process(r5...)

	r6 := h.insertRound(6, r5)
	process(r6...)
	r7 := h.insertRound(7, r6)
	process(r7...)

	// By now leader 6 has committed and its history includes a2.
	require.GreaterOrEqual(t, bs.LastCommitRound(), types.Round(6))
	require.Equal(t, 1, outputs[a2.Digest()],
		"the straggler certificate must commit despite its round being below "+
			"the frontier and a higher-round cert from the same author having "+
			"committed first")

	// And still exactly-once for everything.
	for d, n := range outputs {
		require.LessOrEqualf(t, n, 1, "certificate %v emitted %d times", d, n)
	}
}
