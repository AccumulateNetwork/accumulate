// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// testChains creates a source chain with n entries and a "DN" chain that
// anchors the source's root after every entry, mirroring how partition roots
// are anchored into the directory. It returns the chains, the source entries,
// and the source root after each entry.
func testChains(t *testing.T, n int) (src, dn *merkle.Chain, entries, roots [][]byte) {
	t.Helper()
	store := memory.New(nil)
	tx := store.Begin(nil, true)
	t.Cleanup(tx.Discard)
	rs := keyvalue.RecordStore{Store: tx}

	src = merkle.NewChain(nil, rs, record.NewKey("src"), 8, merkle.ChainTypeTransaction, "src")
	dn = merkle.NewChain(nil, rs, record.NewKey("dn"), 8, merkle.ChainTypeAnchor, "dn")

	for i := 0; i < n; i++ {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(i))
		h := sha256.Sum256(b[:])
		entries = append(entries, h[:])
		require.NoError(t, src.AddEntry(h[:], false))

		ms, err := src.Head().Get()
		require.NoError(t, err)
		root := ms.Anchor()
		roots = append(roots, root)
		require.NoError(t, dn.AddEntry(root, false))
	}
	return src, dn, entries, roots
}

func TestBuildCollectionProof(t *testing.T) {
	const size = 100
	src, dn, entries, roots := testChains(t, size)

	dnHead, err := dn.Head().Get()
	require.NoError(t, err)

	for _, r := range [][2]int64{{0, 0}, {0, 9}, {37, 61}, {90, size - 1}} {
		start, end := r[0], r[1]

		// The list's receipt anchors at the source root as of entry `end`;
		// continue from there to the DN root, as the conductor would using the
		// receipt machinery that serves individual proofs today.
		continued, err := dn.Receipt(end, dnHead.Count-1)
		require.NoError(t, err)

		proof, err := BuildCollectionProof(src, start, end, continued)
		require.NoError(t, err)
		require.NotNil(t, proof.ReceiptList)
		require.Nil(t, proof.Receipt)

		// The proof must validate and cover exactly the requested entries
		require.True(t, proof.ReceiptList.Validate(nil))
		for i := start; i <= end; i++ {
			require.True(t, proof.ReceiptList.Included(entries[i]))
		}
		bogus := sha256.Sum256([]byte("not in the chain"))
		require.False(t, proof.ReceiptList.Included(bogus[:]))

		// The terminal anchor is the DN root, same trust root as an
		// individual receipt
		require.Equal(t, continued.Anchor, proof.TerminalAnchor())

		// The counted start state binds absolute indices
		require.Equal(t, start, proof.ReceiptList.MerkleState.Count)

		// The proof must survive a wire round trip intact
		b, err := proof.MarshalBinary()
		require.NoError(t, err)
		decoded := new(protocol.AnnotatedReceipt)
		require.NoError(t, decoded.UnmarshalBinary(b))
		require.True(t, proof.Equal(decoded))
		require.True(t, decoded.ReceiptList.Validate(nil))

		_ = roots
	}
}

func TestBuildCollectionProofRejects(t *testing.T) {
	const size = 20
	src, dn, _, _ := testChains(t, size)

	dnHead, err := dn.Head().Get()
	require.NoError(t, err)

	// A range larger than the DoS bound is rejected before any work is done
	_, err = BuildCollectionProof(src, 0, protocol.MaxReceiptListElements, nil)
	require.ErrorContains(t, err, "exceeds")

	// A continuation that does not chain from the list's anchor is rejected
	continued, err := dn.Receipt(3, dnHead.Count-1) // anchors entry 3, list ends at 10
	require.NoError(t, err)
	_, err = BuildCollectionProof(src, 0, 10, continued)
	require.ErrorContains(t, err, "does not chain")
}
