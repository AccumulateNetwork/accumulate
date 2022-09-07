package client

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Client struct {
	client jsonrpc2.Client
	server string
}

var _ api.NodeService = (*Client)(nil)
var _ api.QueryService = (*Client)(nil)
var _ api.SubmitService = (*Client)(nil)
var _ api.FaucetService = (*Client)(nil)

// New creates new API client with default config
func New(server string) (*Client, error) {
	switch server {
	case "local":
		server = "http://127.0.1.1:26660"
	case "testnet":
		server = "https://testnet.accumulatenetwork.io"
	case "beta":
		server = "https://beta.testnet.accumulatenetwork.io"
	case "mainnet":
		server = "https://mainnet.accumulatenetwork.io"
	}

	u, err := url.Parse(server)
	if err != nil {
		return nil, errors.Format(errors.StatusBadRequest, "invalid server: %v", err)
	}

	c := new(Client)
	c.client.Timeout = 15 * time.Second
	switch u.Path {
	case "":
		c.server = server + "/v3"
	case "/":
		c.server = server + "v3"
	case "/v3":
		c.server = server
	default:
		if !strings.HasSuffix(u.Path, "/v3") {
			return nil, errors.Format(errors.StatusBadRequest, "invalid server: URL path must be empty or end with /v3")
		}
	}

	return c, nil
}

func (c *Client) Status(ctx context.Context) (*api.NodeStatus, error) {
	return sendRequestUnmarshalAs[*api.NodeStatus](c, ctx, "status", struct{}{})
}

func (c *Client) Version(ctx context.Context) (*api.NodeVersion, error) {
	return sendRequestUnmarshalAs[*api.NodeVersion](c, ctx, "version", struct{}{})
}

func (c *Client) Describe(ctx context.Context) (*api.NodeDescription, error) {
	return sendRequestUnmarshalAs[*api.NodeDescription](c, ctx, "describe", struct{}{})
}

func (c *Client) Metrics(ctx context.Context) (*api.NodeMetrics, error) {
	return sendRequestUnmarshalAs[*api.NodeMetrics](c, ctx, "metrics", struct{}{})
}

func (c *Client) QueryRecord(ctx context.Context, account *url.URL, fragment []string, opts api.QueryRecordOptions) (api.Record, error) {
	req := &jsonrpc.QueryRecordRequest{Account: account, Fragment: fragment, QueryRecordOptions: opts}
	return sendRequestUnmarshalWith(c, ctx, "query-record", req, api.UnmarshalRecordJSON)
}

func (c *Client) QueryRange(ctx context.Context, account *url.URL, fragment []string, opts api.QueryRangeOptions) (*api.RecordRange[api.Record], error) {
	req := &jsonrpc.QueryRangeRequest{Account: account, Fragment: fragment, QueryRangeOptions: opts}
	return sendRequestUnmarshalAs[*api.RecordRange[api.Record]](c, ctx, "query-range", req)
}

func (c *Client) Submit(ctx context.Context, envelope *protocol.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	req := &jsonrpc.SubmitRequest{Envelope: envelope, SubmitOptions: opts}
	return sendRequestUnmarshalAs[[]*api.Submission](c, ctx, "submit", req)
}

func (c *Client) Faucet(ctx context.Context, account *url.URL, opts api.SubmitOptions) (*api.Submission, error) {
	req := &jsonrpc.FaucetRequest{Account: account, SubmitOptions: opts}
	return sendRequestUnmarshalAs[*api.Submission](c, ctx, "submit", req)
}

func (c *Client) sendRequest(ctx context.Context, method string, req, resp interface{}) error {
	err := c.client.Request(ctx, c.server, method, req, &resp)
	if err == nil {
		return nil
	}

	var jerr *jsonrpc2.Error
	if !errors.As(err, &jerr) || jerr.Code > jsonrpc.ErrCodeProtocol {
		return err
	}

	var err2 *errors.Error
	if b, e := json.Marshal(jerr.Data); e != nil {
		return err
	} else if json.Unmarshal(b, &err2) != nil {
		return err
	}
	return errors.Format(errors.StatusUnknownError, "request failed: %w", err2)
}

func sendRequestUnmarshalWith[T any](c *Client, ctx context.Context, method string, req interface{}, unmarshal func([]byte) (T, error)) (T, error) {
	var v T
	var resp json.RawMessage
	err := c.sendRequest(ctx, method, req, &resp)
	if err != nil {
		return v, errors.Wrap(errors.StatusUnknownError, err)
	}
	v, err = unmarshal(resp)
	if err != nil {
		return v, errors.Format(errors.StatusEncodingError, "unmarshal response: %w", err)
	}
	return v, nil
}

func sendRequestUnmarshalAs[T any](c *Client, ctx context.Context, method string, req interface{}) (T, error) {
	var v T
	var resp json.RawMessage
	err := c.sendRequest(ctx, method, req, &resp)
	if err != nil {
		return v, errors.Wrap(errors.StatusUnknownError, err)
	}
	err = json.Unmarshal(resp, &v)
	if err != nil {
		return v, errors.Format(errors.StatusEncodingError, "unmarshal response: %w", err)
	}
	return v, nil
}
