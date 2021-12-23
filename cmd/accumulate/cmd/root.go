package cmd

import (
	"log"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/AccumulateNetwork/accumulate/client"
	"github.com/AccumulateNetwork/accumulate/cmd/cli/db"
	"github.com/spf13/cobra"
)

var (
	Client         = client.NewAPIClient()
	Db             db.DB
	WantJsonOutput = false
	TxPretend      = false
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

	defaultServer := os.Getenv("ACC_API")
	if defaultServer == "" {
		defaultServer = "https://testnet.accumulatenetwork.io/v2"
	}

	flags := cmd.PersistentFlags()
	flags.StringVarP(&Client.Server, "server", "s", defaultServer, "Accumulated server")
	flags.DurationVarP(&Client.Timeout, "timeout", "t", 5*time.Second, "Timeout for all API requests (i.e. 10s, 1m)")
	flags.BoolVarP(&Client.DebugRequest, "debug", "d", false, "Print accumulated API calls")
	flags.BoolVarP(&WantJsonOutput, "json", "j", false, "print outputs as json")
	flags.BoolVarP(&TxPretend, "pretend", "n", false, "Enables check-only mode for transactions")

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
	//cmd.AddCommand(tokenCmd)

	//for the testnet integration
	cmd.AddCommand(faucetCmd)

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
	if memDb == true {
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
