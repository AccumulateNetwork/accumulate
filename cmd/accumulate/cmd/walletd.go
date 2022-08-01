package cmd

import (
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	service2 "github.com/tendermint/tendermint/libs/service"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/badger"
	"syscall"
	"time"
)

//var walletdCmd = &cobra.Command{
//	Use:   "walletd",
//	Short: "Run wallet daemon",
//	Run: func(cmd *cobra.Command, _ []string) {
//		out, err := GetVersion()
//		printOutput(cmd, out, err)
//	},
//	Args: cobra.NoArgs,
//}

var walletdCmd = &cobra.Command{
	Use:   "walletd",
	Short: "run wallet service daemon",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runWalletd(cmd, args)
		printOutput(cmd, out, err)
	},
}

var walletdConfig = &service.Config{
	Name:        "accumulate walletd",
	DisplayName: "accumulate-walletd",
	Description: "Service daemon for the accumulate wallet",
	Arguments:   []string{"run"},
}
var flagRun = struct {
	Node             int
	Truncate         bool
	CiStopAfter      time.Duration
	LogFile          string
	JsonLogFile      string
	EnableTimingLogs bool
}{}

func init() {
	//cmd.RootCmd.AddCommand(walletdCmd)

	//	initRunFlags(walletdCmd, false)
}

func initRunFlags(cmd *cobra.Command, forService bool) {
	cmd.ResetFlags()
	cmd.Flags().IntVarP(&flagRun.Node, "node", "n", -1, "Which node are we? [0, n)")
	cmd.PersistentFlags().BoolVar(&flagRun.Truncate, "truncate", false, "Truncate Badger if necessary")
	cmd.PersistentFlags().StringVar(&flagRun.LogFile, "log-file", "", "Write logs to a file as plain text")
	cmd.PersistentFlags().StringVar(&flagRun.JsonLogFile, "json-log-file", "", "Write logs to a file as JSON")
	cmd.PersistentFlags().BoolVar(&flagRun.EnableTimingLogs, "enable-timing-logs", false, "Enable core timing analysis logging")

	if !forService {
		cmd.Flags().DurationVar(&flagRun.CiStopAfter, "ci-stop-after", 0, "FOR CI ONLY - stop the node after some time")
		cmd.Flag("ci-stop-after").Hidden = true
	}

	cmd.PersistentPreRun = func(*cobra.Command, []string) {
		badger.TruncateBadger = flagRun.Truncate
	}
}

func runWalletd(cmd *cobra.Command, _ []string) (string, error) {
	prog, err := walletd.NewProgram(cmd, &walletd.ServiceOptions{WorkDir: DatabaseDir,
		LogFilename: flagRun.LogFile, JsonLogFilename: flagRun.JsonLogFile}, GetWallet())
	if err != nil {
		return "", err
	}

	svc, err := service.New(prog, walletdConfig)
	if err != nil {
		return "", err
	}

	logger, err := svc.Logger(nil)
	if err != nil {
		return "", err
	}

	if flagRun.CiStopAfter != 0 {
		go watchDog(prog, svc, flagRun.CiStopAfter)
	}

	err = svc.Run()
	if err != nil {
		//if it is already stopped, that is ok.
		if !errors.Is(err, service2.ErrAlreadyStopped) {
			_ = logger.Error(err)
			return "", err
		}
	}
	return "shutdown complete", nil
}

func interrupt(pid int) {
	_ = syscall.Kill(pid, syscall.SIGINT)
}

func watchDog(prog *walletd.Program, svc service.Service, duration time.Duration) {
	time.Sleep(duration)

	//this will cause tendermint to stop and exit cleanly.
	_ = prog.Stop(svc)

	//the following will stop the Run()
	interrupt(syscall.Getpid())
}
