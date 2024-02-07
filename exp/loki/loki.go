package loki

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang/snappy"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/exp/slog"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const MaxWait = 10 * time.Second //5 * time.Minute
const BatchLimit = 5000

func Start(cfg *config.Logging, labels map[string]string) (chan<- *Entry, error) {
	u, err := url.Parse(cfg.LokiUrl)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("Loki URL: %v", err)
	}
	u.User = url.UserPassword(cfg.LokiUsername, cfg.LokiPassword)

	strLabels := new(strings.Builder)
	strLabels.WriteByte('{')
	var comma bool
	for k, v := range labels {
		if comma {
			strLabels.WriteByte(',')
		}
		comma = true
		strLabels.WriteString(k)
		strLabels.WriteByte('=')
		strLabels.WriteString(strconv.Quote(v))
	}
	strLabels.WriteByte('}')

	ch := make(chan *Entry, 1000)
	c := &client{
		url:     u.String(),
		batch:   make([]*Entry, 0, BatchLimit),
		maxWait: time.NewTimer(MaxWait),
		labels:  strLabels.String(),
		ch:      ch,
	}

	// Sanity check
	c.batch = append(c.batch, &Entry{
		Timestamp: timestamppb.Now(),
		Line:      "Starting Loki",
	})
	err = c.send()
	if err != nil {
		return nil, errors.BadRequest.WithFormat("start Loki: %w", err)
	}

	go c.run()
	return ch, nil
}

type client struct {
	url     string
	batch   []*Entry
	maxWait *time.Timer
	labels  string
	ch      <-chan *Entry
}

func (c *client) run() {
	defer c.maxWait.Stop()

	// Send remainder
	defer func() {
		err := c.send()
		if err != nil {
			slog.Error("Error sending logs to Loki", "error", err)
		}
	}()

	for {
		select {
		case <-c.maxWait.C:

		case entry, ok := <-c.ch:
			if !ok {
				return
			}
			c.batch = append(c.batch, entry)
			if len(c.batch) < BatchLimit {
				continue
			}
		}

		err := c.send()
		if err != nil {
			slog.Error("Error sending logs to Loki", "error", err)
		}
	}
}

func (c *client) send() error {
	c.maxWait.Reset(MaxWait)

	if len(c.batch) == 0 {
		return nil
	}

	batch := c.batch
	c.batch = c.batch[:0]

	b, err := proto.Marshal(&PushRequest{
		Streams: []*Stream{{
			Labels:  c.labels,
			Entries: batch,
		}},
	})
	if err != nil {
		return fmt.Errorf("marshal push request: %w", err)
	}
	b = snappy.Encode(nil, b)

	//sendJsonReq("POST", c.config.PushURL, "application/x-protobuf", buf)
	req, err := http.NewRequest("POST", c.url, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-protobuf")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 204 {
		return nil
	}

	b, err = io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	return fmt.Errorf("unexpected HTTP status code: %d, body: %s\n", resp.StatusCode, b)
}
