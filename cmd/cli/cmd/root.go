package cmd

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/boltdb/bolt"
	"github.com/spf13/cobra"

	"github.com/AccumulateNetwork/accumulated/client"
	"github.com/AccumulateNetwork/accumulated/smt/storage/database"
)

var (
	Client = client.NewAPIClient()
	Db     = initDB() //initManagedDB()
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

const (
	BucketAnon     = "anon"
	BucketAdi      = "adi"
	BucketKeys     = "keys"
	BucketLabel    = "label"
	BucketMnemonic = "mnemonic"
)

func initDB() *bolt.DB {
	err := os.MkdirAll(defaultWorkDir, 0600)
	if err != nil {
		log.Fatal(err)
	}

	db, err := bolt.Open(defaultWorkDir+"/wallet.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}

	//err = db.Update(func(tx *bolt.Tx) error {
	//	_, err := tx.CreateBucket([]byte("anon"))
	//	if err != nil {
	//		return fmt.Errorf("DB: %s", err)
	//	}
	//	return nil
	//})

	err = db.Update(func(tx *bolt.Tx) error {
		_, err = tx.CreateBucket([]byte("adi"))
		if err != nil {
			return fmt.Errorf("DB: %s", err)
		}
		return nil
	})

	err = db.Update(func(tx *bolt.Tx) error {
		_, err = tx.CreateBucket([]byte("keys"))
		if err != nil {
			return fmt.Errorf("DB: %s", err)
		}
		return nil
	})

	err = db.Update(func(tx *bolt.Tx) error {
		_, err = tx.CreateBucket([]byte("label"))
		if err != nil {
			return fmt.Errorf("DB: %s", err)
		}
		return nil
	})

	err = db.Update(func(tx *bolt.Tx) error {
		_, err = tx.CreateBucket([]byte("mnemonic"))
		if err != nil {
			return fmt.Errorf("DB: %s", err)
		}
		return nil
	})
	return db
}
