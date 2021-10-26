package logging

import (
	"fmt"

	"github.com/rs/zerolog"
	"github.com/tendermint/tendermint/libs/log"
)

// TendermintZeroLogger is a Tendermint logger implementation that passes
// messages to a Zerolog logger. It is basically a complete clone of
// Tendermint's default logger.
type TendermintZeroLogger struct {
	Zerolog zerolog.Logger
	Trace   bool
}

// NewTendermintLogger is the default logger implementation for our Tendermint
// nodes. It is based on part of Tendermint's NewTendermintLogger.
func NewTendermintLogger(zl zerolog.Logger, level string, trace bool) (log.Logger, error) {
	logLevel, err := zerolog.ParseLevel(level)
	if err != nil {
		return nil, fmt.Errorf("failed to parse log level: %v", err)
	}

	zl = zl.Level(logLevel).With().Timestamp().Logger()
	return &TendermintZeroLogger{zl, trace}, nil
}

func (l *TendermintZeroLogger) Info(msg string, keyVals ...interface{}) {
	l.Zerolog.Info().Fields(getLogFields(keyVals...)).Msg(msg)
}

func (l *TendermintZeroLogger) Error(msg string, keyVals ...interface{}) {
	e := l.Zerolog.Error()
	if l.Trace {
		e = e.Stack()
	}

	e.Fields(getLogFields(keyVals...)).Msg(msg)
}

func (l *TendermintZeroLogger) Debug(msg string, keyVals ...interface{}) {
	l.Zerolog.Debug().Fields(getLogFields(keyVals...)).Msg(msg)
}

func (l *TendermintZeroLogger) With(keyVals ...interface{}) log.Logger {
	return &TendermintZeroLogger{
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
