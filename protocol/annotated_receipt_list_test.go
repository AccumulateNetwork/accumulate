// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestAnnotatedReceiptListRoundTrip(t *testing.T) {
	a := &AnnotatedReceipt{
		ReceiptList: &merkle.ReceiptList{
			MerkleState: new(merkle.State),
			Elements:    [][]byte{{1, 2, 3}, {4, 5, 6}},
			Receipt:     new(merkle.Receipt),
		},
		Anchor: &AnchorMetadata{Account: DnUrl()},
	}
	b, err := a.MarshalBinary()
	require.NoError(t, err)

	c := new(AnnotatedReceipt)
	require.NoError(t, c.UnmarshalBinary(b))
	require.NotNil(t, c.ReceiptList, "ReceiptList must survive the wire")
	require.Len(t, c.ReceiptList.Elements, 2)
	require.True(t, a.Equal(c))
}

// Adding ReceiptList must not disturb the existing encoding. It is field 3, so
// a value that does not set it encodes byte-for-byte as it did before, and a
// node that predates the field still reads Anchor from field 2.
func TestAnnotatedReceiptFieldNumbersUnchanged(t *testing.T) {
	a := &AnnotatedReceipt{
		Receipt: new(merkle.Receipt),
		Anchor:  &AnchorMetadata{Account: DnUrl()},
	}
	b, err := a.MarshalBinary()
	require.NoError(t, err)

	// Field 1 (Receipt) then field 2 (Anchor); no field 3 present.
	require.Equal(t, byte(1), b[0], "Receipt must remain field 1")
	require.NotContains(t, string(b[:1]), "\x03", "no field 3 when ReceiptList is unset")

	c := new(AnnotatedReceipt)
	require.NoError(t, c.UnmarshalBinary(b))
	require.Nil(t, c.ReceiptList)
	require.True(t, a.Equal(c))
}
