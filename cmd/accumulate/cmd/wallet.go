package cmd

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/kardianos/service"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	service2 "github.com/tendermint/tendermint/libs/service"
	"github.com/tyler-smith/go-bip39"
	"gitlab.com/accumulatenetwork/core/wallet/cmd/accumulate/db"
	"gitlab.com/accumulatenetwork/core/wallet/cmd/accumulate/walletd"
)

var walletCmd = &cobra.Command{
	Use:   "wallet",
	Short: "initialize wallet or start wallet as a service",
	Args:  cobra.ExactArgs(2),
}

var walletInitCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "create a mnemonic seed and wallet",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		err := InitDBCreate(false)
		printOutput(cmd, "", err)
	},
}

var walletInitImportCmd = &cobra.Command{
	Use:   "import",
	Short: "import a mnemonic seed via command prompt or import wallet backup file to create wallet",
}

var walletInitImportMnemonicCmd = &cobra.Command{
	Use:   "mnemonic",
	Short: "import from a mnemonic seed via command prompt",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		err := InitDBImport(cmd, false)
		printOutput(cmd, "", err)
	},
}

var walletInitImportKeystoreCmd = &cobra.Command{
	Use:   "keystore",
	Short: "restore wallet and seed from exported backup keystore file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		err := ImportAccounts(args[0])
		printOutput(cmd, "", err)
	},
}

var walletInitScriptCmd = &cobra.Command{
	Use:   "script",
	Short: "create a wallet from a script (used for testing only)",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		err := InitDBScript()
		printOutput(cmd, "", err)
	},
}

var walletInitCmd = &cobra.Command{
	Use:   "init",
	Short: "create, import, or restore a wallet",
}

var walletServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "run wallet service daemon",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runWalletd(cmd, args)
		printOutput(cmd, out, err)
	},
}

var walletExportCmd = &cobra.Command{
	Use:   "export [location to export]",
	Short: "export wallet details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		err := ExportAccounts(args[0])
		printOutput(cmd, "File downloaded successfully", err)
	},
}

func init() {
	initRunFlags(walletCmd, false)
	walletInitCmd.AddCommand(walletInitCreateCmd, walletInitImportCmd, walletInitImportCmd, walletInitScriptCmd)
	walletInitImportCmd.AddCommand(walletInitImportMnemonicCmd, walletInitImportKeystoreCmd)
	walletCmd.AddCommand(walletInitCmd)
	walletCmd.AddCommand(walletServeCmd)
	walletCmd.AddCommand(walletExportCmd)
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

func watchDog(prog *walletd.Program, svc service.Service, duration time.Duration) {
	time.Sleep(duration)

	//this will cause tendermint to stop and exit cleanly.
	_ = prog.Stop(svc)

	//the following will stop the Run()
	walletd.Interrupt(syscall.Getpid())
}

func InitDBImport(cmd *cobra.Command, memDb bool) error {
	mnemonicString, err := getPasswdPrompt(cmd, "Enter mnemonic : ", true)
	if err != nil {
		return db.ErrInvalidPassword
	}
	mnemonic := strings.Split(mnemonicString, " ")
	_, err = walletd.ImportMnemonic(mnemonic)
	if err != nil {
		return err
	}
	return nil
}

func InitDBCreate(memDb bool) error {
	root, _ := walletd.GetWallet().Get(walletd.BucketMnemonic, []byte("seed"))
	if len(root) != 0 {
		return fmt.Errorf("mnemonic seed phrase already exists within wallet")
	}

	entropy, err := bip39.NewEntropy(int(walletd.Entropy))
	if err != nil {
		return err
	}
	mnemonicString, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return err
	}
	_, err = promptMnemonic(mnemonicString)
	if err != nil {
		return err
	}
	mnemonicConfirm, err := promptMnemonicConfirm()
	if err != nil {
		return err
	}
	if mnemonicString != mnemonicConfirm {
		return fmt.Errorf("mnemonic doesn't match.")
	}
	mnemonic := strings.Split(mnemonicString, " ")
	_, err = walletd.ImportMnemonic(mnemonic)
	if err != nil {
		return err
	}
	return nil
}

// InitDBScript makes it easy to initialize the wallet for validation tests
func InitDBScript() error {
	root, _ := walletd.GetWallet().Get(walletd.BucketMnemonic, []byte("seed"))
	if len(root) != 0 {
		return nil // mnemonic seed phrase already exists within wallet
	}

	entropy, err := bip39.NewEntropy(int(walletd.Entropy))
	if err != nil {
		return err
	}
	mnemonicString, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return err
	}
	mnemonic := strings.Split(mnemonicString, " ")
	_, err = walletd.ImportMnemonic(mnemonic)
	if err != nil {
		return err
	}
	return nil
}

type promptContent struct {
	errorMsg string
	label    string
}

func promptMnemonic(mnemonic string) (string, error) {
	pc := promptContent{
		"",
		"Please write down your mnemonic phrase and press <enter> when done. '" +
			mnemonic + "'",
	}
	items := []string{"\n"}
	index := -1
	var result string
	var err error

	for index < 0 {
		prompt := promptui.SelectWithAdd{
			Label: pc.label,
			Items: items,
		}
		index, result, err = prompt.Run()
		if index == -1 {
			items = append(items, result)
		}
	}

	if err != nil {
		return "", err
	}

	return result, nil
}

func promptMnemonicConfirm() (string, error) {
	validate := func(input string) error {
		if len(input) <= 0 {
			return errors.New("Invalid mnemonic enttered")
		}
		return nil
	}

	prompt := promptui.Prompt{
		Label:    "Please re-enter the mnemonic phrase.",
		Validate: validate,
	}
	result, err := prompt.Run()
	if err != nil {
		return "", err
	}
	return result, nil
}
