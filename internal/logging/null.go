package logging

import "github.com/tendermint/tendermint/libs/log"

type NullLogger struct{}

func (NullLogger) Debug(msg string, keyVals ...interface{}) {}
func (NullLogger) Info(msg string, keyVals ...interface{})  {}
func (NullLogger) Error(msg string, keyVals ...interface{}) {}
func (l NullLogger) With(keyVals ...interface{}) log.Logger { return l }
