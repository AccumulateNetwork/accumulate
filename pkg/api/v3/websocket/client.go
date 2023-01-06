// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package websocket

import (
	"context"
	"io"
	"runtime/debug"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/multiformats/go-multiaddr"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Client is a WebSocket message transport client for API v3.
type Client struct {
	logger   logging.OptionalLogger
	inner    message.Client
	conn     message.StreamOf[*Message]
	outgoing chan *Message
	context  context.Context
	cancel   context.CancelFunc

	dialMu  *sync.RWMutex
	nextID  uint64
	streams map[uint64]streamCanceller
}

// clientConn provides convenience methods wrapping [websocket.Conn].
type clientConn struct {
	*websocket.Conn
}

// Read reads a message as JSON from the websocket connection.
func (c clientConn) Read() (*Message, error) {
	msg := new(Message)
	err := c.ReadJSON(msg)
	return msg, err
}

// Write writes a message as JSON to the websocket connection.
func (c clientConn) Write(msg *Message) error {
	return c.WriteJSON(msg)
}

// NewClient returns a new WebSocket API client for the given server.
func NewClient(server string, logger log.Logger) (*Client, error) {
	// Dial the websocket server
	conn, _, err := websocket.DefaultDialer.Dial(server, nil)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Create a new client
	c := newClient(clientConn{conn}, logger)

	// Close the websocket once the client is done
	go func() { <-c.Done(); conn.Close() }()

	return c, nil
}

// newClient is separate from [NewClient] purely to facilitate testing.
func newClient(s message.StreamOf[*Message], logger log.Logger) *Client {
	c := new(Client)
	c.logger.Set(logger)
	c.inner.Dialer = (*clientDialer)(c)
	c.inner.Router = clientRouter{}
	c.inner.DisableFanout = true
	c.conn = s
	c.context, c.cancel = context.WithCancel(context.Background())
	c.outgoing = make(chan *Message)
	c.dialMu = new(sync.RWMutex)
	c.streams = map[uint64]streamCanceller{}

	// Gorilla websocket connections are not necessarily concurrency safe, so
	// use a channel to serialize outgoing messages
	go func() {
		defer c.cancel()
		for {
			select {
			case <-c.context.Done():
				return
			case msg := <-c.outgoing:
				err := c.conn.Write(msg)
				if err != nil {
					if !errors.Is(err, io.EOF) {
						c.logger.Info("Failed to write to connection", "message", msg, "error", err)
					}
					return
				}
			}
		}
	}()

	// Read incoming messages
	go func() {
		defer c.cancel()
		for {
			// Wait for a message
			msg, err := c.conn.Read()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					c.logger.Error("Failed to read from connection", "error", err)
				}
				return
			}

			// Find the sub-stream
			c.dialMu.RLock()
			p, ok := c.streams[msg.ID]
			c.dialMu.RUnlock()
			if !ok {
				c.logger.Error("Got message for unknown stream", "message", msg)
				continue
			}

			// Write the message to the sub-stream
			if msg.Message != nil {
				err = p.Write(msg.Message)
				if err != nil {
					if !errors.Is(err, io.EOF) {
						c.logger.Info("Failed to write to stream", "id", msg.ID, "error", err)
					}
					return
				}
			}

			// Cancel the sub-stream if the server closed it
			if msg.Status == StreamStatusClosed {
				p.cancel()
				continue
			}
		}
	}()

	return c
}

// Done returns a channel that will be closed once the client is closed.
func (c *Client) Done() <-chan struct{} {
	return c.context.Done()
}

// Close closes the client.
func (c *Client) Close() error {
	c.cancel()
	return nil
}

// NodeStatus implements [api.NodeService.NodeStatus].
func (c *Client) NodeStatus(ctx context.Context, opts api.NodeStatusOptions) (*api.NodeStatus, error) {
	return c.inner.NodeStatus(ctx, opts)
}

// NetworkStatus implements [api.NetworkService.NetworkStatus].
func (c *Client) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	return c.inner.NetworkStatus(ctx, opts)
}

// Metrics implements [api.MetricsService.Metrics].
func (c *Client) Metrics(ctx context.Context, opts api.MetricsOptions) (*api.Metrics, error) {
	return c.inner.Metrics(ctx, opts)
}

// Query implements [api.Querier.Query].
func (c *Client) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	return c.inner.Query(ctx, scope, query)
}

// Submit implements [api.Submitter.Submit].
func (c *Client) Submit(ctx context.Context, envelope *protocol.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	return c.inner.Submit(ctx, envelope, opts)
}

// Validate implements [api.Validator.Validate].
func (c *Client) Validate(ctx context.Context, envelope *protocol.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	return c.inner.Validate(ctx, envelope, opts)
}

// Faucet implements [api.Faucet.Faucet].
func (c *Client) Faucet(ctx context.Context, account *url.URL, opts api.FaucetOptions) (*api.Submission, error) {
	return c.inner.Faucet(ctx, account, opts)
}

// Subscribe implements [api.EventService.Subscribe].
func (c *Client) Subscribe(ctx context.Context, opts api.SubscribeOptions) (<-chan api.Event, error) {
	return c.inner.Subscribe(ctx, opts)
}

// clientRouter implements [message.Router].
type clientRouter struct{}

// Route always returns a fixed address since the websocket transport is
// strictly client-server.
func (clientRouter) Route(message.Message) (multiaddr.Multiaddr, error) {
	// Return some arbitrary address. This will be passed to Dial.
	return multiaddr.NewComponent("ws", "")
}

// clientDialer implements [message.Dialer].
type clientDialer Client

// Dial creates a new sub-stream.
func (c *clientDialer) Dial(ctx context.Context, _ multiaddr.Multiaddr) (message.Stream, error) {
	ctx, cancel := context.WithCancel(ctx)

	// Create a pipe and record it
	c.dialMu.Lock()
	c.nextID++
	id := c.nextID
	p, q := message.DuplexPipe(ctx)
	c.streams[id] = streamCanceller{p, cancel}
	c.dialMu.Unlock()

	// Cleanup
	go func() {
		<-ctx.Done()
		c.dialMu.Lock()
		defer c.dialMu.Unlock()
		delete(c.streams, id)
	}()

	// Forward messages
	go func() {
		defer func() {
			if r := recover(); r != nil {
				c.logger.Error("Panicked while handling stream", "error", r, "stack", debug.Stack())
			}
		}()
		defer cancel()
		for {
			msg, err := p.Read()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					c.logger.Info("Failed to read from stream", "id", id, "error", err)
				}
				return
			}

			c.outgoing <- &Message{ID: id, Message: msg}
		}
	}()

	return q, nil
}
