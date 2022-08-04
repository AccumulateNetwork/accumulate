package cmd

import (
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	service2 "github.com/tendermint/tendermint/libs/service"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"syscall"
	"time"
)

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

var flagRunWalletd = struct {
	ListenAddress string
	CiStopAfter   time.Duration
	LogFile       string
	JsonLogFile   string
}{}

func init() {
	initRunFlags(walletdCmd, false)
}

func initRunFlags(cmd *cobra.Command, forService bool) {
	cmd.ResetFlags()
	cmd.PersistentFlags().StringVar(&flagRunWalletd.ListenAddress, "listen", "http://0.0.0.0:26661", "listen address for daemon")
	cmd.PersistentFlags().StringVar(&flagRunWalletd.LogFile, "log-file", "", "Write logs to a file as plain text")
	cmd.PersistentFlags().StringVar(&flagRunWalletd.JsonLogFile, "json-log-file", "", "Write logs to a file as JSON")

	if !forService {
		cmd.Flags().DurationVar(&flagRunWalletd.CiStopAfter, "ci-stop-after", 0, "FOR CI ONLY - stop the node after some time")
		cmd.Flag("ci-stop-after").Hidden = true
	}
}

func runWalletd(cmd *cobra.Command, _ []string) (string, error) {
	//this will be reworked when wallet database accessed via GetWallet() is moved to the backend.
	prog, err := walletd.NewProgram(cmd, &walletd.ServiceOptions{WorkDir: DatabaseDir,
		LogFilename: flagRunWalletd.LogFile, JsonLogFilename: flagRunWalletd.JsonLogFile}, flagRunWalletd.ListenAddress, GetWallet())
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

	if flagRunWalletd.CiStopAfter != 0 {
		go watchDog(prog, svc, flagRunWalletd.CiStopAfter)
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
