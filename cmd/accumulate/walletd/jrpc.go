package walletd

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
	"github.com/go-playground/validator/v10"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/db"
	"io"
	stdlog "log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	//_ "github.com/pdrum/swagger-automation/docs" // This line is necessary for go-swagger to find your docs!
	"github.com/tendermint/tendermint/libs/log"
	apiv2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type Options struct {
	Logger        log.Logger
	TxMaxWaitTime time.Duration
	database      db.DB
}

type JrpcMethods struct {
	Options
	methods  jsonrpc2.MethodMap
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

	m.populateMethodTable()

	return m, nil
}

func (m *JrpcMethods) NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/wallet", jsonrpc2.HTTPRequestHandler(m.methods, stdlog.New(os.Stdout, "", 0)))
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

var walletEndpoint = "http://0.0.0.0:33322"

func (m *JrpcMethods) Start() error {
	// Create the JSON-RPC handler
	jrpc, err := NewJrpc(Options{
		TxMaxWaitTime: time.Minute,
	})

	if err != nil {
		return err
	}

	// Run JSON-RPC server
	m.api = &http.Server{Handler: jrpc.NewMux()}
	l, secure, err := listenHttpUrl(walletEndpoint)
	if err != nil {
		return err
	}
	if secure {
		return fmt.Errorf("currently doesn't support secure server")
	}
	//
	//go func() {
	//	err := api.Serve(l)
	//	if err != nil {
	//		jrpc.Logger.Error("JSON-RPC server", "err", err)
	//	}
	//}()

	return m.api.Serve(l)
}

func (m *JrpcMethods) Stop() error {
	if m.api == nil {
		return nil
	}
	return m.api.Shutdown(context.Background())
}

func validatorError(err error) jsonrpc2.Error {
	return jsonrpc2.NewError(apiv2.ErrCodeValidation, "Validation Error", err)
}

// func submissionError(err error) jsonrpc2.Error {
// 	return jsonrpc2.NewError(ErrCodeSubmission, "Submission Entry Error", err)
// }

func accumulateError(err error) jsonrpc2.Error {

	if errors.Is(err, storage.ErrNotFound) {
		return jsonrpc2.NewError(apiv2.ErrCodeNotFound, "Accumulate Error", "Not Found")
	}

	var perr *errors.Error
	if errors.As(err, &perr) {
		return jsonrpc2.NewError(apiv2.ErrCodeProtocolBase-jsonrpc2.ErrorCode(perr.Code), "Accumulate Error", perr.Message)
	}

	var jerr jsonrpc2.Error
	if errors.As(err, &jerr) {
		return jerr
	}

	return jsonrpc2.NewError(apiv2.ErrCodeAccumulate, "Accumulate Error", err)
}
