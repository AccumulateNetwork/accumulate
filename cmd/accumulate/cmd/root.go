package cmd

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/db"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
)

var (
	Client         *client.Client
	ClientTimeout  time.Duration
	ClientDebug    bool
	Db             db.DB
	WantJsonOutput = false
	TxPretend      = false
	Prove          = false
	Memo           string
	Metadata       string
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
	Db = database
	cmd := &cobra.Command{
		Use:   "accumulate",
		Short: "CLI for Accumulate Network",
	}

	cmd.SetOut(os.Stdout)

	serverAddr := os.Getenv("ACC_API")
	if serverAddr == "" {
		serverAddr = "https://bvn0.testnet.accumulatenetwork.io/v2"
	}

	flags := cmd.PersistentFlags()
	flags.StringVarP(&serverAddr, "server", "s", serverAddr, "Accumulated server")
	flags.DurationVarP(&ClientTimeout, "timeout", "t", 5*time.Second, "Timeout for all API requests (i.e. 10s, 1m)")
	flags.BoolVarP(&ClientDebug, "debug", "d", false, "Print accumulated API calls")
	flags.BoolVarP(&WantJsonOutput, "json", "j", false, "print outputs as json")
	flags.BoolVarP(&TxPretend, "pretend", "n", false, "Enables check-only mode for transactions")
	flags.BoolVar(&Prove, "prove", false, "Request a receipt proving the transaction or account")
	flags.BoolVar(&TxNoWait, "no-wait", false, "Don't wait for the transaction to complete")
	flags.DurationVarP(&TxWait, "wait", "w", 0, "Wait for the transaction to complete")
	flags.StringVarP(&Memo, "memo", "m", Memo, "Memo")
	flags.StringVarP(&Metadata, "metadata", "a", Metadata, "Transaction Metadata")

	//add the commands
	cmd.AddCommand(accountCmd)
	cmd.AddCommand(adiCmd)
	cmd.AddCommand(bookCmd)
	cmd.AddCommand(creditsCmd)
	cmd.AddCommand(dataCmd)
	cmd.AddCommand(getCmd)
	cmd.AddCommand(keyCmd)
	cmd.AddCommand(pageCmd)
	cmd.AddCommand(txCmd)
	cmd.AddCommand(versionCmd)
	cmd.AddCommand(tokenCmd)
	cmd.AddCommand(managerCmd)
	cmd.AddCommand(oracleCmd)
	cmd.AddCommand(blocksCmd)
	cmd.AddCommand(validatorCmd)

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
	rootCmd := InitRootCmd(initDB(filepath.Join(currentUser.HomeDir, ".accumulate"), false))

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
	BucketMnemonic = []byte("mnemonic")
)

func initDB(defaultWorkDir string, memDb bool) db.DB {
	var ret db.DB
	if memDb {
		ret = new(db.MemoryDB)
	} else {
		err := os.MkdirAll(defaultWorkDir, 0700)
		if err != nil {
			log.Fatal(err)
		}

		ret = new(db.BoltDB)
	}
	err := ret.InitDB(filepath.Join(defaultWorkDir, "wallet.db"))
	if err != nil {
		log.Fatal(err)
	}
	return ret

}
