// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"gitlab.com/accumulatenetwork/accumulate/exp/loki"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/exp/slog"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (l *Logging) newHandler(inst *Instance, out io.Writer) (slog.Handler, error) {
	setDefaultPtr(&l.Color, true)

	defaultLevel := slog.LevelWarn
	modules := map[string]slog.Level{}
	for _, r := range l.Rules {
		if len(r.Modules) == 0 {
			defaultLevel = r.Level
			continue
		}
		for _, m := range r.Modules {
			modules[strings.ToLower(m)] = r.Level
		}
	}

	switch l.Format {
	case "", "text", "plain":
		out = logging.ConsoleSlogWriter(out, *l.Color)
	case "json":
		// No change
	default:
		return nil, errors.BadRequest.WithFormat("log format %q is not supported", l.Format)
	}

	if l.Loki != nil && l.Loki.Enable {
		out2, err := l.Loki.start(inst)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("Loki: %w", err)
		}
		out = io.MultiWriter(out, out2)
	}

	return logging.NewSlogHandler(logging.SlogConfig{
		DefaultLevel: defaultLevel,
		ModuleLevels: modules,
	}, out)
}

func (l *Logging) start(inst *Instance) error {
	if l == nil {
		l = new(Logging)
	}

	h, err := l.newHandler(inst, os.Stderr)
	if err != nil {
		return err
	}

	inst.logger = slog.New(h)
	slog.SetDefault(inst.logger)

	return nil
}

func (l *LokiLogging) start(inst *Instance) (io.Writer, error) {
	hostname, _ := os.Hostname()
	ch, err := loki.Start(loki.Options{
		Url:      l.Url,
		Username: l.Username,
		Password: l.Password,
		Labels: map[string]string{
			"hostname": hostname,
			"process":  "accumulated",
			"network":  inst.config.Network,
		},
	})
	if err != nil {
		return nil, errors.BadRequest.WithFormat("init Loki: %v", err)
	}

	pipe := make(chan *loki.Entry)
	go func() {
		defer close(ch)
		for {
			select {
			case e := <-pipe:
				ch <- e
			case <-inst.Done():
				return
			}
		}
	}()

	return writeFunc(func(b []byte) (int, error) {
		var evt struct {
			// Time  time.Time     `json:"time"`
			Level zerolog.Level `json:"level"`
		}
		if json.Unmarshal(b, &evt) != nil || evt.Level < zerolog.InfoLevel {
			return len(b), nil
		}
		// if evt.Time.IsZero() {
		// 	evt.Time = time.Now()
		// }

		pipe <- &loki.Entry{
			Timestamp: timestamppb.Now(),
			Line:      string(b),
		}

		return len(b), nil
	}), nil
}

type writeFunc func([]byte) (int, error)

func (l writeFunc) Write(b []byte) (int, error) {
	return l(b)
}
