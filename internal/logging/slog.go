// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/rs/zerolog"
)

type Slogger slog.Logger

func (s *Slogger) Debug(msg string, keyvals ...interface{}) {
	(*slog.Logger)(s).Debug(msg, keyvals...)
}

func (s *Slogger) Info(msg string, keyvals ...interface{}) {
	(*slog.Logger)(s).Info(msg, keyvals...)
}

func (s *Slogger) Error(msg string, keyvals ...interface{}) {
	(*slog.Logger)(s).Error(msg, keyvals...)
}

func (s *Slogger) With(keyvals ...interface{}) log.Logger {
	l := (*slog.Logger)(s).With(keyvals...)
	return (*Slogger)(l)
}

type SlogConfig struct {
	DefaultLevel slog.Level
	ModuleLevels map[string]slog.Level
}

func NewSlogHandler(c SlogConfig, out io.Writer) (slog.Handler, error) {
	opts := &slog.HandlerOptions{
		Level: c.DefaultLevel,
	}
	for _, l := range c.ModuleLevels {
		if l < opts.Level.Level() {
			opts.Level = l
		}
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
		case "stack":
			if b, ok := a.Value.Any().([]byte); ok {
				return slog.String(a.Key, string(b))
			}
			return a
		default:
			return a
		}
	}

	return &logHandler{
		handler:      slog.NewJSONHandler(out, opts),
		defaultLevel: c.DefaultLevel,
		lowestLevel:  opts.Level.Level(),
		modules:      c.ModuleLevels,
	}, nil
}

// ConsoleSlogWriter returns a zerolog console writer that outputs nicely
// formatted logs.
func ConsoleSlogWriter(w io.Writer, color bool) *zerolog.ConsoleWriter {
	return &zerolog.ConsoleWriter{
		Out:        w,
		TimeFormat: time.RFC3339,
		NoColor:    !color,
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
	}
}

type logHandler struct {
	handler      slog.Handler
	defaultLevel slog.Level
	lowestLevel  slog.Level
	modules      map[string]slog.Level
}

const messageKey = "theMessage"

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
	if level < h.levelFor2(slog.LevelDebug, Attrs(ctx)) {
		return false
	}
	return h.handler.Enabled(ctx, level)
}

func (h *logHandler) Handle(ctx context.Context, record slog.Record) error {
	record.AddAttrs(Attrs(ctx)...)
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
