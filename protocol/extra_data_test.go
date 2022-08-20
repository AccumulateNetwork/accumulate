package protocol

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
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
