package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/stretchr/testify/require"
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
		req.Sponsor = fmt.Sprintf("test%d", i)
		go func() { ch <- j.execute(context.Background(), req, []byte{}) }()
	}

	for i := 0; i < count; i++ {
		r := <-ch
		err, _ := r.(error)
		require.NoError(t, err)
	}
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

func TestDispatchExecute(t *testing.T) {
	c0, h0 := newJrpcCounter()
	c1, h1 := newJrpcCounter()
	c2, h2 := newJrpcCounter()

	mux := http.NewServeMux()
	mux.Handle("/h0", h0)
	mux.Handle("/h1", h1)
	mux.Handle("/h2", h2)

	s := http.Server{Handler: mux}

	l, err := net.Listen("tcp", fmt.Sprintf("localhost:"))
	require.NoError(t, err)
	go func() { _ = s.Serve(l) }()
	t.Cleanup(func() { s.Shutdown(context.Background()) })

	j, err := NewJrpc(JrpcOptions{
		Remote: []string{
			fmt.Sprintf("http://%s/h0", l.Addr().String()),
			fmt.Sprintf("http://%s/h1", l.Addr().String()),
			fmt.Sprintf("http://%s/h2", l.Addr().String()),
		},
		QueueDuration: time.Millisecond,
		QueueDepth:    10,
	})
	require.NoError(t, err)

	const N = 100
	count := map[uint64]int{}
	for i := 0; i < N; i++ {
		u, err := url.Parse(fmt.Sprintf("test%d", i))
		require.NoError(t, err)
		count[u.Routing()%3]++
	}
	testExecute(t, j, N)

	require.Equal(t, count[0], c0.calls["execute"], "Expected %d calls for BVC0, got %d", count[0], c0.calls["execute"])
	require.Equal(t, count[1], c1.calls["execute"], "Expected %d calls for BVC1, got %d", count[1], c1.calls["execute"])
	require.Equal(t, count[2], c2.calls["execute"], "Expected %d calls for BVC2, got %d", count[2], c2.calls["execute"])
}

func TestDispatchExecuteQueueDepth(t *testing.T) {
	c, h := newJrpcCounter()
	s := http.Server{Handler: h}
	l, err := net.Listen("tcp", fmt.Sprintf("localhost:"))
	require.NoError(t, err)
	go func() { _ = s.Serve(l) }()
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })


	j, err := NewJrpc(JrpcOptions{
		Remote:        []string{fmt.Sprintf("http://%s", l.Addr().String())},
		QueueDuration: 1e6 * time.Hour, // Forever
		QueueDepth:    2,
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


	j, err := NewJrpc(JrpcOptions{
		Remote:        []string{fmt.Sprintf("http://%s", l.Addr().String())},
		QueueDuration: time.Millisecond,
		QueueDepth:    1e10, // Infinity
	})
	require.NoError(t, err)

	testExecute(t, j, 4)
	require.Equal(t, 4, c.calls["execute"])
}
