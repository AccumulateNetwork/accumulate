package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"mime"
	"net/http"
	"os"

	rpc "github.com/tendermint/tendermint/rpc/client"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/go-playground/validator/v10"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
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
	//Config := config.Default(m.Options.Describe.Network.Id, m.Options.Describe.NetworkType, config.NodeTypeValidator, m.Options.Describe.PartitionId)
	// Create node

	qu := new(query.UnknownRequest)
	qd, _ := qu.MarshalBinary()
	res, err := m.Router.Query(c, m.Options.Describe.PartitionId, qd, rpc.DefaultABCIQueryOptions)
	if err != nil {
		fmt.Printf("error is %w", err)
		return internalError(err)
	}
	fmt.Printf("%v", *res)
	//	cl := node.NewLocalClient
	/*	cm := connections.NewConnectionManager(Config, m.Options.Logger, func(server string) (connections.APIClient, error) {

			return New(server)
		})

		fmt.Printf("%v", cm)
		//cm.InitClients(cl., nil)
		fmt.Println(cm.GetLocalClient())

		/*cl, err := local.New()
		if err != nil {
			fmt.Printf("error is %w", err)
			return internalError(err)
		}
		cm.InitClients(cl, nil)*/
	/*	cc, err := cm.SelectConnection(m.Options.Describe.PartitionId, true)
		if err != nil {
			fmt.Printf("error is %w", err)
			return internalError(err)
		}
		tmRes, err := cc.GetABCIClient().ABCIQueryWithOptions(context.Background(), "/status", qd, rpc.DefaultABCIQueryOptions)
		fmt.Println(tmRes.Response)

		/*hash := new([32]byte)
		height := tminfo.Response.LastBlockHeight

		if err != nil {
			fmt.Printf("error is %w", err)
			return internalError(err)
		}
		copy(hash[:], tminfo.Response.LastBlockAppHash)
		if err != nil {
			fmt.Printf("error is %w", err)
			return internalError(err)
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
		status.Ok = true*/
	return "status"
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
