// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// BuildCollectionProof builds an [protocol.AnnotatedReceipt] carrying a
// collection proof (#4048): a [merkle.ReceiptList] proving every entry of
// chain in [start, end], in order, at proven absolute indices. The list
// carries the counted merkle state at start-1, so replaying its elements onto
// that state binds each element's index — sequence numbers are proven without
// additional machinery.
//
// continued must be a receipt from the list's anchor (the chain's root at
// end) to a directory-anchored root; the destination checks the terminal
// anchor against its DN anchor pool exactly as it does for an individual
// receipt. Pass nil only when the chain's own root at end is itself the
// anchored root.
//
// Collection proofs are currently emitted only by the recovery path
// (SequenceRange): steady-state synthetics dispatch one at a time, where a
// single-element list has no advantage over a per-message receipt. Batching
// steady-state emission is #4055 — a sender-side change only, since the
// v2-kourou accept path already handles both proof forms.
func BuildCollectionProof(chain *merkle.Chain, start, end int64, continued *merkle.Receipt) (*protocol.AnnotatedReceipt, error) {
	if end-start+1 > protocol.MaxReceiptListElements {
		return nil, errors.BadRequest.WithFormat("range [%d, %d] exceeds %d elements", start, end, protocol.MaxReceiptListElements)
	}

	list, err := merkle.GetReceiptList(chain, start, end)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("build receipt list: %w", err)
	}

	if continued != nil {
		if !bytes.Equal(continued.Start, list.Receipt.Anchor) {
			return nil, errors.InternalError.WithFormat("continuation does not chain: starts at %x, list anchors at %x", continued.Start, list.Receipt.Anchor)
		}
		list.ContinuedReceipt = continued
	}

	if !list.Validate(nil) {
		return nil, errors.InternalError.With("built an invalid receipt list")
	}

	return &protocol.AnnotatedReceipt{
		ReceiptList: list,
		Anchor: &protocol.AnchorMetadata{
			Account: protocol.DnUrl(),
		},
	}, nil
}
