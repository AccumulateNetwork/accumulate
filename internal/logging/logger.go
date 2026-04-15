// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import "log/slog"

// Logger is a CometBFT-free logging interface. This allows packages to use
// logging without depending on CometBFT. Use CometBFTLogger() to convert to
// cometbft/libs/log.Logger when needed for compatibility.
type Logger interface {
	Debug(msg string, keyvals ...interface{})
	Info(msg string, keyvals ...interface{})
	Error(msg string, keyvals ...interface{})
	With(keyvals ...interface{}) Logger
}

// SlogLogger wraps an slog.Logger to implement the Logger interface.
type SlogLogger struct {
	L *slog.Logger
}

// NewSlogLogger creates a new SlogLogger from an slog.Logger.
func NewSlogLogger(l *slog.Logger) *SlogLogger {
	return &SlogLogger{L: l}
}

func (s *SlogLogger) Debug(msg string, keyvals ...interface{}) {
	s.L.Debug(msg, keyvals...)
}

func (s *SlogLogger) Info(msg string, keyvals ...interface{}) {
	s.L.Info(msg, keyvals...)
}

func (s *SlogLogger) Error(msg string, keyvals ...interface{}) {
	s.L.Error(msg, keyvals...)
}

func (s *SlogLogger) With(keyvals ...interface{}) Logger {
	return &SlogLogger{L: s.L.With(keyvals...)}
}

// Slog returns the underlying slog.Logger.
func (s *SlogLogger) Slog() *slog.Logger {
	return s.L
}

// Nop is an alias for NullLogger.
type Nop = NullLogger
