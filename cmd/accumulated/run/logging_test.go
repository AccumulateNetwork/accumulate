// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"golang.org/x/exp/slog"
)

func TestLoggingCtxAttrs(t *testing.T) {
	var records records
	logger := slog.New(&logHandler{
		handler:      handler{&records},
		defaultLevel: slog.LevelDebug,
		lowestLevel:  slog.LevelDebug,
	})

	ctx := logging.With(context.Background(), "foo", "bar")
	logger.InfoContext(ctx, "Hello world")

	require.Len(t, records, 1)
	r := records[0]
	require.Equal(t, "Hello world", r.Message)

	var foo *slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "foo" {
			foo = &a
		}
		return foo == nil
	})
	require.NotNil(t, foo)
	require.Equal(t, "bar", foo.Value.String())
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
