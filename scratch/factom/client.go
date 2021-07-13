// MIT License
//
// Copyright 2018 Canonical Ledgers, LLC
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

package factom

import (
	"context"
	"fmt"

	"github.com/AdamSLevy/jsonrpc2/v14"
)

// Client makes RPC requests to factomd's and factom-walletd's APIs.  Client
// embeds two jsonrpc2.Clients, and thus also two http.Client, one for requests
// to factomd and one for requests to facgo butom-walletd.  Use jsonrpc2.Client's
// BasicAuth settings to set up BasicAuth and http.Client's transport settings
// to configure TLS.
type Client struct {
	Factomd       jsonrpc2.Client
	FactomdServer string
	Walletd       jsonrpc2.Client
	WalletdServer string
}

// Defaults for the factomd and factom-walletd endpoints.
const (
	FactomdDefault = "http://localhost:8088/v2"
	WalletdDefault = "http://localhost:8089/v2"
)

// NewClient returns a pointer to a new Client initialized with the default
// localhost endpoints for factomd and factom-walletd. If factomdDoer or
// walletdDoer is nil, the default http.Client is used. See jsonrpc2.NewClient
// for more details.
func NewClient() *Client {
	c := &Client{FactomdServer: FactomdDefault, WalletdServer: WalletdDefault}
	c.Factomd = jsonrpc2.Client{}
	c.Walletd = jsonrpc2.Client{}
	return c
}

// FactomdRequest makes a request to factomd's v2 API.
func (c *Client) FactomdRequest(
	ctx context.Context, method string, params, result interface{}) error {

	url := c.FactomdServer
	if c.Factomd.DebugRequest {
		fmt.Println("factomd:", url)
	}
	return c.Factomd.Request(ctx, url, method, params, result)
}

// WalletdRequest makes a request to factom-walletd's v2 API.
func (c *Client) WalletdRequest(
	ctx context.Context, method string, params, result interface{}) error {

	url := c.WalletdServer
	if c.Walletd.DebugRequest {
		fmt.Println("factom-walletd:", url)
	}
	return c.Walletd.Request(ctx, url, method, params, result)
}
