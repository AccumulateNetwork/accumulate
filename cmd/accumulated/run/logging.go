// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/exp/slog"
)

const messageKey = "theMessage"

func (l *Logging) newHandler(out io.Writer) (slog.Handler, error) {
	setDefaultPtr(&l.Color, true)

	defaultLevel := slog.LevelWarn
	lowestLevel := defaultLevel
	modules := map[string]slog.Level{}
	for _, r := range l.Rules {
		if r.Level < lowestLevel {
			lowestLevel = r.Level
		}
		if len(r.Modules) == 0 {
			defaultLevel = r.Level
			continue
		}
		for _, m := range r.Modules {
			modules[strings.ToLower(m)] = r.Level
		}
	}

	opts := &slog.HandlerOptions{
		Level: lowestLevel,
	}

	opts.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
		switch a.Key {
		case "msg":
			if a.Value.Kind() != slog.KindString {
				return slog.String(a.Key, fmt.Sprint(a.Value.Any()))
			}
			if a.Value.String() == "" {
				return slog.Attr{} // Discard
			}
			return a
		case "message":
			return a
		case messageKey:
			return slog.Any("message", a.Value)
		default:
			return a
		}
	}

	var h slog.Handler
	switch l.Format {
	case "", "text", "plain":
		// Use zerolog's console writer to write pretty logs
		h = slog.NewJSONHandler(&zerolog.ConsoleWriter{
			Out:        out,
			TimeFormat: time.RFC3339,
			NoColor:    !*l.Color,
			FormatLevel: func(i interface{}) string {
				if ll, ok := i.(string); ok {
					return strings.ToUpper(ll)
				}
				return "????"
			},
			FormatMessage: func(i interface{}) string {
				s, ok := i.(string)
				if ok {
					return s
				}
				return fmt.Sprint(i)
			},
		}, opts)
	case "json":
		h = slog.NewJSONHandler(out, opts)
	default:
		return nil, errors.BadRequest.WithFormat("log format %q is not supported", l.Format)
	}

	return &logHandler{
		handler:      h,
		defaultLevel: defaultLevel,
		lowestLevel:  lowestLevel,
		modules:      modules,
	}, nil
}

func (l *Logging) start(inst *Instance) error {
	if l == nil {
		l = new(Logging)
	}

	h, err := l.newHandler(os.Stderr)
	if err != nil {
		return err
	}

	inst.logger = slog.New(h)
	slog.SetDefault(inst.logger)

	return nil
}

type logHandler struct {
	handler      slog.Handler
	defaultLevel slog.Level
	lowestLevel  slog.Level
	modules      map[string]slog.Level
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	i := *h
	i.handler = h.handler.WithAttrs(attrs)
	i.lowestLevel = h.levelFor2(i.lowestLevel, attrs)
	return &i
}

func (h *logHandler) WithGroup(name string) slog.Handler {
	i := *h
	i.handler = h.handler.WithGroup(name)
	return &i
}

func (h *logHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if level < h.lowestLevel {
		return false
	}
	if level < h.levelFor2(slog.LevelDebug, logging.Attrs(ctx)) {
		return false
	}
	return h.handler.Enabled(ctx, level)
}

func (h *logHandler) Handle(ctx context.Context, record slog.Record) error {
	record.AddAttrs(logging.Attrs(ctx)...)
	if record.Level < h.levelFor(h.defaultLevel, record.Attrs) {
		return nil
	}

	// Discard specific consensus messages that do not specify a module
	var discard bool
	switch record.Message {
	case "Version info":
		record.Attrs(func(a slog.Attr) bool {
			discard = a.Key == "tendermint_version"
			return !discard
		})

	case "service start":
		record.Attrs(func(a slog.Attr) bool {
			discard = a.Key == "impl" && a.Value.String() == "Node"
			return !discard
		})
	}
	if discard {
		return nil
	}

	record.Add(messageKey, record.Message)
	record.Message = ""
	return h.handler.Handle(ctx, record)
}

func (h *logHandler) levelFor2(level slog.Level, attrs []slog.Attr) slog.Level {
	if len(attrs) == 0 {
		return level
	}
	return h.levelFor(level, func(fn func(slog.Attr) bool) {
		for _, a := range attrs {
			if !fn(a) {
				return
			}
		}
	})
}

func (h *logHandler) levelFor(level slog.Level, fn func(func(slog.Attr) bool)) slog.Level {
	fn(func(a slog.Attr) bool {
		if a.Key != "module" {
			return true
		}
		if l, ok := h.modules[strings.ToLower(a.Value.String())]; ok {
			level = l
		}
		return false
	})
	return level
}
