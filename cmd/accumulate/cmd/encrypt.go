package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/howeyc/gopass"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/db"
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

//newPassword Get a new password.
func newPassword() (string, error) {
	bytepw1, err := gopass.GetPasswdPrompt("New Password: ", true, os.Stdin, os.Stderr) //term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", db.ErrInvalidPassword
	}

	if len(bytepw1) < 8 {
		return "", fmt.Errorf("%v: password length must be a minimum of 8 characters", db.ErrInvalidPassword)
	}

	bytepw2, err := gopass.GetPasswdPrompt("Re-Enter New Password: ", true, os.Stdin, os.Stderr) //term.ReadPassword(int(syscall.Stdin))

	if err != nil {
		return "", db.ErrInvalidPassword
	}

	if bytes.Compare(bytepw1, bytepw2) != 0 {
		return "", fmt.Errorf("passwords do not match")
	}

	return string(bytepw1), nil
}

func EncryptDatabase() (string, error) {
	//we will test to see if we already have an unencrypted database
	dbePath := filepath.Join(DatabaseDir, "wallet_encrypted.db")
	if _, err := os.Stat(dbePath); !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("encrypted database wallet already exists %s", dbePath)
	}

	// check to see if unencrypted wallet is present.
	unencryptedWalletPath := filepath.Join(DatabaseDir, "wallet.db")
	haveUnencryptedWallet := false
	if _, err := os.Stat(unencryptedWalletPath); !errors.Is(err, os.ErrNotExist) {
		haveUnencryptedWallet = true
	}

	_, _ = os.Stderr.WriteString("Creating an encrypted database at " + dbePath + "\n")

	var err error
	Password, err = newPassword()
	if err != nil {
		return "", err
	}

	//if we have an unencrypted wallet we want to now open it to use to set up the new encrypted wallet
	var dbu *db.BoltDB
	if haveUnencryptedWallet {
		dbu = new(db.BoltDB)
		err = dbu.InitDB(unencryptedWalletPath, "")
		defer dbu.Close()
		if err != nil && err != db.ErrDatabaseNotEncrypted {
			return "", err
		}
	}

	//now create the new encrypted database
	dbe := new(db.BoltDB)
	err = dbe.InitDB(dbePath, Password)
	defer dbe.Close()
	if err != nil {
		return "", err
	}

	if !haveUnencryptedWallet {
		//if we don't have an unencrypted wallet, nothing left to do -- we're done.
		return fmt.Sprintf("success - encrypted wallet created at %s.\n", dbe.Name()), nil
	}

	err = copyBucket(dbe, dbu, BucketLabel)
	if err != nil && err != db.ErrNoBucket {
		return "", err
	}

	err = copyBucket(dbe, dbu, BucketAdi)
	if err != nil && err != db.ErrNoBucket {
		return "", err
	}

	err = copyBucket(dbe, dbu, BucketKeys)
	if err != nil && err != db.ErrNoBucket {
		return "", err
	}

	err = copyBucket(dbe, dbu, BucketLite)
	if err != nil && err != db.ErrNoBucket {
		return "", err
	}

	err = copyBucket(dbe, dbu, BucketMnemonic)
	if err != nil && err != db.ErrNoBucket {
		return "", err
	}

	err = copyBucket(dbe, dbu, BucketKeyInfo)
	if err != nil && err != db.ErrNoBucket {
		return "", err
	}

	if !equalBucket(dbe, dbu, BucketMnemonic) ||
		!equalBucket(dbe, dbu, BucketAdi) ||
		!equalBucket(dbe, dbu, BucketKeys) ||
		!equalBucket(dbe, dbu, BucketLabel) ||
		!equalBucket(dbe, dbu, BucketLite) ||
		!equalBucket(dbe, dbu, BucketKeyInfo) {
		return "", db.ErrMalformedEncryptedDatabase
	}

	msg := fmt.Sprintf("\nSuccess:\tEncrypted wallet created at %s.\n", dbe.Name())
	msg += fmt.Sprintf("\t\tImported unencrypted data from wallet %s.\n", dbu.Name())
	msg += fmt.Sprintf("\t\tIt is advised you backup your unencrypted wallet and/or keys, then remove\n\t\t%s from your system.\n", dbu.Name())

	return msg, nil
}
