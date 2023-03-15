// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

//go:generate go run github.com/vektra/mockery/v2
//go:generate go run github.com/rinchsan/gosimports/cmd/gosimports -w .

func init() { acctesting.EnableDebugFeatures() }

func TestExecuteCheckOnly(t *testing.T) {
	env := acctesting.NewTransaction().
		WithPrincipal(protocol.FaucetUrl).
		WithBody(&protocol.AcmeFaucet{}).
		Faucet()

	data, err := env.MarshalBinary()
	require.NoError(t, err)

	baseReq := TxRequest{
		Origin:     protocol.AccountUrl("check"),
		Payload:    hex.EncodeToString(data),
		IsEnvelope: true,
	}

	t.Run("True", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		v3 := NewMockV3(t)
		j, err := NewJrpc(Options{NetV3: v3, LocalV3: v3})
		require.NoError(t, err)

		txid := protocol.AccountUrl("foo").WithTxID([32]byte{1})
		v3.EXPECT().Validate(mock.Anything, mock.Anything, mock.Anything).Return([]*api.Submission{{
			Status:  &protocol.TransactionStatus{TxID: txid},
			Success: true,
		}}, nil)

		req := baseReq
		req.CheckOnly = true
		r := j.Execute(context.Background(), mustMarshal(t, &req))
		err, _ = r.(error)
		require.NoError(t, err)
	})

	t.Run("False", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		v3 := NewMockV3(t)
		j, err := NewJrpc(Options{NetV3: v3, LocalV3: v3})
		require.NoError(t, err)

		txid := protocol.AccountUrl("foo").WithTxID([32]byte{1})
		v3.EXPECT().Submit(mock.Anything, mock.Anything, mock.Anything).Return([]*api.Submission{{
			Status:  &protocol.TransactionStatus{TxID: txid},
			Success: true,
		}}, nil)

		req := baseReq
		req.CheckOnly = false
		r := j.Execute(context.Background(), mustMarshal(t, &req))
		err, _ = r.(error)
		require.NoError(t, err)
	})
}

func mustMarshal(t testing.TB, v interface{}) []byte {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
