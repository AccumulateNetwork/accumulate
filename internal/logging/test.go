// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	tmconfig "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/log"
)

type testLogger struct {
	Test TB
}

var _ io.Writer = (*testLogger)(nil)

func (l *testLogger) Write(b []byte) (int, error) {
	s := string(b)
	s = strings.TrimSuffix(s, "\n")
	l.Test.Log(s)
	return len(b), nil
}

type TB interface {
	Name() string
	Log(...interface{})
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	FailNow()
	Helper()
}

func TestLogWriter(t TB) func(string) (io.Writer, error) {
	return func(format string) (io.Writer, error) {
		var w io.Writer = &testLogger{Test: t}
		switch strings.ToLower(format) {
		case tmconfig.LogFormatPlain:
			w = newConsoleWriter(w)

		case tmconfig.LogFormatJSON:

		default:
			t.Fatalf("Unsupported log format: %s", format)
		}

		return w, nil
	}
}

func NewTestLogger(t TB, format, level string, trace bool) log.Logger {
	writer, _ := TestLogWriter(t)(format)
	level, writer, err := ParseLogLevel(level, writer)
	require.NoError(t, err)
	logger, err := NewTendermintLogger(zerolog.New(writer), level, trace)
	require.NoError(t, err)
	return logger.With("test", t.Name())
}

func ExcludeMessages(messages ...string) zerolog.HookFunc {
	return func(e *zerolog.Event, _ zerolog.Level, message string) {
		for _, m := range messages {
			if m == message {
				e.Discard()
				return
			}
		}
	}
}

// BodyHook is a HORRIBLE HACK, really the hackiest of hacks. It filters zerolog
// messages based on the log body. DO NOT USE THIS except for tests.
func BodyHook(hook func(e *zerolog.Event, level zerolog.Level, body map[string]interface{})) zerolog.HookFunc {
	return func(e *zerolog.Event, level zerolog.Level, _ string) {
		// This is the hackiest of hacks, but I want the buffer
		rv := reflect.ValueOf(e)
		buf := rv.Elem().FieldByName("buf").Bytes()
		buf = append(buf, '}')

		var v map[string]interface{}
		err := json.Unmarshal(buf, &v)
		if err != nil {
			return
		}

		hook(e, level, v)
	}
}
