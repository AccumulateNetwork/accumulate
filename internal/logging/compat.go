// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import "github.com/cometbft/cometbft/libs/log"

// CometBFTLogger returns a cometbft/libs/log.Logger from our Logger interface.
// This is used for compatibility with code that still requires the CometBFT
// logger type.
func CometBFTLogger(l Logger) log.Logger {
	if l == nil {
		return cometNop{}
	}
	// Check if it's a fromCometAdapter, return the underlying logger
	if a, ok := l.(*fromCometAdapter); ok {
		return a.l
	}
	return &cometAdapter{l}
}

// FromCometBFT wraps a cometbft/libs/log.Logger as our Logger interface.
func FromCometBFT(l log.Logger) Logger {
	if l == nil {
		return NullLogger{}
	}
	// Check if it's a cometAdapter, return the underlying logger
	if a, ok := l.(*cometAdapter); ok {
		return a.l
	}
	return &fromCometAdapter{l}
}

// cometAdapter adapts our Logger to cometbft/libs/log.Logger.
type cometAdapter struct {
	l Logger
}

func (c *cometAdapter) Debug(msg string, keyvals ...interface{}) {
	c.l.Debug(msg, keyvals...)
}

func (c *cometAdapter) Info(msg string, keyvals ...interface{}) {
	c.l.Info(msg, keyvals...)
}

func (c *cometAdapter) Error(msg string, keyvals ...interface{}) {
	c.l.Error(msg, keyvals...)
}

func (c *cometAdapter) With(keyvals ...interface{}) log.Logger {
	return &cometAdapter{c.l.With(keyvals...)}
}

// cometNop is a nil-safe CometBFT logger.
type cometNop struct{}

func (cometNop) Debug(msg string, keyvals ...interface{}) {}
func (cometNop) Info(msg string, keyvals ...interface{})  {}
func (cometNop) Error(msg string, keyvals ...interface{}) {}
func (c cometNop) With(keyvals ...interface{}) log.Logger { return c }

// fromCometAdapter adapts cometbft/libs/log.Logger to our Logger interface.
type fromCometAdapter struct {
	l log.Logger
}

func (f *fromCometAdapter) Debug(msg string, keyvals ...interface{}) {
	f.l.Debug(msg, keyvals...)
}

func (f *fromCometAdapter) Info(msg string, keyvals ...interface{}) {
	f.l.Info(msg, keyvals...)
}

func (f *fromCometAdapter) Error(msg string, keyvals ...interface{}) {
	f.l.Error(msg, keyvals...)
}

func (f *fromCometAdapter) With(keyvals ...interface{}) Logger {
	return &fromCometAdapter{f.l.With(keyvals...)}
}
