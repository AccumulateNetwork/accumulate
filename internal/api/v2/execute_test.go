package api_test

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	core "github.com/tendermint/tendermint/rpc/core/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	. "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/connections"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestExecuteCheckOnly(t *testing.T) {
	payload, err := new(protocol.SignPending).MarshalBinary()
	require.NoError(t, err)

	baseReq := TxRequest{
		Origin:  url.MustParse("check"),
		Payload: hex.EncodeToString(payload),
		Signer: Signer{
			PublicKey: make([]byte, 32),
			Url:       url.MustParse("check"),
			Timestamp: 1,
		},
		KeyPage: KeyPage{
			Height: 1,
		},
		Signature: make([]byte, 64),
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
		r := j.DoExecute(context.Background(), &req, payload)
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
		r := j.DoExecute(context.Background(), &req, payload)
		err, _ = r.(error)
		require.NoError(t, err)
	})
}
