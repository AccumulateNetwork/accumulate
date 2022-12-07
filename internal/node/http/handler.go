package http

import (
	"net/http"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/p2p"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	v2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/websocket"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Handler processes API requests.
type Handler struct {
	logger    logging.OptionalLogger
	mux       *http.ServeMux
	router    routing.Router
	node      *p2p.Node
	partition string
}

// Options are the options for a [Handler].
type Options struct {
	Logger    log.Logger
	Node      *p2p.Node
	Router    routing.Router
	Partition string

	// Deprecated
	V2 *v2.JrpcMethods
}

// NewHandler returns a new Handler.
func NewHandler(opts Options) (*Handler, error) {
	h := new(Handler)
	h.logger.Set(opts.Logger)
	h.router = opts.Router
	h.node = opts.Node
	h.partition = opts.Partition

	if h.partition == "" {
		panic("no partition")
	}

	// Message clients
	selfClient := &message.Client{
		Dialer: h.node.SelfDialer(h.partition),
	}

	client := &message.Client{
		Router: routing.MessageRouter{Router: opts.Router},
		Dialer: h.node.Dialer(),
	}

	// JSON-RPC API v3
	v3, err := jsonrpc.NewHandler(
		opts.Logger,
		jsonrpc.NodeService{NodeService: selfClient},
		jsonrpc.NetworkService{NetworkService: client},
		jsonrpc.MetricsService{MetricsService: client},
		jsonrpc.Querier{Querier: client},
		jsonrpc.Submitter{Submitter: client},
		jsonrpc.Validator{Validator: client},
	)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("initialize API v3: %w", err)
	}

	// WebSocket API v3
	ws, err := websocket.NewHandler(
		opts.Logger,
		message.NodeService{NodeService: selfClient},
		message.NetworkService{NetworkService: client},
		message.MetricsService{MetricsService: client},
		message.Querier{Querier: client},
		message.Submitter{Submitter: client},
		message.Validator{Validator: client},
	)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("initialize websocket API: %w", err)
	}

	// Initialize the mux
	if opts.V2 != nil {
		h.mux = opts.V2.NewMux()
	} else {
		h.mux = http.NewServeMux()
	}

	// Handle /v3
	h.mux.Handle("/v3", ws.FallbackTo(v3))

	return h, nil
}

// ServeHTTP implements [http.Handler].
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}
