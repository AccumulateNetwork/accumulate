// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import (
	"context"
	"log/slog"
)

type _contextKey struct{}

var contextKey _contextKey

func WithAttrs(ctx context.Context, attrs []slog.Attr) context.Context {
	old := Attrs(ctx)
	return context.WithValue(ctx, contextKey, append(old, attrs...))
}

func Attrs(ctx context.Context) []slog.Attr {
	v, _ := ctx.Value(contextKey).([]slog.Attr)
	return v
}

func With(ctx context.Context, args ...any) context.Context {
	var attrs []slog.Attr
	for len(args) > 0 {
		switch v := args[0].(type) {
		case string:
			if len(args) == 1 {
				attrs, args = append(attrs, slog.Any("!BADKEY", v)), nil
			} else {
				attrs, args = append(attrs, slog.Any(v, args[1])), args[2:]
			}
		case slog.Attr:
			attrs, args = append(attrs, v), args[1:]
		default:
			attrs, args = append(attrs, slog.Any("!BADKEY", v)), args[1:]
		}
	}
	return WithAttrs(ctx, attrs)
}
