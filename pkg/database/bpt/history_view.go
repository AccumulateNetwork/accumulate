// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// A historical view of the BPT: the tree as it stood at the end of a past block.
//
// The view is not a separate tree. It is the ordinary BPT with its node loads
// redirected — a node is read from its retained version when one postdates the
// view's height, and from the current record when none does, because a node with
// no later version has not changed and the current record IS the historical one.
// Everything above the load path — descent, hashing, receipt construction — is
// the code that serves current queries, unmodified. A second implementation of
// the walk would be a second thing that can be wrong.

// historyView is the height a BPT is being read at, and the root it must
// produce.
type historyView struct {
	height uint64
	root   [32]byte
}

// ViewAt returns a read-only view of the BPT as of the end of the given block,
// anchored at the given root.
//
// The root is supplied rather than derived because the BPT does not record which
// root belongs to which block — the partition ledger's BptChain does, and the
// caller reads it from there. Supplying it also makes the view self-checking:
// the root recomputed from the retained nodes must equal the root the ledger
// recorded, and [BPT.GetReceiptAt] refuses if it does not.
//
// The view is a distinct instance and shares no loaded nodes with the receiver,
// so reading history cannot disturb the current tree.
func (b *BPT) ViewAt(height uint64, root [32]byte) *BPT {
	v := New(nil, b.logger.L, b.store, b.key)
	v.view = &historyView{height: height, root: root}
	return v
}

// GetReceiptAt constructs a receipt for a key against the BPT as it stood at the
// end of the given block, terminating at the root recorded for that block.
//
// It never falls back to the current root. If the tree reconstructed from
// retained nodes does not hash to the root the ledger recorded, that is a
// retention defect and it returns an error rather than a receipt against
// whatever it did produce — a receipt that validates against the wrong root is
// worse than no receipt, because it is checkable and wrong.
func (b *BPT) GetReceiptAt(key *record.Key, height uint64, root [32]byte) (*merkle.Receipt, error) {
	v := b.ViewAt(height, root)

	r, err := v.GetReceipt(key)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	if !bytes.Equal(r.Anchor, root[:]) {
		return nil, errors.InternalError.WithFormat(
			"reconstructed root for block %d is %x, but the ledger recorded %x", height, r.Anchor, root)
	}
	return r, nil
}
