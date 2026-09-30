// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import (
	"fmt"
	"runtime/debug"

	"github.com/rs/zerolog"
)

// ZerologLogger is a logger implementation that passes
// messages to a Zerolog logger.
type ZerologLogger struct {
	Zerolog zerolog.Logger
	Trace   bool
}

// NewZerologLogger is the default logger implementation for our nodes.
func NewZerologLogger(zl zerolog.Logger, level string, trace bool) (Logger, error) {
	logLevel, err := zerolog.ParseLevel(level)
	if err != nil {
		return nil, fmt.Errorf("failed to parse log level: %v", err)
	}

	zl = zl.Level(logLevel).With().Timestamp().Logger()
	return &ZerologLogger{zl, trace}, nil
}

func (l *ZerologLogger) recover() {
	r := recover()
	if r == nil {
		return
	}

	l.Zerolog.Error().Err(fmt.Errorf("%v", r)).Str("stack", string(debug.Stack())).Msg("Panicked while logging")
}

func (l *ZerologLogger) Info(msg string, keyVals ...interface{}) {
	defer l.recover()
	l.Zerolog.Info().Fields(getLogFields(keyVals...)).Msg(msg)
}

func (l *ZerologLogger) Error(msg string, keyVals ...interface{}) {
	defer l.recover()
	e := l.Zerolog.Error()
	if l.Trace {
		e = e.Stack()
	}

	e.Fields(getLogFields(keyVals...)).Msg(msg)
}

func (l *ZerologLogger) Debug(msg string, keyVals ...interface{}) {
	defer l.recover()
	l.Zerolog.Debug().Fields(getLogFields(keyVals...)).Msg(msg)
}

func (l *ZerologLogger) With(keyVals ...interface{}) Logger {
	return &ZerologLogger{
		Zerolog: l.Zerolog.With().Fields(getLogFields(keyVals...)).Logger(),
		Trace:   l.Trace,
	}
}

func getLogFields(keyVals ...interface{}) map[string]interface{} {
	if len(keyVals)%2 != 0 {
		return nil
	}

	fields := make(map[string]interface{}, len(keyVals))
	for i := 0; i < len(keyVals); i += 2 {
		fields[fmt.Sprint(keyVals[i])] = keyVals[i+1]
	}

	return fields
}
