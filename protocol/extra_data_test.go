package protocol

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func TestExtraData(t *testing.T) {
	txn1 := new(Transaction)
	txn1.Header.Principal = url.MustParse("foo")
	txn1.Header.Initiator = storage.MakeKey(t.Name())
	txn1.Header.Memo = "asdf"
	txn1.Header.Metadata = []byte("qwer")
	txn1.Header.extraData = []byte("extra header data")

	body1 := new(SendTokens)
	txn1.Body = body1
	body1.To = []*TokenRecipient{{Url: url.MustParse("bar"), Amount: *big.NewInt(10)}, {Url: url.MustParse("baz"), Amount: *big.NewInt(20)}}
	body1.extraData = []byte("extra body data")

	// Marshal and unmarshal
	data, err := txn1.MarshalBinary()
	require.NoError(t, err)
	txn2 := new(Transaction)
	require.NoError(t, txn2.UnmarshalBinary(data))
	body2 := txn2.Body.(*SendTokens)

	// Check
	require.Equal(t, txn1.Header.extraData, txn2.Header.extraData)
	require.Equal(t, body1.extraData, body2.extraData)
	require.Equal(t, txn1.GetHash(), txn2.GetHash())
}
