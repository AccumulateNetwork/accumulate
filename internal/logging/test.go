package logging

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/tendermint/tendermint/libs/log"
)

type TestLogger struct {
	Test testing.TB
}

var _ io.Writer = (*TestLogger)(nil)

func (l *TestLogger) Write(b []byte) (int, error) {
	s := string(b)
	if strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	l.Test.Log(s)
	return len(b), nil
}

func NewTestZeroLogger(t testing.TB, format string) zerolog.Logger {
	var w io.Writer = &TestLogger{Test: t}
	switch strings.ToLower(format) {
	case log.LogFormatPlain, log.LogFormatText:
		w = newConsoleWriter(w)

	case log.LogFormatJSON:

	default:
		t.Fatalf("Unsupported log format: %s", format)
	}

	return zerolog.New(w)
}

func ExcludeMessages(messages ...string) zerolog.HookFunc {
	return func(e *zerolog.Event, level zerolog.Level, message string) {
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
