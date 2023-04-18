// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package badger

import (
	"fmt"
	"strings"

	"github.com/tendermint/tendermint/libs/log"
)

type badgerLogger struct {
	log.Logger
}

func (l badgerLogger) format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return strings.TrimRight(s, "\n")
}

func (l badgerLogger) Errorf(format string, args ...interface{}) {
	l.Error(l.format(format, args...))
}

func (l badgerLogger) Warningf(format string, args ...interface{}) {
	l.Error(l.format(format, args...))
}

func (l badgerLogger) Infof(format string, args ...interface{}) {
	l.Info(l.format(format, args...))
}

func (l badgerLogger) Debugf(format string, args ...interface{}) {
	l.Debug(l.format(format, args...))
}
