package websocket

import (
	"context"
	"encoding/json"
	"io"
	"runtime/debug"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

func NewClient(server string, logger log.Logger) (*Client, error) {
	conn, _, err := websocket.DefaultDialer.Dial(server, nil)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	c := new(Client)
	c.logger.Set(logger)
	c.conn = conn
	c.responseMu = new(sync.Mutex)
	c.requests = map[uint]func(*Response){}
	c.streams = map[uint]*clientStream{}

	go c.readConn()
	return c, nil
}

type Client struct {
	logger logging.OptionalLogger
	conn   *websocket.Conn

	responseMu  *sync.Mutex
	nextRequest uint
	requests    map[uint]func(*Response)
	streams     map[uint]*clientStream
}

func (c *Client) readConn() {
	for {
		resp := new(Response)
		err := c.conn.ReadJSON(resp)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				c.logger.Error("Read loop failed", "error", err)
			}
			return
		}

		// Grab the response channel and stream, if they are defined
		c.responseMu.Lock()
		request := c.requests[resp.RequestId]
		stream := c.streams[resp.Stream]
		delete(c.requests, resp.RequestId)
		c.responseMu.Unlock()

		if request != nil {
			request(resp)
		} else if stream != nil {
			stream.recv <- resp
		}
	}
}

func (c *Client) OpenStream(ctx context.Context, method string, params interface{}) (<-chan *Response, chan<- *Request, error) {
	b, err := json.Marshal(params)
	if err != nil {
		return nil, nil, errors.Format(errors.StatusEncodingError, "marshal params: %w", err)
	}

	req := new(Request)
	req.Stream = 0
	req.Method = method
	req.Params = b

	ctx, cancel := context.WithCancel(ctx)
	send := make(chan *Request)
	recv := make(chan *Response)

	s := new(clientStream)
	s.close = new(sync.Once)
	s.cancel = cancel
	s.recv = recv
	s.cleanup = func() {
		c.responseMu.Lock()
		delete(c.streams, s.id)
		c.responseMu.Unlock()
	}

	go c.writeStream(ctx, s, send)

	respCh := make(chan error)
	gotResp := func(r *Response) {
		if r.Error != nil {
			respCh <- r.Error
		} else {
			s.id = r.Stream
			c.responseMu.Lock()
			c.streams[s.id] = s
			c.responseMu.Unlock()
			close(respCh)
		}
	}

	c.responseMu.Lock()
	c.nextRequest++
	req.Id = c.nextRequest
	c.requests[req.Id] = gotResp
	c.responseMu.Unlock()

	return nil, nil, nil
}

func (c *Client) writeStream(ctx context.Context, s *clientStream, send <-chan *Request) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("Write loop panicked", "error", r, "stack", debug.Stack())
		}
	}()
	defer s.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case req, ok := <-send:
			if !ok {
				return
			}
			err := c.conn.WriteJSON(req)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					c.logger.Error("Write failed", "error", err)
				}
				return
			}
		}
	}
}
