// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package fastsync implements the client side of fast validator deployment
// (#4058): header-first sync that verifies the directory's major-block spine
// forward from a trust anchor, then proves account states into the verified
// roots. See docs/plans/fast-validator-deployment.md.
package fastsync

import (
	"bytes"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Spine walks the directory's major-block spine, verifying each major block's
// closing anchor against the validator set tracked by induction: each anchor
// is verified with the set as of the previous anchor, then the network
// updates it carries are applied. The initial state comes from the trust
// anchor (the pinned genesis snapshot).
type Spine struct {
	globals *network.GlobalValues

	// NextMajor is the index of the next major block to verify.
	NextMajor uint64

	// LastMinorBlock is the minor block of the last verified anchor.
	LastMinorBlock uint64

	// RootChainAnchor is the root chain anchor of the last verified anchor.
	RootChainAnchor [32]byte

	// StateTreeAnchor is the state tree (BPT) anchor of the last verified
	// anchor.
	StateTreeAnchor [32]byte
}

// NewSpine constructs a spine walk starting from the given trust anchor
// state, expecting major block `next` as the first record.
func NewSpine(genesis *network.GlobalValues, next uint64) (*Spine, error) {
	if genesis == nil || genesis.Network == nil || genesis.Globals == nil {
		return nil, errors.BadRequest.With("missing network definition or globals")
	}
	if next == 0 {
		return nil, errors.BadRequest.With("major blocks are 1-based")
	}
	return &Spine{globals: genesis.Copy(), NextMajor: next}, nil
}

// Globals returns the validator set and globals as of the last verified major
// block.
func (s *Spine) Globals() *network.GlobalValues { return s.globals }

// Advance verifies one major header record and advances the spine. The record
// must be the next major block in sequence. Any invalid signature rejects the
// record outright — archived quorum signatures were verified when the anchor
// executed, so an invalid one means the server tampered with the response.
func (s *Spine) Advance(r *private.MajorHeaderRecord) error {
	// The record must be well-formed and next in sequence
	if r == nil || r.Entry == nil || r.Anchor == nil {
		return errors.BadRequest.With("incomplete major header record")
	}
	if r.Index != s.NextMajor {
		return errors.Conflict.WithFormat("expected major block %d, got %d", s.NextMajor, r.Index)
	}
	if r.Entry.BlockIndex != r.Index {
		return errors.BadRequest.WithFormat("index entry is for major block %d, not %d", r.Entry.BlockIndex, r.Index)
	}

	// The anchor must be a directory self-anchor
	txnMsg, ok := r.Anchor.Message.(*messaging.TransactionMessage)
	if !ok {
		return errors.BadRequest.With("anchor is not a transaction")
	}
	body, ok := txnMsg.Transaction.Body.(*protocol.DirectoryAnchor)
	if !ok {
		return errors.BadRequest.WithFormat("anchor is %v, not a directory anchor", txnMsg.Transaction.Body.Type())
	}
	if !protocol.DnUrl().Equal(r.Anchor.Source) ||
		!protocol.DnUrl().Equal(r.Anchor.Destination) ||
		!protocol.DnUrl().JoinPath(protocol.AnchorPool).Equal(txnMsg.Transaction.Header.Principal) {
		return errors.BadRequest.With("anchor is not a directory self-anchor")
	}
	if body.MinorBlockIndex <= s.LastMinorBlock {
		return errors.Conflict.WithFormat("anchor for minor block %d does not advance past %d", body.MinorBlockIndex, s.LastMinorBlock)
	}

	// Apply the record's proven network updates to a candidate state — this
	// is the induction step that tracks the validator set forward. The
	// receipts bind each update to this anchor's root chain anchor, so the
	// updates inherit the anchor's quorum trust.
	candidate := s.globals.Copy()
	for _, u := range r.Updates {
		err := applyProvenUpdate(candidate, u, body.RootChainAnchor)
		if err != nil {
			return errors.UnknownError.WithFormat("major block %d: %w", r.Index, err)
		}
	}

	// Verify the quorum. Updates take effect when they execute, so an anchor
	// after a mid-window set change is signed by the post-update set — but a
	// change in the closing block itself is signed by the pre-update set, so
	// accept either.
	err := verifyQuorum(candidate, r)
	if err != nil && len(r.Updates) > 0 {
		err = verifyQuorum(s.globals, r)
	}
	if err != nil {
		return err
	}

	s.globals = candidate
	s.NextMajor++
	s.LastMinorBlock = body.MinorBlockIndex
	s.RootChainAnchor = body.RootChainAnchor
	s.StateTreeAnchor = body.StateTreeAnchor
	return nil
}

func verifyQuorum(g *network.GlobalValues, r *private.MajorHeaderRecord) error {
	seen := map[[32]byte]bool{}
	for _, sig := range r.Signatures {
		if !sig.Verify(nil, r.Anchor) {
			return errors.Unauthenticated.WithFormat("major block %d: invalid signature", r.Index)
		}
		v, ok := findDirectoryValidator(g, sig.GetPublicKey())
		if !ok {
			return errors.Unauthorized.WithFormat("major block %d: signer is not an active directory validator", r.Index)
		}
		seen[v.PublicKeyHash] = true
	}
	threshold := g.ValidatorThreshold(protocol.Directory)
	if uint64(len(seen)) < threshold {
		return errors.Unauthorized.WithFormat("major block %d: quorum not met: %d of %d validator signatures", r.Index, len(seen), threshold)
	}
	return nil
}

func findDirectoryValidator(g *network.GlobalValues, key []byte) (*protocol.ValidatorInfo, bool) {
	for _, v := range g.Network.Validators {
		if !bytes.Equal(v.PublicKey, key) {
			continue
		}
		for _, p := range v.Partitions {
			if p.Active && strings.EqualFold(p.ID, protocol.Directory) {
				return v, true
			}
		}
	}
	return nil, false
}

// applyProvenUpdate verifies that the update's receipt binds its transaction
// to the anchor's root chain anchor, then applies it. A stale network
// definition (version at or below the current) is skipped rather than
// rejected: every definition entry is the complete state, so a skipped no-op
// is self-healing, and genesis writes fall in the first window.
func applyProvenUpdate(g *network.GlobalValues, u *private.NetworkUpdateProof, root [32]byte) error {
	if u.Transaction == nil || u.Receipt == nil {
		return errors.BadRequest.With("incomplete network update proof")
	}
	if !bytes.Equal(u.Receipt.Start, u.Transaction.GetHash()) {
		return errors.Unauthenticated.With("network update receipt does not start at the transaction")
	}
	if !bytes.Equal(u.Receipt.Anchor, root[:]) {
		return errors.Unauthenticated.With("network update receipt does not end at the anchor's root")
	}
	if !u.Receipt.Validate(nil) {
		return errors.Unauthenticated.With("invalid network update receipt")
	}

	wd, ok := u.Transaction.Body.(*protocol.WriteData)
	if !ok {
		// Other transaction types do not affect the consensus validator set
		return nil
	}

	principal := u.Transaction.Header.Principal
	switch {
	case protocol.DnUrl().JoinPath(protocol.Network).Equal(principal):
		def := new(protocol.NetworkDefinition)
		if entry := wd.Entry; entry != nil && len(entry.GetData()) == 1 && def.UnmarshalBinary(entry.GetData()[0]) == nil && def.Version <= g.Network.Version {
			return nil // Stale or genesis definition — a complete-state no-op
		}
		return g.ParseNetwork(wd.Entry)
	case protocol.DnUrl().JoinPath(protocol.Globals).Equal(principal):
		return g.ParseGlobals(wd.Entry)
	}
	return nil
}
