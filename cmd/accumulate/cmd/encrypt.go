package cmd

import (
	"bytes"
	"fmt"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/libs/os"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/db"
	"golang.org/x/term"
)

var encryptCmd = &cobra.Command{
	Use:   "encrypt",
	Short: "encrypt the database",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) == 0 {
			out, err = EncryptDatabase()
		} else {
			fmt.Println("Usage:")
			PrintEncrypt()
		}
		printOutput(cmd, out, err)
	},
}

func PrintEncrypt() {
	fmt.Println("  accumulate encrypt 		Encrypt the database, will be prompted for password")
}

func copyBucket(dst db.DB, src db.DB, bucket []byte) error {
	bucketData, err := src.GetBucket(bucket)
	if err != nil {
		return db.ErrNoBucket
	}

	for _, v := range bucketData.KeyValueList {
		err = dst.Put(bucket, v.Key, v.Value)
		if err != nil {
			return err
		}
	}

	return nil
}

func equalBucket(dst db.DB, src db.DB, bucket []byte) bool {

	bucketDataDst, errDst := dst.GetBucket(bucket)
	bucketDataSrc, errSrc := src.GetBucket(bucket)
	if errDst != nil && errSrc != nil {
		if errDst == db.ErrNoBucket {
			return true
		}
		return false
	}

	for _, v := range bucketDataSrc.KeyValueList {
		val := bucketDataDst.Get(v.Key)
		if bytes.Compare(val, v.Value) != 0 {
			return false
		}
	}

	return true
}

func EncryptDatabase() (string, error) {
	if DatabaseEncrypted {
		return "", db.ErrDatabaseAlreadyEncrypted
	}

	fmt.Println("Encrypting Wallet")

	//we will now create a new database
	dbePath := filepath.Join(DatabaseDir, "wallet_encrypted.db")
	if os.FileExists(dbePath) {
		return "", fmt.Errorf("encrypted database wallet already exists %s", dbePath)
	}
	fmt.Print("New Password: ")
	bytepw1, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", db.ErrInvalidPassword
	}

	fmt.Print("\nRe-Enter New Password: ")
	bytepw2, err := term.ReadPassword(int(syscall.Stdin))

	if err != nil {
		return "", db.ErrInvalidPassword
	}

	if bytes.Compare(bytepw1, bytepw2) != 0 {
		return "", fmt.Errorf("passwords do not match")
	}

	Password = string(bytepw1)

	dbe := new(db.BoltDB)

	err = dbe.InitDB(dbePath, Password)
	defer dbe.Close()
	if err != nil {
		return "", err
	}

	if wallet == nil {
		//we don't have an open database so we are creating one from scratch.
		return fmt.Sprintf("success - encrypted wallet created at %s.\n", dbe.Name()), nil
	}

	err = copyBucket(dbe, GetWallet(), BucketLabel)
	if err != nil && err != db.ErrNoBucket {
		return "", err
	}

	err = copyBucket(dbe, GetWallet(), BucketAdi)
	if err != nil && err != db.ErrNoBucket {
		return "", err
	}

	err = copyBucket(dbe, GetWallet(), BucketKeys)
	if err != nil && err != db.ErrNoBucket {
		return "", err
	}

	err = copyBucket(dbe, GetWallet(), BucketLite)
	if err != nil && err != db.ErrNoBucket {
		return "", err
	}

	err = copyBucket(dbe, GetWallet(), BucketMnemonic)
	if err != nil && err != db.ErrNoBucket {
		return "", err
	}

	err = copyBucket(dbe, GetWallet(), BucketSigType)
	if err != nil && err != db.ErrNoBucket {
		return "", err
	}

	if !equalBucket(dbe, GetWallet(), BucketMnemonic) ||
		!equalBucket(dbe, GetWallet(), BucketAdi) ||
		!equalBucket(dbe, GetWallet(), BucketKeys) ||
		!equalBucket(dbe, GetWallet(), BucketLabel) ||
		!equalBucket(dbe, GetWallet(), BucketLite) ||
		!equalBucket(dbe, GetWallet(), BucketSigType) {
		return "", db.ErrMalformedEncryptedDatabase
	}

	msg := fmt.Sprintf("success - encrypted wallet created at %s.\n", dbe.Name())
	msg += fmt.Sprintf("\tImport of unencrypted wallet %s data successful.\n", GetWallet().Name())
	msg += fmt.Sprintf("\tIt is advised you backup unencrypted wallet and/or keys and remove from your system ")

	return msg, nil
}
