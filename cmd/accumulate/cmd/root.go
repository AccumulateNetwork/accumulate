package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/db"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"golang.org/x/term"
)

func GetWallet() db.DB {
	if wallet == nil {
		wallet = initDB(DatabaseDir, false)
	}
	return wallet
}

var (
	Client            *client.Client
	ClientTimeout     time.Duration
	ClientDebug       bool
	DoNotEncrypt      bool
	wallet            db.DB
	WantJsonOutput    = false
	TxPretend         = false
	Prove             = false
	Memo              string
	Metadata          string
	SigType           string
	Authorities       []string
	Password          string
	DatabaseEncrypted bool
	DatabaseDir       string
	ClientMode        bool
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
		serverAddr = "https://testnet2.accumulatenetwork.io/v2"
	}

	flags := cmd.PersistentFlags()
	flags.StringVarP(&serverAddr, "server", "s", serverAddr, "Accumulated server")
	flags.DurationVarP(&ClientTimeout, "timeout", "t", 5*time.Second, "Timeout for all API requests (i.e. 10s, 1m)")
	flags.BoolVar(&DoNotEncrypt, "do-not-encrypt-wallet", false, "Create an unencrypted wallet (strongly discouraged)")
	flags.BoolVarP(&ClientDebug, "debug", "d", false, "Print accumulated API calls")
	flags.BoolVarP(&WantJsonOutput, "json", "j", false, "print outputs as json")
	flags.BoolVarP(&TxPretend, "pretend", "n", false, "Enables check-only mode for transactions")
	flags.BoolVar(&Prove, "prove", false, "Request a receipt proving the transaction or account")
	flags.BoolVar(&TxNoWait, "no-wait", false, "Don't wait for the transaction to complete")
	flags.BoolVar(&TxIgnorePending, "ignore-pending", false, "Ignore pending transactions. Combined with --wait, this waits for transactions to be delivered.")
	flags.DurationVarP(&TxWait, "wait", "w", 0, "Wait for the transaction to complete")
	flags.StringVarP(&Memo, "memo", "m", Memo, "Memo")
	flags.StringVarP(&Metadata, "metadata", "a", Metadata, "Transaction Metadata")
	flags.BoolVar(&ClientMode, "client-mode", false, "pipe in password in client mode")
	flags.StringSliceVar(&Authorities, "authority", nil, "Additional authorities to add when creating an account")

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
	cmd.AddCommand(validatorCmd)
	cmd.AddCommand(versionCmd)

	//for the testnet integration
	cmd.AddCommand(faucetCmd)

	cmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
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

		out, err := RestoreAccounts()
		if err != nil && err != db.ErrNoBucket {
			return err
		}
		if out != "" {
			cmd.Println("performing account database update")
			cmd.Println(out)
		}

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
	DatabaseDir = filepath.Join(currentUser.HomeDir, ".accumulate")
	rootCmd := InitRootCmd(nil)
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize()
}

var (
	BucketAnon     = []byte("anon")
	BucketAdi      = []byte("adi")
	BucketKeys     = []byte("keys")
	BucketLabel    = []byte("label")
	BucketLite     = []byte("lite")
	BucketMnemonic = []byte("mnemonic")
	BucketSigType  = []byte("sigtype")
)

func initDB(defaultWorkDir string, memDb bool) db.DB {
	var ret db.DB
	if memDb {
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
	//search for encrypted database first
	databaseName := filepath.Join(defaultWorkDir, "wallet_encrypted.db")
	if _, err := os.Stat(databaseName); errors.Is(err, os.ErrNotExist) {
		// path/to/whatever does not exist
		databaseName = filepath.Join(defaultWorkDir, "wallet.db")
		if _, err := os.Stat(databaseName); errors.Is(err, os.ErrNotExist) && !DoNotEncrypt {
			//no database exists, so create a new encrypted database
			databaseName = filepath.Join(defaultWorkDir, "wallet_encrypted.db")
			//lets make an encrypted database...
			_, err = EncryptDatabase()
			if err != nil {
				log.Fatal(err)
			}
		}
	} else {
		if Password == "" {
			if ClientMode {
				scanner := bufio.NewScanner(os.Stdin)
				if scanner.Scan() {
					Password = scanner.Text()
				}
			} else {
				os.Stderr.WriteString("Password: ")
				bytepw1, err := term.ReadPassword(int(syscall.Stdin))
				if err != nil {
					log.Fatal(db.ErrNoPassword)
				}

				Password = string(bytepw1)
			}
		}
	}

	err := ret.InitDB(databaseName, Password)

	DatabaseEncrypted = true
	if err != nil {
		if err != db.ErrDatabaseNotEncrypted {
			log.Fatal(err)
		}
		DatabaseEncrypted = false
	}

	return ret

}
