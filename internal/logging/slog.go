// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import (
	"github.com/cometbft/cometbft/libs/log"
	"golang.org/x/exp/slog"
)

type Slogger slog.Logger

func (s *Slogger) Debug(msg string, keyvals ...interface{}) {
	(*slog.Logger)(s).Debug(msg, keyvals...)
}

func (s *Slogger) Info(msg string, keyvals ...interface{}) {
	(*slog.Logger)(s).Info(msg, keyvals...)
}

func (s *Slogger) Error(msg string, keyvals ...interface{}) {
	(*slog.Logger)(s).Error(msg, keyvals...)
}

func (s *Slogger) With(keyvals ...interface{}) log.Logger {
	l := (*slog.Logger)(s).With(keyvals...)
	return (*Slogger)(l)
}
