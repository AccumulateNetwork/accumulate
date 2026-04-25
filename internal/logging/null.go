// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

// NullLogger implements Logger and discards all log messages.
type NullLogger struct{}

func (NullLogger) Debug(msg string, keyVals ...interface{}) {}
func (NullLogger) Info(msg string, keyVals ...interface{})  {}
func (NullLogger) Error(msg string, keyVals ...interface{}) {}

// With returns the same NullLogger.
func (l NullLogger) With(keyVals ...interface{}) Logger { return l }
