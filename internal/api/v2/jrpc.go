package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"mime"
	"net/http"
	neturl "net/url"
	"os"
	"strconv"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/go-playground/validator/v10"
	"github.com/tendermint/tendermint/libs/log"
	tmhttp "github.com/tendermint/tendermint/rpc/client/http"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type JrpcMethods struct {
	Options
	querier  *queryDispatch
	methods  jsonrpc2.MethodMap
	validate *validator.Validate
	logger   log.Logger
}

func NewJrpc(opts Options) (*JrpcMethods, error) {
	var err error
	m := new(JrpcMethods)
	m.Options = opts
	m.querier = new(queryDispatch)
	m.querier.Options = opts

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

func (m *JrpcMethods) Querier_TESTONLY() Querier {
	return m.querier
}

func (m *JrpcMethods) logError(msg string, keyVals ...interface{}) {
	if m.logger != nil {
		m.logger.Error(msg, keyVals...)
	}
}

func (m *JrpcMethods) EnableDebug() {
	q := m.querier.direct(m.Options.Describe.PartitionId)
	m.methods["debug-query-direct"] = func(_ context.Context, params json.RawMessage) interface{} {
		req := new(GeneralQuery)
		err := m.parse(params, req)
		if err != nil {
			return err
		}

		return jrpcFormatResponse(q.QueryUrl(req.Url, req.QueryOptions))
	}
}

func (m *JrpcMethods) NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/status", m.jrpc2http(m.Status))
	mux.Handle("/version", m.jrpc2http(m.Version))
	mux.Handle("/describe", m.jrpc2http(m.Describe))
	mux.Handle("/v2", jsonrpc2.HTTPRequestHandler(m.methods, stdlog.New(os.Stdout, "", 0)))
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

func (m *JrpcMethods) Status(c context.Context, params json.RawMessage) interface{} {
	add := m.Options.Describe.Network.Partitions[0].Nodes[0].Address
	nodeurl, err := neturl.Parse(add)
	port, err := strconv.Atoi(nodeurl.Port())
	if err != nil {
		return internalError(err)
	}
	tmurl := fmt.Sprint(nodeurl.Scheme, "://", nodeurl.Hostname(), ":", port+1)
	fmt.Println(tmurl)

	client, err := tmhttp.New(tmurl)

	if err != nil {
		return internalError(err)
	}
	tminfo, err := client.ABCIInfo(c)
	hash := new([32]byte)
	height := tminfo.Response.LastBlockHeight
	copy(hash[:], tminfo.Response.LastBlockAppHash)
	if err != nil {
		return internalError(err)
	}
	res := new(DescriptionResponse)
	res.Network = m.Options.Describe.Network
	res.PartitionId = m.Options.Describe.PartitionId
	res.NetworkType = m.Options.Describe.NetworkType

	// Load network variable values
	err = res.Values.Load(m.Options.Describe.PartitionUrl(), func(account *url.URL, target interface{}) error {
		return m.Database.View(func(batch *database.Batch) error {
			return batch.Account(account).GetStateAs(target)
		})
	})
	if err != nil {
		res.Error = errors.Wrap(errors.StatusUnknownError, err).(*errors.Error)
	}

	status := new(StatusResponse)
	status.Ok = true
	if m.Options.Describe.NetworkType == config.NetworkTypeDirectory {
		status.DnHeight = uint64(height)
		status.DnRootHash = *hash
		return status
	}
	status.BvnHeight = uint64(height)
	status.BvnRootHash = *hash
	status.Ok = true
	status.LastAnchor = res.NetworkAnchor
	return status
}

func (m *JrpcMethods) Version(_ context.Context, params json.RawMessage) interface{} {
	res := new(ChainQueryResponse)
	res.Type = "version"
	res.Data = VersionResponse{
		Version:        accumulate.Version,
		Commit:         accumulate.Commit,
		VersionIsKnown: accumulate.IsVersionKnown(),
	}
	return res
}

func (m *JrpcMethods) Describe(_ context.Context, params json.RawMessage) interface{} {
	res := new(DescriptionResponse)
	res.Network = m.Options.Describe.Network
	res.PartitionId = m.Options.Describe.PartitionId
	res.NetworkType = m.Options.Describe.NetworkType

	// Load network variable values
	err := res.Values.Load(m.Options.Describe.PartitionUrl(), func(account *url.URL, target interface{}) error {
		return m.Database.View(func(batch *database.Batch) error {
			return batch.Account(account).GetStateAs(target)
		})
	})
	if err != nil {
		res.Error = errors.Wrap(errors.StatusUnknownError, err).(*errors.Error)
	}

	return res
}
