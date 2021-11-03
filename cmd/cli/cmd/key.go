package cmd

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/boltdb/bolt"
	"github.com/spf13/cobra"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
	"log"
	"strconv"
	"strings"
)

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Create and manage Keys for ADI Key Books, and Pages",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "import":
				if len(args) > 2 {
					switch args[1] {
					case "mnemonic":
						ImportMnemonic(args[2:])
					default:
						ImportKey(args[1], args[2])
					}
				} else {
					PrintKeyImport()
				}
			case "export":
				if len(args) > 1 {
					switch args[1] {
					case "all":
						ExportKeys()
					case "seed":
						ExportSeed()
					case "name":
						if len(args) > 2 {
							ExportKey(args[2])
						} else {
							PrintKeyExport()
						}
					case "mnemonic":
						ExportMnemonic()
					default:
						PrintKeyExport()
					}
				} else {
					PrintKeyExport()
				}
			case "list":
				ListKeyPublic()
			case "generate":
				if len(args) > 1 {
					GenerateKey(args[1])
				} else {
					PrintKeyGenerate()
				}
			default:
				fmt.Println("Usage:")
				PrintKey()
			}
		} else {
			fmt.Println("Usage:")
			PrintKey()
		}

	},
}

func init() {
	rootCmd.AddCommand(keyCmd)
}

func PrintKeyPublic() {
	fmt.Println("  accumulate key list			List generated keys associated with the wallet")
}

func PrintKeyExport() {
	fmt.Println("  accumulate key export all			            export all keys in wallet")
	fmt.Println("  accumulate key export name [key label]			export key by key name")
	fmt.Println("  accumulate key export mnemonic		            export the mnemonic phrase if one was entered")
	fmt.Println("  accumulate key export seed                       export the seed generated from the mnemonic phrase")
}

func PrintKeyGenerate() {
	fmt.Println("  accumulate key generate [key name]     Generate a new key and give it a name in the wallet")
}

func PrintKeyImport() {
	fmt.Println("  accumulate key import mnemonic [mnemonic phrase...]     Import the mneumonic phrase used to generate keys in the wallet")
	fmt.Println("  accumulate key import [private key hex] [key name]      Import a key and give it a name in the wallet")
}

func PrintKey() {
	PrintKeyGenerate()
	PrintKeyPublic()
	PrintKeyImport()
}

func pubKeyFromString(s string) ([]byte, error) {
	var pubKey types.Bytes32
	if len(s) != 64 {
		return nil, fmt.Errorf("invalid public key or wallet key label")
	}
	i, err := hex.Decode(pubKey[:], []byte(s))

	if err != nil {
		return nil, err
	}

	if i != 32 {
		return nil, fmt.Errorf("invalid public key")
	}

	return pubKey[:], nil
}

func getPublicKey(s string) ([]byte, error) {
	var pubKey types.Bytes32
	privKey, err := LookupByLabel(s)

	if err != nil {
		b, err := pubKeyFromString(s)
		if err != nil {
			return nil, fmt.Errorf("unable to resolve public key %s,%v", s, err)
		}
		pubKey.FromBytes(b)
	} else {
		pubKey.FromBytes(privKey[32:])
	}

	return pubKey[:], nil
}

func LookupByAnon(anon string) (privKey []byte, err error) {
	err = Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("anon"))
		privKey = b.Get([]byte(anon))
		if len(privKey) == 0 {
			err = fmt.Errorf("valid key not found for %s", anon)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return
}

func LookupByLabel(label string) (asData []byte, err error) {
	err = Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("label"))
		asData = b.Get([]byte(label))
		if len(asData) == 0 {
			err = fmt.Errorf("valid key not found for %s", label)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return LookupByPubKey(asData)
}

func LookupByPubKey(pubKey []byte) (asData []byte, err error) {
	err = Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("keys"))
		asData = b.Get(pubKey)
		return err
	})
	return
}

func GenerateKey(label string) {

	if _, err := strconv.ParseInt(label, 10, 64); err == nil {
		log.Fatal("label cannot be a number")
	}
	_, err := LookupByLabel(label)
	if err == nil {
		log.Fatal(fmt.Errorf("key already exists for label %s", label))
	}
	privKey, err := GeneratePrivateKey()

	if err != nil {
		log.Fatal(err)
	}

	pubKey := privKey[32:]

	err = Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("keys"))
		err := b.Put(pubKey, privKey)
		return err
	})

	err = Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("label"))
		err := b.Put([]byte(label), pubKey)
		return err
	})

	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Public Key %s : %x", label, pubKey)
}

func ListKeyPublic() {

	fmt.Printf("Key name \t\t Public Key\n")
	err := Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("label"))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			fmt.Printf("%s \t\t %x\n", k, v)
		}
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}
}

// ImportKey will import the private key and assign it to the label
func ImportKey(pkhex string, label string) {
	_, err := LookupByLabel(label)
	if err == nil {
		log.Fatal("key label is already being used")
	}

	var pk ed25519.PrivateKey

	token, err := hex.DecodeString(pkhex)
	if err != nil {
		log.Fatal(err)
	}

	if len(token) == 32 {
		pk = ed25519.NewKeyFromSeed(token)
	} else {
		pk = token
	}

	_, err = LookupByPubKey(pk[32:])
	lab := ""
	if err == nil {

		err = Db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("label"))
			c := b.Cursor()
			for k, v := c.First(); k != nil; k, v = c.Next() {
				if bytes.Equal(v, pk[32:]) {
					lab = string(k)
					break
				}
			}
			return nil
		})
		log.Fatalf("private key already exists in wallet as label %s", lab)
	}

	err = Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("keys"))
		err := b.Put([]byte(pk[32:]), pk)
		return err
	})

	err = Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("label"))
		err := b.Put([]byte(label), pk[32:])
		return err
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("{\"label\":\"%s\",\"publicKey\":\"%x\"}\n", label, pk[32:])
}

func ExportKey(label string) {
	pk, err := LookupByLabel(label)
	if err != nil {
		log.Fatalf("no private key found for label %s", label)
	}
	//fmt.Println(hex.EncodeToString(pk))
	fmt.Printf("{\"label\":\"%s\",\"privateKey\":\"%x\",\"publicKey\":\"%x\"}\n", label, pk[:32], pk[32:])
}

func GeneratePrivateKey() (privKey []byte, err error) {
	seed, err := lookupSeed()

	if err != nil {
		//if private key seed doesn't exist, just create a key
		_, privKey, err = ed25519.GenerateKey(nil)
		if err != nil {
			return nil, err
		}
	} else {
		//if we do have a seed, then create a new key
		masterKey, _ := bip32.NewMasterKey(seed)

		newKey, err := masterKey.NewChildKey(uint32(getKeyCountAndIncrement()))
		if err != nil {
			return nil, err
		}
		privKey = ed25519.NewKeyFromSeed(newKey.Key)
	}
	return
}

func getKeyCountAndIncrement() (count uint32) {
	err := Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("mnemonic"))
		if b != nil {
			ct := b.Get([]byte("count"))
			if ct != nil {
				count = binary.LittleEndian.Uint32(ct)
			}
			ct = make([]byte, 8)
			binary.LittleEndian.PutUint32(ct, count+1)
			return b.Put([]byte("count"), ct)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	return count
}

func lookupSeed() (seed []byte, err error) {

	err = Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("mnemonic"))
		seed = b.Get([]byte("seed"))
		if len(seed) == 0 {
			err = fmt.Errorf("mnemonic seed doesn't exist")
		}
		return err
	})

	return
}

func ImportMnemonic(mnemonic []string) {
	mns := strings.Join(mnemonic, " ")

	if !bip39.IsMnemonicValid(mns) {

		log.Fatal("invalid mnemonic provided")
	}

	// Generate a Bip32 HD wallet for the mnemonic and a user supplied password
	seed := bip39.NewSeed(mns, "")

	var err error

	err = Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("mnemonic"))
		seed := b.Get([]byte("seed"))
		if len(seed) != 0 {
			err = fmt.Errorf("mnemonic seed phrase already exists within wallet")
		}
		return err
	})
	if err != nil {
		log.Fatal(err)
	}

	err = Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("mnemonic"))
		if b != nil {
			b.Put([]byte("seed"), seed)
			b.Put([]byte("phrase"), []byte(mns))
		} else {
			return fmt.Errorf("DB: %s", err)
		}
		return nil
	})

}

func ExportKeys() {
	err := Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("keys"))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, _ = c.Next() {
			fmt.Printf("%s %x\n", k, v)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}

func ExportSeed() {
	err := Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("mnemonic"))
		if b != nil {
			seed := b.Get([]byte("seed"))
			fmt.Printf(" seed: %x\n", seed)
		} else {
			return fmt.Errorf("mnemonic seed not found")
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}

func ExportMnemonic() {
	err := Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("mnemonic"))
		if b != nil {
			seed := b.Get([]byte("phrase"))
			fmt.Printf("mnemonic phrase: %x\n", seed)
		} else {
			return fmt.Errorf("mnemonic seed not found")
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
