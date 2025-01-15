// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import "github.com/cometbft/cometbft/libs/log"

type NullLogger struct{}

func (NullLogger) Debug(msg string, keyVals ...interface{}) {}
func (NullLogger) Info(msg string, keyVals ...interface{})  {}
func (NullLogger) Error(msg string, keyVals ...interface{}) {}
func (l NullLogger) With(keyVals ...interface{}) log.Logger { return l }
