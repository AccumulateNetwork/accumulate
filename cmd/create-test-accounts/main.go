package main

import (
	"crypto/ed25519"
	"fmt"
	"math"
	"math/big"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	// Create 100 pre-funded lite token accounts
	numAccounts := 100
	tokensPerAccount := int64(20_000_000) // 20M ACME per account

	db := coredb.OpenInMemory(nil)
	db.SetObserver(execute.NewDatabaseObserver())
	batch := db.Begin(true)
	defer batch.Discard()

	fmt.Printf("Creating %d test accounts with %d ACME each...\n", numAccounts, tokensPerAccount)

	for i := 0; i < numAccounts; i++ {
		// Generate deterministic key from index
		seed := make([]byte, 32)
		for j := range seed {
			seed[j] = byte(i)
		}
		privKey := ed25519.NewKeyFromSeed(seed)
		pubKey := privKey.Public().(ed25519.PublicKey)

		// Create lite token account URL
		url, err := protocol.LiteTokenAddress(pubKey, "ACME", protocol.SignatureTypeED25519)
		if err != nil {
			panic(err)
		}

		// Create lite token account
		lta := new(protocol.LiteTokenAccount)
		lta.Url = url
		lta.TokenUrl = protocol.AcmeUrl()
		lta.Balance = *big.NewInt(tokensPerAccount * protocol.AcmePrecision)

		// Create lite identity
		lid := new(protocol.LiteIdentity)
		lid.Url = lta.Url.RootIdentity()
		lid.CreditBalance = math.MaxUint64 // Unlimited credits

		// Store accounts
		for _, a := range []protocol.Account{lta, lid} {
			err := batch.Account(a.GetUrl()).Main().Put(a)
			if err != nil {
				panic(err)
			}
		}

		if i < 5 {
			fmt.Printf("  Account %d: %s (key: %x)\n", i, url, privKey.Seed())
		}
	}

	fmt.Println("  ... and 95 more accounts")

	err := batch.UpdateBPT()
	if err != nil {
		panic(err)
	}
	err = batch.Commit()
	if err != nil {
		panic(err)
	}

	// Write snapshot to file
	file, err := os.Create("/tmp/test-accounts.snap")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	buf := new(ioutil.Buffer)
	_, err = db.Collect(buf, nil, &database.CollectOptions{
		Predicate: func(r record.Record) (bool, error) {
			// Do not create a BPT
			if r.Key().Get(0) == "BPT" {
				return false, nil
			}
			return true, nil
		},
	})
	if err != nil {
		panic(err)
	}

	_, err = file.Write(buf.Bytes())
	if err != nil {
		panic(err)
	}

	fmt.Println("\nSnapshot written to /tmp/test-accounts.snap")
	fmt.Printf("Total: 100 accounts x %d ACME = %d million ACME\n", tokensPerAccount, int64(numAccounts)*tokensPerAccount/1_000_000)
}
