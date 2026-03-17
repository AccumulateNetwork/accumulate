// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

// OptionalLogger wraps a Logger and provides nil-safe logging methods.
type OptionalLogger struct {
	L Logger
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
