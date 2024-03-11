// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testing

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	testhttp "gitlab.com/accumulatenetwork/accumulate/test/util/http"
)

// DirectJrpcClient returns a client that executes HTTP requests directly
// without going through a TCP connection.
func DirectJrpcClient(jrpc *api.JrpcMethods) *client.Client {
	c, err := client.New("http://direct-jrpc-client")
	if err != nil {
		panic(err)
	}

	c.Client.Client = *testhttp.DirectHttpClient(jrpc.NewMux())
	return c
}
