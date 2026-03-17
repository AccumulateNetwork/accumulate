// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import "log/slog"

type OptionalLogger struct {
	L *slog.Logger
}

func (l *OptionalLogger) Set(m *slog.Logger, keyVals ...interface{}) {
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

func (l OptionalLogger) With(keyVals ...interface{}) *slog.Logger {
	if l.L == nil {
		return nil
	}
	return l.L.With(keyVals...)
}
