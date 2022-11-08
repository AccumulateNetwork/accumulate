// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type sentryHack struct{}

func (sentryHack) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "gitlab.com" {
		return http.DefaultTransport.RoundTrip(req)
	}

	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("Failed to send event to sentry: %v", r) //nolint:noprint
		}
	}()

	defer req.Body.Close()
	b, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	var v map[string]interface{}
	err = json.Unmarshal(b, &v)
	if err != nil {
		return nil, err
	}

	ex := v["exception"]

	// GitLab expects the event to have a different shape
	v["exception"] = map[string]interface{}{
		"values": ex,
	}

	b, err = json.Marshal(v)
	if err != nil {
		return nil, err
	}

	req.ContentLength = int64(len(b))
	req.Body = io.NopCloser(bytes.NewReader(b))
	resp, err := http.DefaultTransport.RoundTrip(req)
	return resp, err
}
