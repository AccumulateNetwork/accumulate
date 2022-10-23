// Copyright 2022 The Accumulate Authors
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
	"github.com/stretchr/testify/require"
	core "github.com/tendermint/tendermint/rpc/core/types"
	. "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/connections"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() { acctesting.EnableDebugFeatures() }

//go:generate go run github.com/golang/mock/mockgen -source ../../connections/connection_context.go -package api_test -destination ./mock_connections_test.go

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

		connectionManager := connections.NewFakeConnectionManager([]string{""})
		local := NewMockABCIClient(ctrl)
		clients := map[string]connections.FakeClient{}
		clients[""] = connections.FakeClient{local, nil}
		connectionManager.SetClients(clients)
		table := new(protocol.RoutingTable)
		table.Routes = routing.BuildSimpleTable([]string{""})
		router, err := routing.NewStaticRouter(table, connectionManager, nil)
		require.NoError(t, err)
		j, err := NewJrpc(Options{
			Router: router,
			Key:    make([]byte, 64),
		})
		require.NoError(t, err)

		local.EXPECT().CheckTx(gomock.Any(), gomock.Any()).Return(new(core.ResultCheckTx), nil)

		req := baseReq
		req.CheckOnly = true
		r := j.Execute(context.Background(), mustMarshal(t, &req))
		err, _ = r.(error)
		require.NoError(t, err)
	})

	t.Run("False", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		connectionManager := connections.NewFakeConnectionManager([]string{""})
		local := NewMockABCIClient(ctrl)
		clients := map[string]connections.FakeClient{}
		clients[""] = connections.FakeClient{local, nil}
		connectionManager.SetClients(clients)
		table := new(protocol.RoutingTable)
		table.Routes = routing.BuildSimpleTable([]string{""})
		router, err := routing.NewStaticRouter(table, connectionManager, nil)
		require.NoError(t, err)
		j, err := NewJrpc(Options{
			Router: router,
			Key:    make([]byte, 64),
		})
		require.NoError(t, err)

		local.EXPECT().BroadcastTxSync(gomock.Any(), gomock.Any()).Return(new(core.ResultBroadcastTx), nil)

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
