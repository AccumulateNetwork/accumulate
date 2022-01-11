package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"log"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	. "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	mock_api "github.com/AccumulateNetwork/accumulate/internal/mock/api"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/golang/mock/gomock"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	tmlog "github.com/tendermint/tendermint/libs/log"
	core "github.com/tendermint/tendermint/rpc/core/types"
)

type jrpcCounter struct {
	calls map[string]int
}

func (j *jrpcCounter) addMethod(name string, methods jsonrpc2.MethodMap) {
	methods[name] = func(context.Context, json.RawMessage) interface{} {
		j.calls[name]++
		return j.calls[name]
	}
}

func testExecute(t *testing.T, j *JrpcMethods, count int) {
	ch := make(chan interface{})
	for i := 0; i < count; i++ {
		req := new(TxRequest)
		req.Payload = ""
		req.Origin = &url.URL{Authority: fmt.Sprintf("test%d", i)}
		go func() { ch <- j.DoExecute(context.Background(), req, []byte{}) }()
	}

	for i := 0; i < count; i++ {
		r := <-ch
		err, _ := r.(error)
		require.NoError(t, err)
	}
}

func makeLogger(t *testing.T) tmlog.Logger {
	w, _ := logging.TestLogWriter(t)("plain")
	zl := zerolog.New(w)
	tm, err := logging.NewTendermintLogger(zl, "error", false)
	require.NoError(t, err)
	return tm
}

func newJrpcCounter() (*jrpcCounter, http.Handler) {
	j := new(jrpcCounter)
	j.calls = map[string]int{}
	methods := jsonrpc2.MethodMap{}
	j.addMethod("version", methods)
	j.addMethod("metrics", methods)
	j.addMethod("query", methods)
	j.addMethod("query-directory", methods)
	j.addMethod("query-chain", methods)
	j.addMethod("query-tx", methods)
	j.addMethod("query-tx-history", methods)
	j.addMethod("execute", methods)
	return j, jsonrpc2.HTTPRequestHandler(methods, log.New(os.Stdout, "", 0))
}

func TestDispatchExecuteQueueDepth(t *testing.T) {
	c, h := newJrpcCounter()
	s := http.Server{Handler: h}
	l, err := net.Listen("tcp", fmt.Sprintf("localhost:"))
	require.NoError(t, err)
	go func() { _ = s.Serve(l) }()
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })
	connRouter := mock_api.NewMockConnectionRouter(nil, l.Addr())

	j, err := NewJrpc(JrpcOptions{
		QueueDuration:    1e6 * time.Hour, // Forever
		QueueDepth:       2,
		Logger:           makeLogger(t),
		ConnectionRouter: connRouter,
	})
	require.NoError(t, err)

	testExecute(t, j, 4)
	require.Equal(t, 4, c.calls["execute"])
}

func TestDispatchExecuteQueueDuration(t *testing.T) {
	c, h := newJrpcCounter()
	s := http.Server{Handler: h}
	l, err := net.Listen("tcp", fmt.Sprintf("localhost:"))
	require.NoError(t, err)
	go func() { _ = s.Serve(l) }()
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })
	connRouter := mock_api.NewMockConnectionRouter(nil, l.Addr())

	j, err := NewJrpc(JrpcOptions{
		QueueDuration:    time.Millisecond,
		QueueDepth:       1e10, // Infinity
		Logger:           makeLogger(t),
		ConnectionRouter: connRouter,
	})
	require.NoError(t, err)

	testExecute(t, j, 4)
	require.Equal(t, 4, c.calls["execute"])
}

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

		local := mock_api.NewMockABCIBroadcastClient(ctrl)
		connRouter := mock_api.NewMockConnectionRouter(local, nil)
		j, err := NewJrpc(JrpcOptions{
			ConnectionRouter: connRouter,
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
		local := mock_api.NewMockABCIBroadcastClient(ctrl)
		connRouter := mock_api.NewMockConnectionRouter(local, nil)

		j, err := NewJrpc(JrpcOptions{
			ConnectionRouter: connRouter,
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
