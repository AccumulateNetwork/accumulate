package api_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	core "github.com/tendermint/tendermint/rpc/coretypes"
	"gitlab.com/accumulatenetwork/accumulate/config"
	. "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/connections"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() { acctesting.EnableDebugFeatures(true) }

func TestExecuteCheckOnly(t *testing.T) {
	env := acctesting.NewTransaction().
		WithPrincipal(protocol.FaucetUrl).
		WithBody(&protocol.AcmeFaucet{}).
		Faucet()

	data, err := env.MarshalBinary()
	require.NoError(t, err)

	baseReq := TxRequest{
		Origin:     url.MustParse("check"),
		Payload:    hex.EncodeToString(data),
		IsEnvelope: true,
	}

	t.Run("True", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		local := connections.NewMockClient(ctrl)
		clients := map[string]connections.Client{}
		clients[""] = local
		connectionManager := connections.NewFakeConnectionManager(clients)
		j, err := NewJrpc(Options{
			Router: &routing.RouterInstance{
				Network: &config.Network{
					Subnets: []config.Subnet{
						{
							ID:   "",
							Type: config.BlockValidator,
						},
					},
				},
				ConnectionManager: connectionManager,
			},
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

		local := connections.NewMockClient(ctrl)
		clients := map[string]connections.Client{}
		clients[""] = local
		connectionManager := connections.NewFakeConnectionManager(clients)
		j, err := NewJrpc(Options{
			Router: &routing.RouterInstance{
				Network: &config.Network{
					Subnets: []config.Subnet{
						{
							ID:   "",
							Type: config.BlockValidator,
						},
					},
				},
				ConnectionManager: connectionManager,
			},
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
