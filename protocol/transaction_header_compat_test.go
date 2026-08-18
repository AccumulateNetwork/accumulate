// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol_test

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Field numbers in TransactionHeader are positional: a field inserted above an
// existing one takes its number and shifts everything below. Anything already
// written to a database or signed by a client was encoded with the old
// numbering, so an insertion in the middle is a wire break, not an addition.
//
// v1.4.6.3 shipped exactly that. HashLock was placed between HoldUntil and
// Authorities, so HashLock took field 7 — the number Authorities had held
// since #3457 — and Authorities moved to 8. A header stored by any earlier
// release failed to decode:
//
//	field HashLock: failed to unmarshal value: failed to read field number:
//	field number is invalid
//
// and the same logical transaction hashed differently, which splits a
// mixed-version network and invalidates signatures made over the old hash.
//
// These goldens are the bytes and hash produced by v1.4.6.2. They are
// deliberately literals rather than values computed from the current code: a
// test that re-derives its expectation from the thing under test cannot
// detect this class of change.

// Produced by v1.4.6.2: a TransactionHeader with Principal
// acc://alice.acme/tokens and one additional authority acc://bob.acme/book.
const headerWithAuthoritiesV1462 = "01176163633a2f2f616c6963652e61636d652f746f6b656e73" +
	"07136163633a2f2f626f622e61636d652f626f6f6b"

// Produced by v1.4.6.2: the hash of a SendTokens transaction carrying that
// header.
const txnHashWithAuthoritiesV1462 = "50f78fe30e6a52d93e8dab7096ef927d61adc792102d5a043ed7ddf934a03e35"

func TestTransactionHeaderDecodesPreHashLockBytes(t *testing.T) {
	b, err := hex.DecodeString(headerWithAuthoritiesV1462)
	require.NoError(t, err)

	h := new(TransactionHeader)
	require.NoError(t, h.UnmarshalBinary(b), "a header stored by an earlier release must still decode")

	require.Equal(t, "acc://alice.acme/tokens", h.Principal.String())
	require.Len(t, h.Authorities, 1, "Authorities must still be read from field 7")
	require.Equal(t, "acc://bob.acme/book", h.Authorities[0].String())
	require.Nil(t, h.HashLock, "no released binary ever wrote a HashLock at field 7")
}

func TestTransactionHashWithAuthoritiesIsUnchanged(t *testing.T) {
	txn := new(Transaction)
	txn.Header.Principal = url.MustParse("acc://alice.acme/tokens")
	txn.Header.Authorities = []*url.URL{url.MustParse("acc://bob.acme/book")}
	txn.Body = new(SendTokens)

	require.Equal(t, txnHashWithAuthoritiesV1462, hex.EncodeToString(txn.GetHash()),
		"the hash of a transaction with additional authorities must not change between releases")
}

// TestTransactionHeaderHashLockRoundTrips covers the new field itself, so the
// fix cannot be mistaken for removing the feature.
func TestTransactionHeaderHashLockRoundTrips(t *testing.T) {
	h := new(TransactionHeader)
	h.Principal = url.MustParse("acc://alice.acme/tokens")
	h.Authorities = []*url.URL{url.MustParse("acc://bob.acme/book")}
	h.HashLock = &HashLockOptions{
		HashAlgorithm: HashAlgorithmSHA256,
		Hash:          make([]byte, 32),
	}

	b, err := h.MarshalBinary()
	require.NoError(t, err)

	g := new(TransactionHeader)
	require.NoError(t, g.UnmarshalBinary(b))
	require.Len(t, g.Authorities, 1)
	require.Equal(t, "acc://bob.acme/book", g.Authorities[0].String())
	require.NotNil(t, g.HashLock)
	require.Equal(t, HashAlgorithmSHA256, g.HashLock.HashAlgorithm)
}
