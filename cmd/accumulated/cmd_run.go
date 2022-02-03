package main

import (
	"time"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/badger"
)

var cmdRun = &cobra.Command{
	Use:   "run",
	Short: "Run node",
	Run:   runNode,
	Args:  cobra.NoArgs,
}

var flagRun = struct {
	Node        int
	Truncate    bool
	CiStopAfter time.Duration
}{}

func init() {
	cmdMain.AddCommand(cmdRun)

	initRunFlags(cmdRun, false)
}

func initRunFlags(cmd *cobra.Command, forService bool) {
	cmd.Flags().IntVarP(&flagRun.Node, "node", "n", -1, "Which node are we? [0, n)")
	cmd.PersistentFlags().BoolVar(&flagRun.Truncate, "truncate", false, "Truncate Badger if necessary")

	if !forService {
		cmd.Flags().DurationVar(&flagRun.CiStopAfter, "ci-stop-after", 0, "FOR CI ONLY - stop the node after some time")
		cmd.Flag("ci-stop-after").Hidden = true
	}

	cmd.PersistentPreRun = func(*cobra.Command, []string) {
		badger.TruncateBadger = flagRun.Truncate
	}
}

func runNode(cmd *cobra.Command, _ []string) {
	prog := NewProgram(cmd)
	svc, err := service.New(prog, serviceConfig)
	check(err)

	logger, err := svc.Logger(nil)
	check(err)

	err = svc.Run()
	if err != nil {
		_ = logger.Error(err)
	}
}
