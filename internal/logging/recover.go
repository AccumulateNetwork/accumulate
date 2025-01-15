// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import (
	stdlog "log"
	"runtime/debug"

	"github.com/cometbft/cometbft/libs/log"
)

func Recover(logger log.Logger, message string, values ...interface{}) {
	defer func() {
		if recover() != nil {
			println("Panicked while recovering from a panic") //nolint:noprint
		}
	}()

	err := recover()
	if err == nil {
		return
	}
	if logger == nil {
		stdlog.Printf("Panicked without a logger\n%v\n%s\n", err, debug.Stack())
		return
	}

	values = append(values, "error", err, "stack", debug.Stack())
	logger.Error(message, values...)
}
