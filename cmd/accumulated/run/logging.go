// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/exp/slog"
)

func (l *Logging) start(inst *Instance) error {
	if l == nil {
		l = new(Logging)
	}

	defaultLevel := slog.LevelError
	lowestLevel := defaultLevel
	modules := map[string]slog.Level{}
	for _, r := range l.Rules {
		if r.Module == "" {
			defaultLevel = r.Level
		} else {
			modules[strings.ToLower(r.Module)] = r.Level
		}
		if r.Level < lowestLevel {
			lowestLevel = r.Level
		}
	}

	opts := &slog.HandlerOptions{
		Level: lowestLevel,
	}

	var h slog.Handler
	switch l.Format {
	case "", "text", "plain":
		// Use zerolog's console writer to write pretty logs
		opts.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
			if a.Key != "msg" {
				return a
			}
			if a.Value.Kind() == slog.KindString {
				return slog.Any("message", a.Value)
			}
			return slog.String("message", fmt.Sprint(a.Value.Any()))
		}
		h = slog.NewJSONHandler(&zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
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
		h = slog.NewJSONHandler(os.Stderr, opts)
	default:
		return errors.BadRequest.WithFormat("log format %q is not supported", l.Format)
	}

	inst.logger = slog.New(&logHandler{
		handler:      h,
		defaultLevel: defaultLevel,
		lowestLevel:  lowestLevel,
		modules:      modules,
	})
	slog.SetDefault(inst.logger)

	return nil
}

type consoleWriter struct {
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
	if record.Level < h.levelFor(h.defaultLevel, record.Attrs) {
		return nil
	}
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
		if l, ok := h.modules[strings.ToLower(a.Key)]; ok {
			level = l
		}
		return false
	})
	return level
}
