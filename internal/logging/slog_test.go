// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slog"
)

func TestLoggingCtxAttrs(t *testing.T) {
	var records records
	logger := slog.New(&logHandler{
		handler:      handler{&records},
		defaultLevel: slog.LevelDebug,
		lowestLevel:  slog.LevelDebug,
	})

	ctx := With(context.Background(), "foo", "bar")
	logger.InfoContext(ctx, "Hello world")
	require.Len(t, records, 1)

	attrs := map[string]any{}
	records[0].Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})

	require.Equal(t, "Hello world", attrs[messageKey])
	require.Equal(t, "bar", attrs["foo"])
}

func TestPlainLogging(t *testing.T) {
	buf := new(bytes.Buffer)
	handler, err := NewSlogHandler(SlogConfig{
		DefaultLevel: slog.LevelDebug,
	}, ConsoleSlogWriter(buf, false))
	require.NoError(t, err)
	logger := slog.New(stripTime{handler})

	logger.Info("Hello world")
	require.Equal(t, testTime.Format(time.RFC3339)+" INFO Hello world\n", buf.String())
}

func TestJSONLogging(t *testing.T) {
	buf := new(bytes.Buffer)
	handler, err := NewSlogHandler(SlogConfig{
		DefaultLevel: slog.LevelDebug,
	}, buf)
	require.NoError(t, err)
	logger := slog.New(stripTime{handler})

	logger.Info("Hello world")
	require.Equal(t, `{`+
		`"time":"`+testTime.Format(time.RFC3339)+`",`+
		`"level":"INFO",`+
		`"message":"Hello world"`+
		`}`+"\n", buf.String())
}

func (*records) Enabled(context.Context, slog.Level) bool     { return true }
func (*attrHandler) Enabled(context.Context, slog.Level) bool { return true }

type handler struct {
	justHandler
}

func (h handler) Enabled(context.Context, slog.Level) bool { return true }
func (h handler) WithAttrs(attrs []slog.Attr) slog.Handler { return &attrHandler{h, attrs} }
func (h handler) WithGroup(name string) slog.Handler       { return &groupHandler{h, name} }

type justHandler interface {
	Handle(context.Context, slog.Record) error
}

type records []slog.Record

func (r *records) Handle(_ context.Context, record slog.Record) error {
	*r = append(*r, record)
	return nil
}

type attrHandler struct {
	slog.Handler
	attrs []slog.Attr
}

func (h *attrHandler) Handle(ctx context.Context, r slog.Record) error {
	r.AddAttrs(h.attrs...)
	return h.Handler.Handle(ctx, r)
}

type groupHandler struct {
	slog.Handler
	group string
}

func (h *groupHandler) Handle(ctx context.Context, r slog.Record) error {
	var attrs []slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})
	r = slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.AddAttrs(slog.Group(h.group, attrs))
	return h.Handler.Handle(ctx, r)
}

type stripTime struct {
	slog.Handler
}

var testTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)

func (s stripTime) Handle(ctx context.Context, r slog.Record) error {
	r.Time = testTime
	return s.Handler.Handle(ctx, r)
}

func (s stripTime) WithAttrs(attrs []slog.Attr) slog.Handler {
	return stripTime{s.Handler.WithAttrs(attrs)}
}

func (s stripTime) WithGroup(name string) slog.Handler {
	return stripTime{s.Handler.WithGroup(name)}
}
