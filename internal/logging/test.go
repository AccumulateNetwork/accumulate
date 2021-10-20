package logging

import (
	"io"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/tendermint/tendermint/libs/log"
)

type TestLogger struct {
	Test testing.TB
}

var _ zerolog.LevelWriter = (*TestLogger)(nil)

func (l *TestLogger) Write(b []byte) (int, error) {
	l.Test.Log(string(b))
	return len(b), nil
}

func (l *TestLogger) WriteLevel(_ zerolog.Level, b []byte) (int, error) {
	l.Test.Log(string(b))
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
