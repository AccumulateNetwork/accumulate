// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !debug

// Not built with -tags debug: the debug observer collapses an account's state
// components into one hash, so nothing is retained and there is no body to
// test. historical_body_debug_test.go tests what that build does instead.

package indexing_test

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

// A historical answer serves the account's main state as of the block its
// receipt is for. These tests pin what that body is and when there is none.

// requireServesBody asserts the proof carries a body, that the body hashes to
// where the receipt starts, and that the receipt validates.
func requireServesBody(t *testing.T, proof *indexing.HistoricalStateProof) {
	t.Helper()
	require.True(t, proof.StartsAtMainState)
	require.NotNil(t, proof.State, "a main-state start with no body to recompute it from")
	data, err := proof.State.MarshalBinary()
	require.NoError(t, err)
	h := sha256.Sum256(data)
	require.Equal(t, h[:], proof.Receipt.Start, "the body served does not hash to the receipt's start")
	require.True(t, proof.Receipt.Validate(nil))
}

// At an earlier change the body served is the body THEN, which differs from
// the body now.
func TestHistoricalBody_IsTheStateAtTheBlock(t *testing.T) {
	sim, lite := changingLite(t, 10_000, 6)

	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		account := batch.Account(lite)
		blocks, err := account.RetainedStateReceiptBlocks().Get()
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(blocks), 2)

		now, err := account.Main().Get()
		require.NoError(t, err)
		nowBytes, err := now.MarshalBinary()
		require.NoError(t, err)

		for _, b := range blocks[:len(blocks)-1] {
			proof, err := indexing.HistoricalAccountStateProof(bvn0, batch, account, b)
			require.NoErrorf(t, err, "block %d", b)
			requireServesBody(t, proof)

			then, err := proof.State.MarshalBinary()
			require.NoError(t, err)
			require.NotEqualf(t, nowBytes, then, "block %d: served the body as it is now", b)
		}
	})
}

// A body is retained exactly where its receipt is, and pruned with it.
func TestHistoricalBody_RetainedAndPrunedWithTheReceipt(t *testing.T) {
	sim, lite := changingLite(t, 6, 12)

	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		account := batch.Account(lite)
		keep, err := account.RetainedStateReceiptBlocks().Get()
		require.NoError(t, err)
		require.NotEmpty(t, keep)
		kept := map[uint64]bool{}
		for _, b := range keep {
			kept[b] = true
		}

		latest := keep[len(keep)-1]
		unpruned, prunedSeen := 0, 0
		for b := uint64(1); b <= latest; b++ {
			body, err := account.RetainedMainState(b).Get()
			if kept[b] {
				require.NoErrorf(t, err, "block %d: receipt retained without its body", b)
				require.NotEmptyf(t, body, "block %d: receipt retained without its body", b)
				r, err := account.RetainedStateReceipt(b).Get()
				require.NoError(t, err)
				h := sha256.Sum256(body)
				require.Equalf(t, h[:], r.Start, "block %d: the body is not the one the receipt starts at", b)
				continue
			}
			if err == nil && len(body) > 0 {
				t.Fatalf("block %d: a body outlived its receipt", b)
			}
			if err != nil {
				require.Truef(t, errors.Is(err, errors.NotFound), "block %d: %v", b, err)
			}
			// Pruning writes nil, which reads back as an empty receipt
			r, err := account.RetainedStateReceipt(b).Get()
			switch {
			case err == nil && r != nil && len(r.Start) > 0:
				unpruned++
			case err == nil:
				prunedSeen++
			}
		}
		require.Zero(t, unpruned, "a receipt outside the kept set was not pruned")
		require.NotZero(t, prunedSeen, "nothing was pruned, so this proves nothing about pruning")
	})
}

// At the latest change the account's entry now IS its entry then, so the
// current body is the body at the block by the hashes. A corrupted retained
// receipt there is not used; the current body still serves.
func TestHistoricalBody_UnchangedSinceServesTheCurrentBody(t *testing.T) {
	sim, lite := changingLite(t, 10_000, 4)

	var block uint64
	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		blocks, err := batch.Account(lite).RetainedStateReceiptBlocks().Get()
		require.NoError(t, err)
		require.NotEmpty(t, blocks)
		block = blocks[len(blocks)-1]
	})

	Update(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		account := batch.Account(lite)
		r, err := account.RetainedStateReceipt(block).Get()
		require.NoError(t, err)
		bad := r.Copy()
		bad.Anchor = append([]byte(nil), r.Anchor...)
		bad.Anchor[0] ^= 0xff
		require.NoError(t, account.RetainedStateReceipt(block).Put(bad))
	})

	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		account := batch.Account(lite)
		proof, err := indexing.HistoricalAccountStateProof(bvn0, batch, account, block)
		require.NoError(t, err)
		requireServesBody(t, proof)

		now, err := account.Main().Get()
		require.NoError(t, err)
		require.True(t, EqualAccount(now, proof.State), "the current body should serve for an account unchanged since the block")
	})
}

// A receipt retained before bodies were - no body beside it - is not a start a
// caller can check. Where the account has changed since, there is no body to
// serve at all, and the answer says so rather than serving the current one.
func TestHistoricalBody_ReceiptWithoutBodyServesNoBody(t *testing.T) {
	sim, lite := changingLite(t, 10_000, 6)

	var block uint64
	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		blocks, err := batch.Account(lite).RetainedStateReceiptBlocks().Get()
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(blocks), 2)
		block = blocks[len(blocks)-2]
	})

	Update(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		require.NoError(t, batch.Account(lite).RetainedMainState(block).Put(nil))
	})

	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		proof, err := indexing.HistoricalAccountStateProof(bvn0, batch, batch.Account(lite), block)
		require.NoError(t, err, "no body is a capability limit, not a reason to deny the entry-rooted proof")
		require.False(t, proof.StartsAtMainState)
		require.Nil(t, proof.State, "served a body the receipt does not start at")
		require.True(t, proof.Receipt.Validate(nil))
	})
}

// The body served is the retained bytes decoded and encoded again, and it is
// used only if that encoding hashes to where the receipt starts. So the form the
// retained bytes took does not matter: the decoder accepts an overlong varint
// the encoder never writes, and bytes carrying one still decode to the account
// the BPT hashed. The body IS served, its encoding is the canonical one, and it
// hashes to Receipt.Start - by the hash, not by storage.
func TestHistoricalBody_OverlongRetainedBytesServeTheHashedBody(t *testing.T) {
	sim, lite := changingLite(t, 10_000, 6)

	var block uint64
	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		blocks, err := batch.Account(lite).RetainedStateReceiptBlocks().Get()
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(blocks), 2)
		block = blocks[len(blocks)-2] // changed since, so the current body cannot stand in
	})

	var retained []byte
	Update(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		account := batch.Account(lite)
		encoded, err := account.RetainedMainState(block).Get()
		require.NoError(t, err)
		retained = append([]byte(nil), encoded...)
		// Field 1 is the account type: 0x01 then its value as a one-byte
		// uvarint. Write the same value as an overlong two-byte uvarint.
		require.Equal(t, byte(0x01), encoded[0])
		require.Less(t, encoded[1], byte(0x80))
		overlong := append([]byte{0x01, encoded[1] | 0x80, 0x00}, encoded[2:]...)
		require.NotEqual(t, encoded, overlong)
		require.NoError(t, account.RetainedMainState(block).Put(overlong))
	})

	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		proof, err := indexing.HistoricalAccountStateProof(bvn0, batch, batch.Account(lite), block)
		require.NoError(t, err)
		requireServesBody(t, proof)

		served, err := proof.State.MarshalBinary()
		require.NoError(t, err)
		require.Equal(t, retained, served, "the served encoding is not the one the BPT hashed")
	})
}

// Retained bytes that do not decode are a body the node cannot produce, and
// nothing more: the entry hash and its path to the root are proven regardless,
// and a hash the node can prove is never withheld. The answer is body-less, as
// for any block whose body the node cannot produce - not an error.
func TestHistoricalBody_UndecodableRetainedBytesServeNoBody(t *testing.T) {
	sim, lite := changingLite(t, 10_000, 6)

	var block uint64
	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		blocks, err := batch.Account(lite).RetainedStateReceiptBlocks().Get()
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(blocks), 2)
		block = blocks[len(blocks)-2] // changed since, so the current body cannot stand in
	})

	Update(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		// Field 1 with a type value no account has
		bad := []byte{0x01, 0x7f}
		_, err := UnmarshalAccount(bad)
		require.Error(t, err, "precondition: the bytes do not decode")
		require.NoError(t, batch.Account(lite).RetainedMainState(block).Put(bad))
	})

	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		proof, err := indexing.HistoricalAccountStateProof(bvn0, batch, batch.Account(lite), block)
		require.NoError(t, err, "a body that does not decode is a capability limit, not a reason to deny the entry-rooted proof")
		require.False(t, proof.StartsAtMainState)
		require.Nil(t, proof.State)
		require.True(t, proof.Receipt.Validate(nil))
	})
}
