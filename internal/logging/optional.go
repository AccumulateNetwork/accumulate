// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import (
	"context"
	"log/slog"
)

// OptionalLogger wraps a Logger and provides nil-safe logging methods.
type OptionalLogger struct {
	L Logger
}

// Enabled reports whether the logger emits at the level, so a caller can skip
// building the arguments of a line that would not be written. A logger that
// cannot say is taken to emit everything.
func (l OptionalLogger) Enabled(ctx context.Context, level slog.Level) bool {
	if l.L == nil {
		return false
	}
	if e, ok := l.L.(interface {
		Enabled(context.Context, slog.Level) bool
	}); ok {
		return e.Enabled(ctx, level)
	}
	return true
}

// Set sets the logger, unwrapping any nested OptionalLoggers.
func (l *OptionalLogger) Set(m Logger, keyVals ...interface{}) {
	for {
		opt, ok := m.(OptionalLogger)
		if !ok {
			break
		}
		m = opt.L
	}
	if m != nil {
		l.L = m.With(keyVals...)
	}
}

func (l OptionalLogger) Debug(msg string, keyVals ...interface{}) {
	if l.L == nil {
		return
	}
	l.L.Debug(msg, keyVals...)
}

func (l OptionalLogger) Info(msg string, keyVals ...interface{}) {
	if l.L == nil {
		return
	}
	l.L.Info(msg, keyVals...)
}

func (l OptionalLogger) Error(msg string, keyVals ...interface{}) {
	if l.L == nil {
		return
	}
	l.L.Error(msg, keyVals...)
}

func (l OptionalLogger) With(keyVals ...interface{}) Logger {
	if l.L == nil {
		return l
	}
	return OptionalLogger{l.L.With(keyVals...)}
}
