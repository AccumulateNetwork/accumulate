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
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestLiveSightedReceived proves what the peer's stored anchor ledger holds in
// Sequence[].Received, as against what it serves there. queryAccount runs the
// loaded state through withSighted (internal/api/v3/sequence_ledger.go), which
// overwrites Received with a value derived from staging, while the receipt it
// serves in the same call is built from the STORED state. The body served
// therefore does not hash to the leaf the receipt proves.
func TestLiveSightedReceived(t *testing.T) {
	q := live(t)
	ctx := context.Background()
	pu := url.MustParse("acc://bvn-BVN1.acme")
	u := pu.JoinPath(protocol.AnchorPool)

	rec, err := q.QueryAccount(ctx, u, &api.DefaultQuery{
		IncludeReceipt: &api.ReceiptOptions{ForAny: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	al := rec.Account.(*protocol.AnchorLedger)
	want := hex.EncodeToString(rec.Receipt.Receipt.Start)

	b, _ := al.MarshalBinary()
	h := sha256.Sum256(b)
	t.Logf("served body hashes to %s", hex.EncodeToString(h[:]))
	t.Logf("receipt proves       %s", want)
	for _, s := range al.Sequence {
		t.Logf("served Sequence[%v]: Produced=%d Received=%d Delivered=%d", s.Url, s.Produced, s.Received, s.Delivered)
	}

	if len(al.Sequence) != 1 {
		t.Skipf("this probe assumes a single sequence entry, got %d", len(al.Sequence))
	}
	s := al.Sequence[0]
	saved := s.Received
	for cand := uint64(0); cand <= 4000; cand++ {
		s.Received = cand
		bb, _ := al.MarshalBinary()
		hh := sha256.Sum256(bb)
		if hex.EncodeToString(hh[:]) == want {
			t.Logf("*** the STORED anchor ledger has Sequence[%v].Received=%d; the API served %d ***",
				s.Url, cand, saved)
			t.Logf("*** everything else in the body is byte-identical; withSighted is the only difference ***")
			return
		}
	}
	s.Received = saved
	t.Errorf("no value of Received reproduces the receipt's leaf; the divergence is elsewhere")
}
