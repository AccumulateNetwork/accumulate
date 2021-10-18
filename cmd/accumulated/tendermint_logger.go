package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/kardianos/service"
	"github.com/rs/zerolog"
	"github.com/tendermint/tendermint/libs/log"
)

// This is based off of Tendermint's NewDefaultLogger

func newDefaultLogger(svc service.Service, format, level string, trace bool) (log.Logger, error) {
	var writer *zerolog.ConsoleWriter
	switch strings.ToLower(format) {
	case log.LogFormatPlain, log.LogFormatText:
		writer = &zerolog.ConsoleWriter{
			NoColor:    true,
			TimeFormat: time.RFC3339,
			FormatLevel: func(i interface{}) string {
				if ll, ok := i.(string); ok {
					return strings.ToUpper(ll)
				}
				return "????"
			},
		}

	case log.LogFormatJSON:

	default:
		return nil, fmt.Errorf("unsupported log format: %s", format)
	}

	logLevel, err := zerolog.ParseLevel(level)
	if err != nil {
		return nil, fmt.Errorf("failed to parse log level: %v", err)
	}

	svcLogger, err := svc.Logger(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get service logger: %v", err)
	}

	sl := newServiceLogger(svcLogger, writer)
	zl := zerolog.New(sl).Level(logLevel).With().Timestamp().Logger()
	return defaultLogger{zl, trace}, nil
}

// Everything below is ripped from Tendermint

type defaultLogger struct {
	zerolog.Logger

	trace bool
}

func (l defaultLogger) Info(msg string, keyVals ...interface{}) {
	l.Logger.Info().Fields(getLogFields(keyVals...)).Msg(msg)
}

func (l defaultLogger) Error(msg string, keyVals ...interface{}) {
	e := l.Logger.Error()
	if l.trace {
		e = e.Stack()
	}

	e.Fields(getLogFields(keyVals...)).Msg(msg)
}

func (l defaultLogger) Debug(msg string, keyVals ...interface{}) {
	l.Logger.Debug().Fields(getLogFields(keyVals...)).Msg(msg)
}

func (l defaultLogger) With(keyVals ...interface{}) log.Logger {
	return defaultLogger{
		Logger: l.Logger.With().Fields(getLogFields(keyVals...)).Logger(),
		trace:  l.trace,
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
