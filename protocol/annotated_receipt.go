// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

// MaxReceiptListElements bounds the number of elements a collection proof may
// carry, limiting the memory an attacker can force a validator to allocate
// while checking one (#4048).
const MaxReceiptListElements = 4096

// TerminalAnchor returns the anchor the proof terminates at — the hash that
// must appear in the destination's DN anchor pool. For an individual receipt
// that is the receipt's anchor; for a collection proof it is the anchor of the
// continued receipt if present, otherwise of the list's own receipt.
func (r *AnnotatedReceipt) TerminalAnchor() []byte {
	switch {
	case r.ReceiptList == nil:
		if r.Receipt == nil {
			return nil
		}
		return r.Receipt.Anchor
	case r.ReceiptList.ContinuedReceipt != nil:
		return r.ReceiptList.ContinuedReceipt.Anchor
	case r.ReceiptList.Receipt != nil:
		return r.ReceiptList.Receipt.Anchor
	default:
		return nil
	}
}
