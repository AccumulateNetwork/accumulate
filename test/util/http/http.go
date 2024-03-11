// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testhttp

import (
	"bytes"
	"io"
	"net/http"
)

// DirectHttpClient returns an http.Client with no timeout that transports
// requests directly to the given handler.
func DirectHttpClient(handler http.Handler) *http.Client {
	c := new(http.Client)
	c.Timeout = 0
	c.Transport = transportFunc(func(req *http.Request) (*http.Response, error) {
		wr := new(httpResponseWriter)
		wr.resp.Request = req
		wr.resp.Header = http.Header{}
		wr.resp.Body = io.NopCloser(&wr.body)
		handler.ServeHTTP(wr, req)
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
