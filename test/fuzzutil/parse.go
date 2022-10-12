// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package fuzzutil

import (
	"encoding"
	"math/big"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

type F interface {
	helpers.T
	Add(...interface{})
}

type TB interface {
	helpers.T
	Skip(args ...any)
	Skipf(format string, args ...any)
}

func AddValue(f F, v encoding.BinaryMarshaler) {
	data, err := v.MarshalBinary()
	require.NoError(f, err)
	f.Add(data)
}

func MustParseUrl(t TB, v string) *url.URL {
	if v == "" {
		return nil
	}
	u, err := url.Parse(v)
	if err != nil {
		t.Skip()
	}
	return u
}

func MustParseHash(t TB, v []byte) [32]byte {
	if len(v) != 32 {
		t.Skip()
	}
	return *(*[32]byte)(v)
}

func MustParseBigInt(t TB, v string) big.Int {
	u := new(big.Int)
	u, ok := u.SetString(v, 10)
	if !ok {
		t.Skip()
	}
	return *u
}

func AddTransaction(f F, list map[[32]byte]bool, header protocol.TransactionHeader, body protocol.TransactionBody) {
	dataHeader, err := header.MarshalBinary()
	require.NoError(f, err)
	dataBody, err := body.MarshalBinary()
	require.NoError(f, err)
	f.Add(dataHeader, dataBody)
	h := (&protocol.Transaction{Header: header, Body: body}).ID().Hash()
	list[h] = true
}

type bodyPtr[T any] interface {
	*T
	protocol.TransactionBody
}

func UnpackTransaction[PT bodyPtr[T], T any](t TB, dataHeader, dataBody []byte) (*protocol.Transaction, PT) {
	var header protocol.TransactionHeader
	body := PT(new(T))
	if header.UnmarshalBinary(dataHeader) != nil {
		t.Skip()
	}
	if body.UnmarshalBinary(dataBody) != nil {
		t.Skip()
	}
	if header.Principal == nil {
		t.Skip()
	}
	return &protocol.Transaction{Header: header, Body: body}, body
}

func ValidateTransaction(t TB, txn *protocol.Transaction, executor chain.TransactionExecutor, requireSuccess bool, accounts ...protocol.Account) {
	db := database.OpenInMemory(nil)
	ValidateTransactionDb(t, db, txn, executor, requireSuccess, accounts...)
}

func ValidateTransactionDb(t TB, db *database.Database, txn *protocol.Transaction, executor chain.TransactionExecutor, requireSuccess bool, accounts ...protocol.Account) {
	require.Equal(t, txn.Body.Type(), executor.Type())

	for _, account := range accounts {
		u := account.GetUrl().RootIdentity()
		if protocol.IsValidAdiUrl(u, true) == nil {
			continue
		}
		if _, err := protocol.ParseLiteAddress(u); err == nil {
			continue
		}
		t.Skip()
	}

	err := helpers.TryMakeAccount(t, db, accounts...)
	if err != nil {
		t.Skip()
	}

	st := helpers.NewStateManager(t.Name(), db, txn)
	defer st.Discard()

	err = st.LoadUrlAs(st.OriginUrl, &st.Origin)
	pv, ok := executor.(chain.PrincipalValidator)
	if !ok ||
		!pv.AllowMissingPrincipal(txn) ||
		!errors.Is(err, errors.StatusNotFound) {
		require.NoError(t, err)
	}

	_, err = executor.Execute(st, &chain.Delivery{Transaction: txn})
	if requireSuccess {
		require.NoError(t, err)
	}
}
