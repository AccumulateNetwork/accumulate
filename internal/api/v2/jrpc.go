package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/AccumulateNetwork/accumulated"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/go-playground/validator/v10"
	"github.com/ybbus/jsonrpc/v2"
)

type JrpcOptions struct {
	Prometheus        string
	Query             Querier
	Local             ABCIBroadcastClient
	Remote            []string
	EnableSubscribeTx bool
	QueueDuration     time.Duration
	QueueDepth        int
}

type JrpcMethods struct {
	opts       JrpcOptions
	validate   *validator.Validate
	remote     []jsonrpc.RPCClient
	localIndex int
	exch       chan executeRequest
	queue      executeQueue
}

func NewJrpc(opts JrpcOptions) (*JrpcMethods, error) {
	var err error
	m := new(JrpcMethods)
	m.opts = opts
	m.remote = make([]jsonrpc.RPCClient, len(opts.Remote))
	m.localIndex = -1
	m.exch = make(chan executeRequest)
	m.queue.leader = make(chan struct{}, 1)
	m.queue.leader <- struct{}{}
	m.queue.enqueue = make(chan *executeRequest)

	m.validate, err = protocol.NewValidator()
	if err != nil {
		return nil, err
	}

	for i, addr := range opts.Remote {
		switch {
		case addr != "local":
			m.remote[i] = jsonrpc.NewClient(addr)
		case m.localIndex < 0:
			m.localIndex = i
		default:
			return nil, errors.New("multiple remote addresses are 'local'")
		}
	}

	if m.localIndex >= 0 && m.opts.Local == nil {
		return nil, errors.New("local node specified but no client provided")
	}

	return m, nil
}

func (m *JrpcMethods) Version(_ context.Context, params json.RawMessage) interface{} {
	res := new(QueryResponse)
	res.Type = "version"
	res.Data = map[string]interface{}{
		"version":        accumulated.Version,
		"commit":         accumulated.Commit,
		"versionIsKnown": accumulated.IsVersionKnown(),
	}
	return res
}
