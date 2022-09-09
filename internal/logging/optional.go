package logging

import "github.com/tendermint/tendermint/libs/log"

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
