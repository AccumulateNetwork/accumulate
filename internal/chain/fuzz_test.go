package chain_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

func addTransaction(f *testing.F, header TransactionHeader, body TransactionBody) {
	dataHeader, err := header.MarshalBinary()
	require.NoError(f, err)
	dataBody, err := body.MarshalBinary()
	require.NoError(f, err)
	f.Add(dataHeader, dataBody)
}

type bodyPtr[T any] interface {
	*T
	TransactionBody
}

func generateTransaction[PT bodyPtr[T], T any](t *testing.T, dataHeader, dataBody []byte) (*Transaction, PT) {
	var header TransactionHeader
	body := PT(new(T))
	if header.UnmarshalBinary(dataHeader) != nil {
		t.Skip()
	}
	if body.UnmarshalBinary(dataBody) != nil {
		t.Skip()
	}
	return &Transaction{Header: header, Body: body}, body
}

func FuzzSendTokens(f *testing.F) {
	addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &SendTokens{To: []*TokenRecipient{{Url: AccountUrl("bar"), Amount: *big.NewInt(123)}}})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()

		txn, body := generateTransaction[*SendTokens](t, dataHeader, dataBody)
		if txn.Header.Principal == nil {
			t.Skip()
		}

		sender := new(TokenAccount)
		sender.AddAuthority(&url.URL{Authority: Unknown})
		sender.Url = txn.Header.Principal
		for _, to := range body.To {
			sender.Balance.Add(&sender.Balance, &to.Amount)
		}

		db := database.OpenInMemory(nil)
		err := TryMakeAccount(t, db, sender)
		if err != nil {
			t.Skip()
		}

		st, err := chain.NewStateManagerForFuzz(t, db, txn)
		if err != nil {
			t.Skip()
		}
		defer st.Discard()

		_, _ = (chain.SendTokens{}).Validate(st, &chain.Delivery{Transaction: txn})
	})
}
