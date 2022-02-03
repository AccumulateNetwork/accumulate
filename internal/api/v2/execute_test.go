package api_test

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	core "github.com/tendermint/tendermint/rpc/core/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	. "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

func TestExecuteCheckOnly(t *testing.T) {
	baseReq := TxRequest{
		Origin:  &url.URL{Authority: "check"},
		Payload: "",
		Signer: Signer{
			PublicKey: make([]byte, 32),
		},
		Signature: make([]byte, 64),
	}

	t.Run("True", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		local := NewMockClient(ctrl)
		j, err := NewJrpc(Options{
			Router: &routing.Direct{
				Network: &config.Network{
					BvnNames: []string{""},
				},
				Clients: map[string]routing.Client{"": local},
			},
		})
		require.NoError(t, err)

		local.EXPECT().CheckTx(gomock.Any(), gomock.Any()).Return(new(core.ResultCheckTx), nil)

		req := baseReq
		req.CheckOnly = true
		r := j.DoExecute(context.Background(), &req, []byte{})
		err, _ = r.(error)
		require.NoError(t, err)
	})

	t.Run("False", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		local := NewMockClient(ctrl)
		j, err := NewJrpc(Options{
			Router: &routing.Direct{
				Network: &config.Network{
					BvnNames: []string{""},
				},
				Clients: map[string]routing.Client{"": local},
			},
		})
		require.NoError(t, err)

		local.EXPECT().BroadcastTxSync(gomock.Any(), gomock.Any()).Return(new(core.ResultBroadcastTx), nil)

		req := baseReq
		req.CheckOnly = false
		r := j.DoExecute(context.Background(), &req, []byte{})
		err, _ = r.(error)
		require.NoError(t, err)
	})
}
