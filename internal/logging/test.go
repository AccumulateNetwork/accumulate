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
