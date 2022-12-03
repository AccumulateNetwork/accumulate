package websocket

import (
	"context"
	"io"
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

type clientConn struct {
	*websocket.Conn
}

func (c clientConn) Read() (*Message, error) {
	msg := new(Message)
	err := c.ReadJSON(msg)
	return msg, err
}

func (c clientConn) Write(msg *Message) error {
	return c.WriteJSON(msg)
}

func NewClient(server string, logger log.Logger) (*Client, error) {
	conn, _, err := websocket.DefaultDialer.Dial(server, nil)
	if err != nil {
		return nil, errors.Wrap(errors.UnknownError, err)
	}

	c := newClient(clientConn{conn}, logger)
	go func() {
		<-c.Done()
		conn.Close()
	}()
	return c, nil
}

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

	go func() {
		defer c.cancel()
		for {
			msg, err := c.conn.Read()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					c.logger.Error("Failed to read from connection", "error", err)
				}
				return
			}

			c.dialMu.RLock()
			p, ok := c.streams[msg.ID]
			c.dialMu.RUnlock()
			if !ok {
				c.logger.Error("Got message for unknown stream", "message", msg)
				continue
			}

			if msg.Status == StreamStatusClosed {
				p.cancel()
				continue
			}

			err = p.Write(msg.Message)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					c.logger.Info("Failed to write to stream", "id", msg.ID, "error", err)
				}
				return
			}
		}
	}()

	return c
}

func (c *Client) Done() <-chan struct{} {
	return c.context.Done()
}

func (c *Client) Close() error {
	c.cancel()
	return nil
}

func (c *Client) NodeStatus(ctx context.Context, opts api.NodeStatusOptions) (*api.NodeStatus, error) {
	return c.inner.NodeStatus(ctx, opts)
}

func (c *Client) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	return c.inner.NetworkStatus(ctx, opts)
}

func (c *Client) Metrics(ctx context.Context, opts api.MetricsOptions) (*api.Metrics, error) {
	return c.inner.Metrics(ctx, opts)
}

func (c *Client) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	return c.inner.Query(ctx, scope, query)
}

func (c *Client) Submit(ctx context.Context, envelope *protocol.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	return c.inner.Submit(ctx, envelope, opts)
}

func (c *Client) Validate(ctx context.Context, envelope *protocol.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	return c.inner.Validate(ctx, envelope, opts)
}

type clientRouter struct{}

func (clientRouter) Route(message.Message) (multiaddr.Multiaddr, error) {
	// Return some arbitrary address. This will be passed to Dial.
	return multiaddr.NewComponent("ws", "")
}

type clientDialer Client

func (c *clientDialer) Dial(ctx context.Context, _ multiaddr.Multiaddr) (message.Stream, error) {
	ctx, cancel := context.WithCancel(ctx)

	// Create new stream
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
				c.logger.Error("Panicked while handling stream", "error", r)
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
