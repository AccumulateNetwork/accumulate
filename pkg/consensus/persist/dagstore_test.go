// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package persist

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// TestDAGStoreRetainSparesLaterWrites: a checkpoint's retention runs
// concurrently with the primary writing the rounds above it (#4448). What
// was written after the checkpoint began must survive its Retain, or a
// restart loses the rounds between the checkpoint and the frontier.
func TestDAGStoreRetainSparesLaterWrites(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenDAGStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	old := &types.Batch{Transactions: [][]byte{[]byte("old")}}
	kept := &types.Batch{Transactions: [][]byte{[]byte("kept")}}
	late := &types.Batch{Transactions: [][]byte{[]byte("late")}}
	for _, b := range []*types.Batch{old, kept} {
		if _, err := s.PutBatch(b); err != nil {
			t.Fatal(err)
		}
	}
	mark := s.Mark()
	if _, err := s.PutBatch(late); err != nil {
		t.Fatal(err)
	}
	s.Retain(map[string]bool{BatchName(kept.Digest()): true}, mark)

	if s.HasBatch(old.Digest()) {
		t.Fatal("a batch no checkpoint names survived Retain")
	}
	for _, b := range []*types.Batch{kept, late} {
		if !s.HasBatch(b.Digest()) {
			t.Fatalf("Retain removed %s", b.Digest())
		}
	}

	// Reopened, the store holds what survived, readable.
	s, err = OpenDAGStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(s.Names()); n != 2 {
		t.Fatalf("reopened store holds %d entries, want 2", n)
	}
	if b, err := s.Batch(BatchName(late.Digest())); err != nil || b.Digest() != late.Digest() {
		t.Fatalf("reading back: %v", err)
	}
}

// TestDAGStoreVotedSurvivesReopen: the headers this node voted for, one per
// author and round, are on disk before the vote is signed and come back when
// the store is reopened (#4590); a checkpoint's retention drops the rounds
// below its floor, and a record cut short by a crash is ignored.
func TestDAGStoreVotedSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenDAGStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	author := make(ed25519.PublicKey, ed25519.PublicKeySize)
	author[0] = 1
	records := []VoteRecord{
		{Author: author, Round: 5, Header: types.HeaderDigest{5}},
		{Author: author, Round: 6, Header: types.HeaderDigest{6}},
		{Author: author, Round: 7, Header: types.HeaderDigest{7}},
	}
	for _, r := range records {
		if err := s.PutVoted(r.Author, r.Round, r.Header); err != nil {
			t.Fatal(err)
		}
	}
	// The same record again, as a rebroadcast after a failed vote sends it,
	// is not written twice.
	if err := s.PutVoted(author, 7, types.HeaderDigest{7}); err != nil {
		t.Fatal(err)
	}

	s, err = OpenDAGStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Voted(); !reflect.DeepEqual(got, records) {
		t.Fatalf("reopened store holds %v, want %v", got, records)
	}

	s.RetainVoted(6)
	s, err = OpenDAGStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Voted(); !reflect.DeepEqual(got, records[1:]) {
		t.Fatalf("after retaining from round 6 the store holds %v, want %v", got, records[1:])
	}

	// A crash in the middle of a write leaves a partial last line.
	f, err := os.OpenFile(filepath.Join(dir, votedFile), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("8 0101"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	s, err = OpenDAGStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Voted(); !reflect.DeepEqual(got, records[1:]) {
		t.Fatalf("with a partial last record the store holds %v, want %v", got, records[1:])
	}
	if err := s.PutVoted(author, 8, types.HeaderDigest{8}); err != nil {
		t.Fatal(err)
	}
	s, err = OpenDAGStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Voted(); len(got) != 3 || got[2].Round != 8 {
		t.Fatalf("a record written after a partial one is lost: %v", got)
	}
}

// TestDAGStoreRecordAfterFailedWriteSurvives: a write that fails part way
// (ENOSPC, say) leaves a partial line with no newline, and the next record
// must start a line of its own (#4590). Before the fix it was appended to
// the partial line, the merged line had four fields, and the record of a
// vote that was sent was dropped on the next start.
func TestDAGStoreRecordAfterFailedWriteSurvives(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenDAGStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, votedFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("0 0102"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	author := make(ed25519.PublicKey, ed25519.PublicKeySize)
	author[0] = 1
	if err := s.PutVoted(author, 1, types.HeaderDigest{1}); err != nil {
		t.Fatal(err)
	}
	s, err = OpenDAGStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Voted(); len(got) != 1 || got[0].Round != 1 {
		t.Fatalf("the record written after a failed write is lost: %v", got)
	}
}

// TestDAGStoreVotedFloorSurvivesReopen: the floor the records were pruned to
// is written with them and read back (#4590). A reopened store otherwise
// held the records it kept but not that lower rounds had been voted in and
// dropped, and a rejoin below them could vote there again. The floor only
// rises: the first checkpoint after a restart asks for a lower one.
func TestDAGStoreVotedFloorSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenDAGStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	author := make(ed25519.PublicKey, ed25519.PublicKeySize)
	author[0] = 1
	records := []VoteRecord{
		{Author: author, Round: 5, Header: types.HeaderDigest{5}},
		{Author: author, Round: 6, Header: types.HeaderDigest{6}},
		{Author: author, Round: 7, Header: types.HeaderDigest{7}},
	}
	for _, r := range records {
		if err := s.PutVoted(r.Author, r.Round, r.Header); err != nil {
			t.Fatal(err)
		}
	}
	reopen := func(floor types.Round, want []VoteRecord, what string) {
		t.Helper()
		var err error
		s, err = OpenDAGStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := s.VotedFloor(); got != floor {
			t.Fatalf("%s: reopened store has floor %d, want %d", what, got, floor)
		}
		if got := s.Voted(); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: reopened store holds %v, want %v", what, got, want)
		}
	}

	// A floor that drops no record is still written.
	s.RetainVoted(4)
	reopen(4, records, "after retaining from round 4")
	s.RetainVoted(6)
	reopen(6, records[1:], "after retaining from round 6")
	// A lower floor does not lower it.
	s.RetainVoted(2)
	reopen(6, records[1:], "after a lower floor")

	// A last line cut short by a crash is dropped; the floor is kept.
	f, err := os.OpenFile(filepath.Join(dir, votedFile), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("8 0101"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	reopen(6, records[1:], "with a partial last record")
	reopen(6, records[1:], "after the rewrite")
}
