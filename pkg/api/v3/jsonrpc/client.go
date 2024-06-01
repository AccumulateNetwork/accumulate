// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package jsonrpc

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type Client struct {
	Client http.Client
	Server string
	Debug  bool
}

var _ api.NodeService = (*Client)(nil)
var _ api.ConsensusService = (*Client)(nil)
var _ api.NetworkService = (*Client)(nil)
var _ api.MetricsService = (*Client)(nil)
var _ api.Querier = (*Client)(nil)
var _ api.Submitter = (*Client)(nil)
var _ api.Validator = (*Client)(nil)
var _ api.Faucet = (*Client)(nil)

// NewClient creates new API client with default config
func NewClient(server string) *Client {
	c := new(Client)
	c.Client.Timeout = 15 * time.Second
	c.Server = server
	return c
}

func (c *Client) NodeInfo(ctx context.Context, opts api.NodeInfoOptions) (*api.NodeInfo, error) {
	return sendRequestUnmarshalAs[*api.NodeInfo](c, ctx, "node-info", &message.NodeInfoRequest{NodeInfoOptions: opts})
}

func (c *Client) FindService(ctx context.Context, opts api.FindServiceOptions) ([]*api.FindServiceResult, error) {
	return sendRequestUnmarshalAs[[]*api.FindServiceResult](c, ctx, "find-service", &message.FindServiceRequest{FindServiceOptions: opts})
}

func (c *Client) ConsensusStatus(ctx context.Context, opts api.ConsensusStatusOptions) (*api.ConsensusStatus, error) {
	return sendRequestUnmarshalAs[*api.ConsensusStatus](c, ctx, "consensus-status", &message.ConsensusStatusRequest{ConsensusStatusOptions: opts})
}

func (c *Client) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	return sendRequestUnmarshalAs[*api.NetworkStatus](c, ctx, "network-status", &message.NetworkStatusRequest{NetworkStatusOptions: opts})
}

func (c *Client) Metrics(ctx context.Context, opts api.MetricsOptions) (*api.Metrics, error) {
	return sendRequestUnmarshalAs[*api.Metrics](c, ctx, "metrics", &message.MetricsRequest{MetricsOptions: opts})
}

func (c *Client) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	req := &message.QueryRequest{Scope: scope, Query: query}
	return sendRequestUnmarshalWith(c, ctx, "query", req, api.UnmarshalRecordJSON)
}

func (c *Client) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	req := &message.SubmitRequest{Envelope: envelope, SubmitOptions: opts}
	return sendRequestUnmarshalAs[[]*api.Submission](c, ctx, "submit", req)
}

func (c *Client) Validate(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	req := &message.ValidateRequest{Envelope: envelope, ValidateOptions: opts}
	return sendRequestUnmarshalAs[[]*api.Submission](c, ctx, "validate", req)
}

func (c *Client) Faucet(ctx context.Context, account *url.URL, opts api.FaucetOptions) (*api.Submission, error) {
	req := &message.FaucetRequest{Account: account, FaucetOptions: opts}
	return sendRequestUnmarshalAs[*api.Submission](c, ctx, "faucet", req)
}

type PrivateClient Client

var _ private.Sequencer = (*PrivateClient)(nil)

func (c *Client) Private() *PrivateClient {
	return (*PrivateClient)(c)
}

func (c *PrivateClient) Sequence(ctx context.Context, src, dst *url.URL, num uint64, opts private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	req := &message.PrivateSequenceRequest{Source: src, Destination: dst, SequenceNumber: num, SequenceOptions: opts}
	return sendRequestUnmarshalAs[*api.MessageRecord[messaging.Message]]((*Client)(c), ctx, "private-sequence", req)
}

func (c *Client) sendRequest(ctx context.Context, method string, req, resp interface{}) error {
	jc := jsonrpc2.Client{Client: c.Client, DebugRequest: c.Debug}
	err := jc.Request(ctx, c.Server, method, req, &resp)
	if err == nil {
		return nil
	}

	var jerr jsonrpc2.Error
	if !errors.As(err, &jerr) || jerr.Code > ErrCodeProtocol {
		return err
	}

	var err2 *errors.Error
	if b, e := json.Marshal(jerr.Data); e != nil {
		return err
	} else if json.Unmarshal(b, &err2) != nil {
		return err
	}
	return errors.UnknownError.WithFormat("request failed: %w", err2)
}

func sendRequestUnmarshalWith[T any](c *Client, ctx context.Context, method string, req interface{}, unmarshal func([]byte) (T, error)) (T, error) {
	var v T
	var resp json.RawMessage
	err := c.sendRequest(ctx, method, req, &resp)
	if err != nil {
		return v, errors.UnknownError.Wrap(err)
	}
	v, err = unmarshal(resp)
	if err != nil {
		return v, errors.EncodingError.WithFormat("unmarshal response: %w", err)
	}
	return v, nil
}

func sendRequestUnmarshalAs[T any](c *Client, ctx context.Context, method string, req interface{}) (T, error) {
	var v T
	var resp json.RawMessage
	err := c.sendRequest(ctx, method, req, &resp)
	if err != nil {
		return v, errors.UnknownError.Wrap(err)
	}
	err = json.Unmarshal(resp, &v)
	if err != nil {
		return v, errors.EncodingError.WithFormat("unmarshal response: %w", err)
	}
	return v, nil
}
