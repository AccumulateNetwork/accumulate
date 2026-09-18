// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestLiveMainStateHash asks the peer for the account WITH a receipt, in one
// call, and compares sha256(MarshalBinary(account)) -- which is element 0 of
// the account hasher, and therefore Receipt.Start -- with the Start the peer
// put in the receipt it served alongside.
func TestLiveMainStateHash(t *testing.T) {
	q := live(t)
	ctx := context.Background()
	pu := url.MustParse("acc://bvn-BVN1.acme")

	for _, name := range []string{protocol.AnchorPool, protocol.Ledger, protocol.Synthetic} {
		u := pu.JoinPath(name)
		rec, err := q.QueryAccount(ctx, u, &api.DefaultQuery{
			IncludeReceipt: &api.ReceiptOptions{ForAny: true},
		})
		if err != nil {
			t.Fatalf("%v: %v", u, err)
		}
		b, err := rec.Account.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(b)
		js, _ := json.Marshal(rec.Account)
		t.Logf("%v", u)
		t.Logf("  receipt.LocalBlock = %d  receipt.Partition = %q", rec.Receipt.LocalBlock, rec.Receipt.Partition)
		t.Logf("  peer  Receipt.Start      = %s", hex.EncodeToString(rec.Receipt.Receipt.Start))
		t.Logf("  local sha256(marshal)    = %s   MATCH=%v", hex.EncodeToString(h[:]),
			hex.EncodeToString(h[:]) == hex.EncodeToString(rec.Receipt.Receipt.Start))
		t.Logf("  binary  = %s", hex.EncodeToString(b))
		t.Logf("  json    = %s", js)

		if al, ok := rec.Account.(*protocol.AnchorLedger); ok {
			t.Logf("  MajorBlockTime == time.Time{} : %v  (loc=%v)", al.MajorBlockTime == (time.Time{}), al.MajorBlockTime.Location())
			// Brute force the one field that moves every anchor.
			save := al.LastAnchorBlock
			for d := uint64(0); d <= 24; d++ {
				for _, cand := range []uint64{save - d, save + d} {
					al.LastAnchorBlock = cand
					bb, _ := al.MarshalBinary()
					hh := sha256.Sum256(bb)
					if hex.EncodeToString(hh[:]) == hex.EncodeToString(rec.Receipt.Receipt.Start) {
						t.Logf("  *** the peer hashed LastAnchorBlock=%d, and served LastAnchorBlock=%d ***", cand, save)
					}
				}
			}
			al.LastAnchorBlock = save
			// And the other mover.
			save2 := al.MinorBlockSequenceNumber
			for d := uint64(0); d <= 24; d++ {
				for _, cand := range []uint64{save2 - d, save2 + d} {
					al.MinorBlockSequenceNumber = cand
					bb, _ := al.MarshalBinary()
					hh := sha256.Sum256(bb)
					if hex.EncodeToString(hh[:]) == hex.EncodeToString(rec.Receipt.Receipt.Start) {
						t.Logf("  *** the peer hashed MinorBlockSequenceNumber=%d, served %d ***", cand, save2)
					}
				}
			}
			al.MinorBlockSequenceNumber = save2
			for _, s := range al.Sequence {
				save3 := s.Delivered
				for d := uint64(0); d <= 24; d++ {
					for _, cand := range []uint64{save3 - d, save3 + d} {
						s.Delivered = cand
						bb, _ := al.MarshalBinary()
						hh := sha256.Sum256(bb)
						if hex.EncodeToString(hh[:]) == hex.EncodeToString(rec.Receipt.Receipt.Start) {
							t.Logf("  *** the peer hashed %v Delivered=%d, served %d ***", s.Url, cand, save3)
						}
					}
				}
				s.Delivered = save3
				save4 := s.Received
				for d := uint64(0); d <= 24; d++ {
					for _, cand := range []uint64{save4 - d, save4 + d} {
						s.Received = cand
						bb, _ := al.MarshalBinary()
						hh := sha256.Sum256(bb)
						if hex.EncodeToString(hh[:]) == hex.EncodeToString(rec.Receipt.Receipt.Start) {
							t.Logf("  *** the peer hashed %v Received=%d, served %d ***", s.Url, cand, save4)
						}
					}
				}
				s.Received = save4
			}
		}
	}
}
