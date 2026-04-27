// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package backwalk

import (
	"errors"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// setupOperatorsKeybook creates a partition operators keybook with the
// given key hashes as page-1 entries. Returns the keybook URL.
func setupOperatorsKeybook(t *testing.T, batch *database.Batch, partition string, keyHashes ...[32]byte) *url.URL {
	t.Helper()
	bookUrl := protocol.PartitionUrl(partition).JoinPath(protocol.Operators)
	pageUrl := protocol.FormatKeyPageUrl(bookUrl, 0)

	book := &protocol.KeyBook{Url: bookUrl, PageCount: 1}
	if err := batch.Account(bookUrl).Main().Put(book); err != nil {
		t.Fatal(err)
	}

	page := &protocol.KeyPage{Url: pageUrl, Version: 1}
	for _, kh := range keyHashes {
		hashCopy := make([]byte, 32)
		copy(hashCopy, kh[:])
		page.AddKeySpec(&protocol.KeySpec{PublicKeyHash: hashCopy})
	}
	if err := batch.Account(pageUrl).Main().Put(page); err != nil {
		t.Fatal(err)
	}
	return bookUrl
}

func mkKeyHash(b byte) [32]byte {
	var h [32]byte
	h[0] = b
	return h
}

func TestVerifyValidatorQuorum_NoSignatures(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(nullObserver{})
	batch := db.Begin(true)
	defer batch.Discard()

	setupOperatorsKeybook(t, batch, "Directory",
		mkKeyHash(1), mkKeyHash(2), mkKeyHash(3), mkKeyHash(4))

	anchorPrincipal := mustParse(t, "dn.acme/anchors")
	res, err := VerifyValidatorQuorum(batch, anchorPrincipal, [32]byte{0xfe}, "Directory", time.Now())

	if !errors.Is(err, ErrQuorumInsufficient) {
		t.Fatalf("expected ErrQuorumInsufficient, got %v", err)
	}
	if res == nil {
		t.Fatal("expected QuorumResult even on insufficient")
	}
	if res.Required != 3 { // ceil(2*4/3) = 3
		t.Errorf("Required = %d, want 3", res.Required)
	}
	if res.Total != 4 {
		t.Errorf("Total = %d, want 4", res.Total)
	}
	if res.Verified != 0 {
		t.Errorf("Verified = %d, want 0", res.Verified)
	}
}

func TestVerifyValidatorQuorum_NoOperatorsKeybook(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(nullObserver{})
	batch := db.Begin(true)
	defer batch.Discard()

	anchorPrincipal := mustParse(t, "dn.acme/anchors")
	_, err := VerifyValidatorQuorum(batch, anchorPrincipal, [32]byte{0xfe}, "Directory", time.Now())
	if err == nil {
		t.Fatal("expected error when operators keybook is missing")
	}
}

func TestVerifyValidatorQuorum_ThresholdMath(t *testing.T) {
	cases := []struct {
		validators int
		expected   int
	}{
		{1, 1},  // ceil(2/3) = 1
		{2, 2},  // ceil(4/3) = 2
		{3, 2},  // ceil(6/3) = 2
		{4, 3},  // ceil(8/3) = 3
		{5, 4},  // ceil(10/3) = 4
		{7, 5},  // ceil(14/3) = 5
		{10, 7}, // ceil(20/3) = 7
		{16, 11},
		{100, 67},
	}
	for _, c := range cases {
		got := (2*c.validators + 2) / 3
		if got != c.expected {
			t.Errorf("threshold for %d validators: got %d, want %d", c.validators, got, c.expected)
		}
	}
}
