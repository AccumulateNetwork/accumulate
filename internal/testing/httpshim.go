package testing

import (
	"bytes"
	"io"
	"net/http"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

// DirectJrpcClient returns a client that executes HTTP requests directly
// without going through a TCP connection.
func DirectJrpcClient(jrpc *api.JrpcMethods) *client.Client {
	c, err := client.New("http://direct-jrpc-client")
	if err != nil {
		panic(err)
	}

	mux := jrpc.NewMux()
	c.Client.Client.Timeout = 0
	c.Client.Client.Transport = transportFunc(func(req *http.Request) (*http.Response, error) {
		wr := new(httpResponseWriter)
		wr.resp.Request = req
		wr.resp.Header = http.Header{}
		wr.resp.Body = io.NopCloser(&wr.body)
		mux.ServeHTTP(wr, req)
		wr.resp.ContentLength = int64(wr.body.Len())
		return &wr.resp, nil
	})
	return c
}

type transportFunc func(*http.Request) (*http.Response, error)

func (t transportFunc) RoundTrip(req *http.Request) (*http.Response, error) { return t(req) }

type httpResponseWriter struct {
	resp        http.Response
	body        bytes.Buffer
	wroteHeader bool
}

func (w *httpResponseWriter) Header() http.Header {
	return w.resp.Header
}

func (w *httpResponseWriter) WriteHeader(statusCode int) {
	w.resp.StatusCode = statusCode
	w.wroteHeader = true
}

func (w *httpResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}

	return w.body.Write(b)
}
