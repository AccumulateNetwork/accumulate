package cmd

import (
	"errors"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	service2 "github.com/tendermint/tendermint/libs/service"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd"
	"syscall"
	"time"
)

var walletCmd = &cobra.Command{
	Use:   "wallet",
	Short: "initialize wallet or start wallet as a service",
	Args:  cobra.ExactArgs(2),
}

var WalletInitCmd = &cobra.Command{
	Use:   "wallet init [create/import]",
	Short: "Import secret factoid key from terminal input",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[1] {
		case "create":
			err := walletd.InitDBCreate(false)
			printOutput(cmd, "", err)
		case "import":
			err := walletd.InitDBImport(cmd, false)
			printOutput(cmd, "", err)
		default:
		}
	},
}

var walletServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "run wallet service daemon",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runWalletd(cmd, args)
		printOutput(cmd, out, err)
	},
}

func init() {
	initRunFlags(walletCmd, false)
	walletCmd.AddCommand(WalletInitCmd)
	walletCmd.AddCommand(walletServeCmd)
}

var walletdConfig = &service.Config{
	Name:        "accumulate wallet serve",
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

func initRunFlags(cmd *cobra.Command, forService bool) {
	cmd.ResetFlags()
	cmd.PersistentFlags().StringVar(&flagRunWalletd.ListenAddress, "listen", "http://localhost:26661", "listen address for daemon")
	cmd.PersistentFlags().StringVar(&flagRunWalletd.LogFile, "log-file", "", "Write logs to a file as plain text")
	cmd.PersistentFlags().StringVar(&flagRunWalletd.JsonLogFile, "json-log-file", "", "Write logs to a file as JSON")

	if !forService {
		cmd.Flags().DurationVar(&flagRunWalletd.CiStopAfter, "ci-stop-after", 0, "FOR CI ONLY - stop the node after some time")
		cmd.Flag("ci-stop-after").Hidden = true
	}
}

func runWalletd(cmd *cobra.Command, _ []string) (string, error) {
	//this will be reworked when wallet database accessed via GetWallet() is moved to the backend.
	prog, err := walletd.NewProgram(cmd, &walletd.ServiceOptions{WorkDir: walletd.DatabaseDir,
		LogFilename: flagRunWalletd.LogFile, JsonLogFilename: flagRunWalletd.JsonLogFile}, flagRunWalletd.ListenAddress)
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