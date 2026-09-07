// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// anchorSignatures is the block's view of one anchor transaction's validator
// signatures: the set as stored before this block, plus what this block's
// copies brought. Every copy of an anchor carries one validator's signature,
// and eight validators send eight copies; reading and rewriting the stored
// set for each is eight rewrites of one record per block (#4224). The block
// reads the set once, counts the quorum from memory, and writes the set once
// — when the quorum is reached, so the anchor's execution commits with it, or
// at close for an anchor still below it.
type anchorSignatures struct {
	principal *url.URL
	hash      [32]byte
	sigs      []protocol.KeySignature // sorted by public key, as the stored set is
	dirty     bool                    // this block added a signature not yet written
}

// anchorSignaturesFor returns the block's signature set for an anchor,
// loading the stored set the first time the block asks about it.
func (b *Block) anchorSignaturesFor(batch *database.Batch, txn *protocol.Transaction) (*anchorSignatures, error) {
	hash := txn.ID().Hash()
	if s, ok := b.anchorSigs[hash]; ok {
		return s, nil
	}
	sigs, err := batch.Account(txn.Header.Principal).
		Transaction(hash).
		ValidatorSignatures().
		Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor signatures: %w", err)
	}
	s := &anchorSignatures{principal: txn.Header.Principal, hash: hash, sigs: sigs}
	if b.anchorSigs == nil {
		b.anchorSigs = map[[32]byte]*anchorSignatures{}
	}
	b.anchorSigs[hash] = s
	return s, nil
}

// add records a copy's signature. It reports whether the signer is new: a
// second copy from the same validator is no second signature.
func (s *anchorSignatures) add(sig protocol.KeySignature) bool {
	key := sig.GetPublicKey()
	i := sort.Search(len(s.sigs), func(i int) bool { return bytes.Compare(s.sigs[i].GetPublicKey(), key) >= 0 })
	if i < len(s.sigs) && bytes.Equal(s.sigs[i].GetPublicKey(), key) {
		return false
	}
	s.sigs = append(s.sigs, nil)
	copy(s.sigs[i+1:], s.sigs[i:])
	s.sigs[i] = sig
	s.dirty = true
	return true
}

// write stores the set if this block added to it.
func (s *anchorSignatures) write(batch *database.Batch) error {
	if !s.dirty {
		return nil
	}
	err := batch.Account(s.principal).Transaction(s.hash).ValidatorSignatures().Put(s.sigs)
	if err != nil {
		return errors.UnknownError.WithFormat("store anchor signatures: %w", err)
	}
	s.dirty = false
	return nil
}

// flushAnchorSignatures writes every anchor signature set this block added
// to and has not yet written — the anchors still below their quorum — once
// each, in one order on every node.
func (b *Block) flushAnchorSignatures() error {
	keys := make([][32]byte, 0, len(b.anchorSigs))
	for k, s := range b.anchorSigs {
		if s.dirty {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return bytes.Compare(keys[i][:], keys[j][:]) < 0 })
	for _, k := range keys {
		err := b.anchorSigs[k].write(b.Batch)
		if err != nil {
			return err
		}
	}
	return nil
}

// anchorIsAdmissible reports whether an anchor is authorized to execute.
//
// An anchor's gate is not a proof-to-a-DN-root like a synthetic's — it is a
// validator signature quorum, with one shortcut. It sits beside isAdmissible
// (#4169 step 3b) because staging needs ONE answer per message regardless of
// which kind of stream carries it, and because an anchor that is not
// authorized never reaches the sequence check: it is held, so its stream does
// not advance. Staging must not advance over it either.
//
// The shortcut first: a collection proof under a known directory root
// authorizes the anchor by itself (#4056), and that is the same terminal-anchor
// test a synthetic's proof gets, so it is the same function. If the proof's
// anchor has not arrived the anchor is not rejected — it falls through to the
// quorum, because the healing loop resubmits until a current anchor extends
// the destination's directory-root knowledge past the proven range.
//
// The quorum is counted from the block's view of the set (anchorSignatures),
// so copies arriving in the same block reach it in that block.
func (b *Block) anchorIsAdmissible(batch *database.Batch, proof *protocol.AnnotatedReceipt, txn *protocol.Transaction, source *url.URL) (bool, error) {
	if proof != nil {
		ok, err := b.Executor.isAdmissible(batch, proof)
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
		}
		if ok {
			return true, nil
		}
		// Not yet anchored — fall through to the signature quorum.
	}

	partition, ok := protocol.ParsePartitionUrl(source)
	if !ok {
		return false, errors.BadRequest.WithFormat("source %v is not a partition", source)
	}
	sigs, err := b.anchorSignaturesFor(batch, txn)
	if err != nil {
		return false, err
	}
	return uint64(len(sigs.sigs)) >= b.Executor.globals().Active.ValidatorThreshold(partition), nil
}
