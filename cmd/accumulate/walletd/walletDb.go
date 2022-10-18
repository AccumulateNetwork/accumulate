package walletd

import (
	"errors"
	"log"
	"os"
	"path/filepath"

	"github.com/howeyc/gopass"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/db"
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
	BucketAnon              = []byte("anon")
	BucketAdi               = []byte("adi")
	BucketKeys              = []byte("keys")
	BucketLabel             = []byte("label")
	BucketLite              = []byte("lite")
	BucketMnemonic          = []byte("mnemonic")
	BucketKeyInfo           = []byte("keyinfo")
	BucketSigTypeDeprecated = []byte("sigtype")
	BucketTransactionCache  = []byte("TransactionCache")
)
var (
	UseUnencryptedWallet bool
	wallet               db.DB
	Password             string
	DatabaseDir          string
	NoWalletVersionCheck bool
	Entropy              uint
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
	//search for encrypted database first (this is the default)
	databaseName := filepath.Join(defaultWorkDir, "wallet_encrypted.db")
	if _, err := os.Stat(databaseName); errors.Is(err, os.ErrNotExist) || UseUnencryptedWallet {
		// path/to/whatever does not exist -- OR -- we want to fall back to the unencrypted database
		databaseName = filepath.Join(defaultWorkDir, "wallet.db")

		if _, err = os.Stat(databaseName); errors.Is(err, os.ErrNotExist) && !UseUnencryptedWallet {
			//no databases exist, so create a new encrypted database unless the user wanted an unencrypted one
			databaseName = filepath.Join(defaultWorkDir, "wallet_encrypted.db")

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
