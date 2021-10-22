package main

import (
	"time"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

var cmdRun = &cobra.Command{
	Use:   "run",
	Short: "Run node",
	Run:   runNode,
}

var flagRun = struct {
	Node        int
	CiStopAfter time.Duration
}{}

func init() {
	cmdMain.AddCommand(cmdRun)

	initRunFlags(cmdRun, false)
}

func initRunFlags(cmd *cobra.Command, forService bool) {
	cmd.Flags().IntVarP(&flagRun.Node, "node", "n", -1, "Which node are we? [0, n)")

	if !forService {
		cmd.Flags().DurationVar(&flagRun.CiStopAfter, "ci-stop-after", 0, "FOR CI ONLY - stop the node after some time")
		cmd.Flag("ci-stop-after").Hidden = true
	}
}

func runNode(cmd *cobra.Command, args []string) {
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
