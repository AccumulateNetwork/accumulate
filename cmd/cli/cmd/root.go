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
)

var currentUser = func() *user.User {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	return usr
}()

var DidError bool

func InitRootCmd(database db.DB) *cobra.Command {
	Db = database
	cmd := &cobra.Command{
		Use:   "accumulate",
		Short: "CLI for Accumulate Network",
	}

	defaultServer := os.Getenv("ACC_API")
	if defaultServer == "" {
		defaultServer = "https://testnet.accumulatenetwork.io/v1"
	}

	flags := cmd.PersistentFlags()
	flags.StringVarP(&Client.Server, "server", "s", defaultServer, "Accumulated server")
	flags.DurationVarP(&Client.Timeout, "timeout", "t", 5*time.Second, "Timeout for all API requests (i.e. 10s, 1m)")
	flags.BoolVarP(&Client.DebugRequest, "debug", "d", false, "Print accumulated API calls")
	flags.BoolVarP(&WantJsonOutput, "json", "j", false, "print outputs as json")

	//add the commands
	cmd.AddCommand(accountCmd)
	cmd.AddCommand(adiCmd)
	cmd.AddCommand(bookCmd)
	cmd.AddCommand(creditsCmd)
	cmd.AddCommand(getCmd)
	cmd.AddCommand(keyCmd)
	cmd.AddCommand(pageCmd)
	cmd.AddCommand(txCmd)
	cmd.AddCommand(versionCmd)
	//cmd.AddCommand(tokenCmd)

	//for the testnet integration
	cmd.AddCommand(faucetCmd)

	cmd.PersistentPostRun = func(*cobra.Command, []string) {
		if DidError {
			os.Exit(1)
		}
	}

	return cmd
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	rootCmd := InitRootCmd(initDB(filepath.Join(currentUser.HomeDir, ".accumulate")))

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

func initDB(defaultWorkDir string) db.DB {

	err := os.MkdirAll(defaultWorkDir, 0600)
	if err != nil {
		log.Fatal(err)
	}

	db := new(db.BoltDB)
	err = db.InitDB(filepath.Join(defaultWorkDir, "wallet.db"))
	if err != nil {
		log.Fatal(err)
	}
	return db
}
