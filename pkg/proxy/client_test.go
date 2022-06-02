package proxy

import (
	"encoding/json"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"io"
	stdlog "log"
	"mime"
	"net/http"
	"os"
	"testing"
)

func jrpc2http(t *testing.T, jrpc jsonrpc2.MethodFunc) http.HandlerFunc {
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
			t.Log("Failed to marshal status", "error", err)
			res.WriteHeader(http.StatusInternalServerError)
			return
		}

		_, _ = res.Write(data)
	}
}

func newMux(t *testing.T, methods *jsonrpc2.MethodMap) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/rpc", jsonrpc2.HTTPRequestHandler(*methods, stdlog.New(os.Stdout, "", 0)))
	return mux
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

	methods := make(jsonrpc2.MethodMap, 35)

	methods["describe"] = m.Describe
	methods["execute"] = m.Execute
	methods["add-credits"] = m.ExecuteAddCredits
	methods["add-validator"] = m.ExecuteAddValidator
	return m, nil
}

func TestAccuProxyClient(t *testing.T) {

}
