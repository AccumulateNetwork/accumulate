// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	service2 "github.com/tendermint/tendermint/libs/service"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

var cmdRunDual = &cobra.Command{
	Use:   "run-dual <primary> <secondary>",
	Short: "Run a DN and BVN",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runDualNode(cmd, args)
		printOutput(cmd, out, err)
	},
	Args: cobra.ExactArgs(2),
}

var flagRunDual = struct {
	Truncate         bool
	EnableTimingLogs bool

	CiStopAfter time.Duration
}{}

func init() {
	cmdMain.AddCommand(cmdRunDual)

	cmdRunDual.PersistentFlags().BoolVar(&flagRunDual.Truncate, "truncate", false, "Truncate Badger if necessary")
	cmdRunDual.PersistentFlags().BoolVar(&flagRunDual.EnableTimingLogs, "enable-timing-logs", false, "Enable core timing analysis logging")
	cmdRunDual.PersistentFlags().StringVar(&flagRun.PprofListen, "pprof", "", "Address to run net/http/pprof on")

	cmdRunDual.Flags().DurationVar(&flagRunDual.CiStopAfter, "ci-stop-after", 0, "FOR CI ONLY - stop the node after some time")
	cmdRunDual.Flag("ci-stop-after").Hidden = true

	cmdRunDual.PersistentPreRun = func(*cobra.Command, []string) {
		badger.TruncateBadger = flagRunDual.Truncate
	}
}

func runDualNode(cmd *cobra.Command, args []string) (string, error) {
	if flagRun.PprofListen != "" {
		go func() { check(http.ListenAndServe(flagRun.PprofListen, nil)) }()
	}

	prog := NewProgram(cmd, func(cmd *cobra.Command) (string, error) {
		return args[0], nil
	}, func(cmd *cobra.Command) (string, error) {
		return args[1], nil
	})

	flagRun.EnableTimingLogs = flagRunDual.EnableTimingLogs

	// TODO: This only works on POSIX platforms. We need to seriously refactor
	// how service mode works.
	serviceConfig.Option = service.KeyValue{
		"RunWait": func() {
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt)
			defer signal.Stop(sigChan)

			select {
			case <-prog.primary.Done():
			case <-prog.secondary.Done():
			case <-sigChan:
			}
		},
	}

	svc, err := service.New(prog, serviceConfig)
	if err != nil {
		return "", err
	}

	logger, err := svc.Logger(nil)
	if err != nil {
		return "", err
	}

	if flagRunDual.CiStopAfter != 0 {
		go watchDog(prog, svc, flagRunDual.CiStopAfter)
	}

	err = svc.Run()
	if err != nil {
		//if it is already stopped, that is ok.
		if !errors.Is(err, service2.ErrAlreadyStopped) {
			_ = logger.Error(err)
			return "", err
		}
	}
	return "run complete", nil
}
