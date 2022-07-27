package api

import (
	"context"
	"encoding/json"
	"io"
	stdlog "log"
	"mime"
	"net/http"
	"os"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/go-playground/validator/v10"
	"github.com/tendermint/tendermint/libs/log"
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

	ctx, err := m.ConnectionManager.SelectConnection(m.Options.Describe.PartitionId, true)
	if err != nil {
		return internalError(err)
	}
	/*
		req := new(GeneralQuery)
		apiinfo := new(ChainQueryResponse)
		anchorinfo := new(ChainQueryResponse)
		ledgerurl := m.Options.Describe.Ledger()
		anchorurl := m.Options.Describe.AnchorPool()
		hash := new([32]byte)
		roothash := new([32]byte)
		var lastAnchor uint64
		if err != nil {
			return internalError(err)
		}
		req.Url = ledgerurl
		req.Prove = true
		req.Expand = true
		err = apiclient.RequestAPIv2(context.Background(), "query", req, apiinfo)
		if err != nil {
			return internalError(err)
		}
		req.Url = anchorurl
		err = apiclient.RequestAPIv2(context.Background(), "query", req, anchorinfo)
		if err != nil {
			return internalError(err)
		}
		tminfo, err := tmclient.ABCIInfo(c)
		if err != nil {
			return internalError(err)
		}
		height := tminfo.Response.LastBlockHeight
		copy(hash[:], tminfo.Response.LastBlockAppHash)
		for _, chain := range apiinfo.Chains {
			if chain.Name == "root" {
				copy(roothash[:], []byte(chain.Roots[len(chain.Roots)-1]))
			}
		}

		for _, chain := range anchorinfo.Chains {
			if chain.Name == "anchor(directory)-root" {
				lastAnchor = chain.Height
			}
		}*/
	tmclient := ctx.GetABCIClient()
	apiclient := ctx.GetAPIClient()

	status := new(StatusResponse)
	status.Ok = true
	hash, roothash, height, err := GetLatestRootChainAnchor(tmclient, apiclient, m.Options.Describe.Ledger(), c)
	if err != nil {
		return internalError(err)
	}
	lastAnchor, err := getLatestDirectoryAnchor(ctx, m.Options.Describe.AnchorPool())
	if err != nil {
		return err
	}
	if m.Options.Describe.NetworkType == config.NetworkTypeDirectory {
		status.DnHeight = uint64(height)
		status.DnBptHash = *hash
		status.DnRootHash = *roothash
		return status
	}
	status.BvnHeight = uint64(height)
	status.BvnBptHash = *hash
	status.BvnRootHash = *roothash
	status.LastAnchorHeight = uint64(lastAnchor)
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
