// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"reflect"
	"strings"

	tmconfig "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
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

// NewTestSlogger creates a *slog.Logger for testing. If console logging is
// disabled, it returns a no-op logger. Otherwise it logs to stderr with
// the console format.
func NewTestSlogger(t TB, levels string, console bool) *slog.Logger {
	if !console {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	c, err := ParseSlogLevel(levels)
	require.NoError(t, err)
	w := ConsoleSlogWriter(os.Stderr, true)
	handler, err := NewSlogHandler(c, w)
	require.NoError(t, err)
	return slog.New(handler).With("test", t.Name())
}

func ConsoleLoggerForTest(t TB, levels string) log.Logger {
	w, err := NewConsoleWriter("plain")
	require.NoError(t, err)
	level, writer, err := ParseLogLevel(levels, w)
	require.NoError(t, err)
	logger, err := NewTendermintLogger(zerolog.New(writer), level, false)
	require.NoError(t, err)
	return logger
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
