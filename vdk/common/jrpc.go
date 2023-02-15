package vdk

// Package classification awesome.
//
// Documentation of our awesome API.
//
//     Schemes: http
//     BasePath: /
//     Version: 1.0.0
//     Host: some-url.com
//
//     Consumes:
//     - application/json
//
//     Produces:
//     - application/json
//
//     Security:
//     - basic
//
//    SecurityDefinitions:
//    basic:
//      type: basic
//
// swagger:meta

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/go-playground/validator/v10"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type JsonMethods interface {
	populateMethodTable() jsonrpc2.MethodMap
	parse(params json.RawMessage, target interface{}, validateFields ...string) error
	jrpcFormatResponse(res interface{}, err error) interface{}
	muxHandle() string
}

type Options struct {
	Logger        log.Logger
	listenAddress string
	methods       JsonMethods
}

type JrpcMethods struct {
	Options
	validate *validator.Validate
	logger   log.Logger
	api      *http.Server
}

func NewJrpc(opts Options) (*JrpcMethods, error) {
	var err error
	m := new(JrpcMethods)
	m.Options = opts

	if opts.Logger != nil {
		m.logger = opts.Logger.With("module", "jrpc")
	}

	m.validate, err = protocol.NewValidator()
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (m *JrpcMethods) NewMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle(m.Options.methods.muxHandle(), jsonrpc2.HTTPRequestHandler(m.Options.methods.populateMethodTable(), stdlog.New(os.Stdout, "", 0)))
	return mux
}

func (m *JrpcMethods) jrpc2http(jrpc jsonrpc2.MethodFunc) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			res.WriteHeader(http.StatusBadRequest)
			return
		}

		var params json.RawMessage
		mediatype, _, _ := mime.ParseMediaType(req.Header.Get("Content-Type"))
		if mediatype == "application/json" || mediatype == "text/json" {
			params = body
		}

		r := jrpc(req.Context(), params)
		res.Header().Add("Content-Type", "application/json")
		data, err := json.Marshal(r)
		if err != nil {
			m.logError("Failed to marshal status", "error", err)
			res.WriteHeader(http.StatusInternalServerError)
			return
		}

		_, _ = res.Write(data)
	}
}

func (m *JrpcMethods) logError(msg string, keyVals ...interface{}) {
	if m.logger != nil {
		m.logger.Error(msg, keyVals...)
	}
}

// listenHttpUrl takes a string such as `http://localhost:123` and creates a TCP
// listener.
func listenHttpUrl(s string) (net.Listener, bool, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, false, fmt.Errorf("invalid address: %v", err)
	}

	if u.Path != "" && u.Path != "/" {
		return nil, false, fmt.Errorf("invalid address: path is not empty")
	}

	var secure bool
	switch u.Scheme {
	case "tcp", "http":
		secure = false
	case "https":
		secure = true
	default:
		return nil, false, fmt.Errorf("invalid address: unsupported scheme %q", u.Scheme)
	}

	l, err := net.Listen("tcp", u.Host)
	if err != nil {
		return nil, false, err
	}

	return l, secure, nil
}

func (m *JrpcMethods) Start() error {
	// Run JSON-RPC server
	m.api = &http.Server{Handler: m.NewMux()}
	l, secure, err := listenHttpUrl(m.listenAddress)
	if err != nil {
		return err
	}
	if secure {
		return fmt.Errorf("currently doesn't support secure server")
	}
	return m.api.Serve(l)
}

func (m *JrpcMethods) Stop() error {
	if m.api == nil {
		return nil
	}
	return m.api.Shutdown(context.Background())
}
