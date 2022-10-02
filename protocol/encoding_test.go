// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"bytes"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func makeExtraData(t *testing.T, fn func(*encoding.Writer)) []byte {
	buf := new(bytes.Buffer)
	wr := encoding.NewWriter(buf)
	fn(wr)
	_, _, err := wr.Reset(nil)
	require.NoError(t, err)
	return buf.Bytes()
}

func TestExtraData(t *testing.T) {
	txn1 := new(Transaction)
	txn1.Header.Principal = AccountUrl("foo")
	txn1.Header.Initiator = storage.MakeKey(t.Name())
	txn1.Header.Memo = "asdf"
	txn1.Header.Metadata = []byte("qwer")
	txn1.Header.extraData = makeExtraData(t, func(w *encoding.Writer) { w.WriteString(10, "extra header data") })

	body1 := new(SendTokens)
	txn1.Body = body1
	body1.To = []*TokenRecipient{{Url: AccountUrl("bar"), Amount: *big.NewInt(10)}, {Url: AccountUrl("baz"), Amount: *big.NewInt(20)}}
	body1.extraData = makeExtraData(t, func(w *encoding.Writer) { w.WriteString(10, "extra body data") })

	// Marshal and unmarshal
	data, err := txn1.MarshalBinary()
	require.NoError(t, err)
	txn2 := new(Transaction)
	require.NoError(t, txn2.UnmarshalBinary(data))
	body2 := txn2.Body.(*SendTokens)

	// Check
	assert.Equal(t, txn1.Header.extraData, txn2.Header.extraData)
	assert.Equal(t, body1.extraData, body2.extraData)
	assert.Equal(t, txn1.GetHash(), txn2.GetHash())
}

func TestJSONDecodeNull(t *testing.T) {
	t.Run("Ledger", func(t *testing.T) {
		ledger := new(SystemLedger)
		ledger.Index = 123
		ledger.Timestamp = time.Now()
		ledger.Url = AccountUrl("dn.acme", "ledger")
		ledger.Anchor = nil
		b, err := json.Marshal(ledger)
		require.NoError(t, err)

		l2 := new(SystemLedger)
		err = json.Unmarshal(b, l2)
		require.NoError(t, err)
		require.True(t, ledger.Equal(l2))
	})

	t.Run("Union", func(t *testing.T) {
		_, err := UnmarshalAccountJSON([]byte("null"))
		require.NoError(t, err)
	})
}
