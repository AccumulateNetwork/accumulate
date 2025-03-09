// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package cmdutil

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/rs/zerolog"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
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

type LogLevelFlag []*run.LoggingRule

func (ll LogLevelFlag) Type() string { return "log-levels" }

func (ll LogLevelFlag) String() string {
	var s []string
	for _, r := range ll {
		for _, m := range r.Modules {
			s = append(s, fmt.Sprintf("%s=%s", m, r.Level))
		}
	}
	return strings.Join(s, ",")
}

func (ll *LogLevelFlag) Set(s string) error {
	var level slog.Level
	var err error
	var modules []string
	parts := strings.SplitN(s, "=", 2)
	if len(parts) == 1 {
		err = level.UnmarshalText([]byte(parts[0]))
	} else {
		err = level.UnmarshalText([]byte(parts[1]))
		modules = parts[:1]
	}
	if err != nil {
		return err
	}
	*ll = append(*ll, &run.LoggingRule{Modules: modules, Level: level})
	return nil
}
