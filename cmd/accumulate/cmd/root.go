package cmd

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/howeyc/gopass"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/tyler-smith/go-bip39"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/db"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
)

func GetWallet() db.DB {
	if wallet == nil {
		wallet = initDB(DatabaseDir, false)
		//upon first use, make sure database format is up-to-date.
		if !NoWalletVersionCheck {
			out, err := RestoreAccounts()
			if err != nil && err != db.ErrNoBucket {
				log.Fatalf("failed to update wallet database: %v", err)
			}
			if out != "" {
				log.Println("performing account database update")
				log.Println(out)
			}
		}
	}
	return wallet
}

var (
	Client               *client.Client
	ClientTimeout        time.Duration
	ClientDebug          bool
	UseUnencryptedWallet bool
	wallet               db.DB
	WantJsonOutput       = false
	TxPretend            = false
	Prove                = false
	Memo                 string
	Metadata             string
	SigType              string
	Authorities          []string
	Delegators           []string
	Password             string
	DatabaseDir          string
	NoWalletVersionCheck bool
	AdditionalSigners    []string
	SignerVersion        uint
)

var currentUser = func() *user.User {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	return usr
}()

var DidError error

func InitRootCmd(database db.DB) *cobra.Command {
	wallet = database
	cmd := &cobra.Command{
		Use:   "accumulate",
		Short: "CLI for Accumulate Network",
	}

	cmd.SetOut(os.Stdout)

	serverAddr := os.Getenv("ACC_API")
	if serverAddr == "" {
		serverAddr = "https://beta.testnet.accumulatenetwork.io/v2"
	}

	flags := cmd.PersistentFlags()
	flags.StringVar(&DatabaseDir, "database", filepath.Join(currentUser.HomeDir, ".accumulate"), "Directory the database is stored in")
	flags.StringVarP(&serverAddr, "server", "s", serverAddr, "Accumulated server")
	flags.DurationVarP(&ClientTimeout, "timeout", "t", 5*time.Second, "Timeout for all API requests (i.e. 10s, 1m)")
	flags.BoolVar(&UseUnencryptedWallet, "use-unencrypted-wallet", false, "Use unencrypted wallet (strongly discouraged) stored at ~/.accumulate/wallet.db")
	flags.BoolVar(&NoWalletVersionCheck, "no-wallet-version-check", false, "Bypass the check to prevent updating the wallet to the format supported by the cli")
	flags.BoolVarP(&ClientDebug, "debug", "d", false, "Print accumulated API calls")
	flags.BoolVarP(&WantJsonOutput, "json", "j", false, "print outputs as json")
	flags.BoolVarP(&TxPretend, "pretend", "n", false, "Enables check-only mode for transactions")
	flags.BoolVar(&Prove, "prove", false, "Request a receipt proving the transaction or account")
	flags.BoolVar(&TxNoWait, "no-wait", false, "Don't wait for the transaction to complete")
	flags.BoolVar(&TxIgnorePending, "ignore-pending", false, "Ignore pending transactions. Combined with --wait, this waits for transactions to be delivered.")
	flags.DurationVarP(&TxWait, "wait", "w", 0, "Wait for the transaction to complete")
	flags.StringVarP(&Memo, "memo", "m", Memo, "Memo")
	flags.StringVarP(&Metadata, "metadata", "a", Metadata, "Transaction Metadata")
	flags.StringSliceVar(&Authorities, "authority", nil, "Additional authorities to add when creating an account")
	flags.StringSliceVar(&Delegators, "delegator", nil, "Specifies the delegator when creating a delegated signature")
	flags.StringSliceVar(&AdditionalSigners, "sign-with", nil, "Specifies additional keys to sign the transaction with")
	flags.UintVar(&SignerVersion, "signer-version", uint(0), "Specify the signer version. Overrides the default behavior of fetching the signer version.")
	//add the commands
	cmd.AddCommand(encryptCmd)
	cmd.AddCommand(accountCmd)
	cmd.AddCommand(adiCmd)
	cmd.AddCommand(authCmd)
	cmd.AddCommand(bookCmd)
	cmd.AddCommand(creditsCmd)
	cmd.AddCommand(dataCmd)
	cmd.AddCommand(getCmd)
	cmd.AddCommand(keyCmd)
	cmd.AddCommand(oracleCmd)
	cmd.AddCommand(pageCmd)
	cmd.AddCommand(tokenCmd)
	cmd.AddCommand(txCmd)
	cmd.AddCommand(blocksCmd)
	cmd.AddCommand(operatorCmd, validatorCmd)
	cmd.AddCommand(versionCmd, describeCmd)

	//for the testnet integration
	cmd.AddCommand(faucetCmd)

	cmd.PersistentPreRunE = func(*cobra.Command, []string) error {
		switch serverAddr {
		case "local":
			serverAddr = "http://127.0.1.1:26660/v2"
		case "localhost":
			serverAddr = "http://127.0.0.1:26660/v2"
		case "devnet":
			serverAddr = "https://devnet.accumulatenetwork.io/v2"
		case "testnet":
			serverAddr = "https://testnet.accumulatenetwork.io/v2"
		}

		var err error
		Client, err = client.New(serverAddr)
		if err != nil {
			return fmt.Errorf("failed to create client: %v", err)
		}
		Client.Timeout = ClientTimeout
		Client.DebugRequest = ClientDebug

		if TxNoWait {
			TxWait = 0
		}

		return nil
	}

	cmd.PersistentPostRun = func(*cobra.Command, []string) {
		if DidError != nil {
			os.Exit(1)
		}
	}

	return cmd
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	rootCmd := InitRootCmd(nil)
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize()
}

var (
	BucketAnon              = []byte("anon")
	BucketAdi               = []byte("adi")
	BucketKeys              = []byte("keys")
	BucketLabel             = []byte("label")
	BucketLite              = []byte("lite")
	BucketMnemonic          = []byte("mnemonic")
	BucketKeyInfo           = []byte("keyinfo")
	BucketSigTypeDeprecated = []byte("sigtype")
)

func initDB(defaultWorkDir string, memDb bool) db.DB {
	var ret db.DB
	if memDb {
		getMnemonic()
		ret = new(db.MemoryDB)

		err := ret.InitDB("", "")
		if err != nil {
			log.Fatal(err)
		}
		return ret
	} else {
		err := os.MkdirAll(defaultWorkDir, 0700)
		if err != nil {
			log.Fatal(err)
		}

		ret = new(db.BoltDB)
	}

	//search for encrypted database first (this is the default)
	databaseName := filepath.Join(defaultWorkDir, "wallet_encrypted.db")
	if _, err := os.Stat(databaseName); errors.Is(err, os.ErrNotExist) || UseUnencryptedWallet {
		// path/to/whatever does not exist -- OR -- we want to fall back to the unencrypted database
		databaseName = filepath.Join(defaultWorkDir, "wallet.db")

		if _, err = os.Stat(databaseName); errors.Is(err, os.ErrNotExist) && !UseUnencryptedWallet {
			//no databases exist, so create a new encrypted database unless the user wanted an unencrypted one
			databaseName = filepath.Join(defaultWorkDir, "wallet_encrypted.db")

			//get mnemonic
			getMnemonic()

			//let's get a new first-time password...
			Password, err = newPassword()
			if err != nil {
				log.Fatal(err)
			}
		}
	} else {
		if !UseUnencryptedWallet {
			bytepw1, err := gopass.GetPasswdPrompt("Password: ", true, os.Stdin, os.Stderr) // term.ReadPassword(int(syscall.Stdin))
			if err != nil {
				log.Fatal(db.ErrNoPassword)
			}

			Password = string(bytepw1)
		}
	}

	err := ret.InitDB(databaseName, Password)

	if err != nil {
		if err != db.ErrDatabaseNotEncrypted {
			log.Fatal(err)
		}
		//so here we want to implicitly indicate to the rest of the code we're using the unencrypted wallet
		UseUnencryptedWallet = true
	}

	return ret

}

func getMnemonic() {
	res, err := promptGetMnemonicOption()
	if err != nil {
		log.Fatal(err)
	}
	switch res {
	case "1":
		entropy := make([]byte, 128)
		_, err := rand.Read(entropy)
		if err != nil {
			log.Fatal(err)
		}
		mnemonicString, err := bip39.NewMnemonic(entropy)
		if err != nil {
			log.Fatal(err)
		}
		_, err = promptMnemonic(mnemonicString)
		if err != nil {
			log.Fatal(err)
		}
		mnemonicConfirm, err := promptMnemonicConfirm()
		if err != nil {
			log.Fatal(err)
		}
		if mnemonicString != mnemonicConfirm {
			log.Fatal("mnemonic doesn't match.")
		}
		mnemonic := strings.Split(mnemonicString, " ")
		_, err = ImportMnemonic(mnemonic)
		if err != nil {
			log.Fatal(err)
		}
	case "2":
		mnemonicBytes, err := gopass.GetPasswdPrompt("Enter Mnemonic: ", false, os.Stdin, os.Stderr)
		if err != nil {
			log.Fatal(err)
		}
		mnemonic := strings.Split(string(mnemonicBytes), " ")
		_, err = ImportMnemonic(mnemonic)
		if err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatal("invalid mnemonic option selected.")
	}

}

type promptContent struct {
	errorMsg string
	label    string
}

func promptGetMnemonicOption() (string, error) {
	pc := promptContent{
		"Please select an option.",
		"Select an option." +
			" 1. Create Mnemonic" +
			" 2. Import Mnemonic",
	}
	items := []string{"1", "2"}
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
