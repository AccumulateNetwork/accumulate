package cmd

import (
	"github.com/AccumulateNetwork/accumulate/cmd/cli/db"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/AccumulateNetwork/accumulate/client"
	"github.com/AccumulateNetwork/accumulate/smt/storage/database"
)

var (
	Client         = client.NewAPIClient()
	Db             = initDB() //initManagedDB()
	WantJsonOutput = false
)

var currentUser = func() *user.User {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	return usr
}()

var defaultWorkDir = filepath.Join(currentUser.HomeDir, ".accumulate")

// rootCmd represents the base command when called without any subcommands
var rootCmd = func() *cobra.Command {

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

	return cmd

}()

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize()
}

func initManagedDB() *database.Manager {
	err := os.MkdirAll(defaultWorkDir, 0600)
	if err != nil {
		log.Fatal(err)
	}

	db, err := database.NewDBManager("badger", defaultWorkDir+"/wallet.db")
	if err != nil {
		log.Fatal(err)
	}
	return db
}

var (
	BucketAnon     = []byte("anon")
	BucketAdi      = []byte("adi")
	BucketKeys     = []byte("keys")
	BucketLabel    = []byte("label")
	BucketMnemonic = []byte("mnemonic")
)

func initDB() db.DB {
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
