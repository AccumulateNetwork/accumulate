// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package cmdutil

import (
	"github.com/cometbft/cometbft/libs/log"
	"github.com/rs/zerolog"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

func NewConsoleLogger(levels string) log.Logger {
	lw, err := logging.NewConsoleWriter("plain")
	Check(err)
	ll, lw, err := logging.ParseLogLevel(levels, lw)
	Check(err)
	logger, err := logging.NewTendermintLogger(zerolog.New(lw), ll, false)
	Check(err)
	return logger
}
