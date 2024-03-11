// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import "github.com/cometbft/cometbft/libs/log"

type OptionalLogger struct {
	L log.Logger
}

func (l *OptionalLogger) Set(m log.Logger, keyVals ...interface{}) {
	for {
		l, ok := m.(OptionalLogger)
		if !ok {
			break
		}
		m = l.L
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

func (l OptionalLogger) With(keyVals ...interface{}) log.Logger {
	if l.L == nil {
		return l
	}
	return OptionalLogger{l.L.With(keyVals...)}
}
