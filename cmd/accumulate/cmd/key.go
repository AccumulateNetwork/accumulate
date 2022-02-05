package cmd

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tyler-smith/go-bip39"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Create and manage Keys for ADI Key Books, and Pages",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "import":
				if len(args) == 3 {
					if args[1] == "lite" {
						out, err = ImportKey(args[2], "")
					} else {
						PrintKeyImport()
					}
				} else if len(args) > 3 {
					switch args[1] {
					case "mnemonic":
						out, err = ImportMnemonic(args[2:])
					case "private":
						out, err = ImportKey(args[2], args[3])
					case "public":
						//reserved for future use.
						fallthrough
					default:
						PrintKeyImport()
					}
				} else {
					PrintKeyImport()
				}
			case "export":
				if len(args) > 1 {
					switch args[1] {
					case "all":
						out, err = ExportKeys()
					case "seed":
						out, err = ExportSeed()
					case "private":
						if len(args) > 2 {
							out, err = ExportKey(args[2])
						} else {
							PrintKeyExport()
						}
					case "mnemonic":
						out, err = ExportMnemonic()
					default:
						PrintKeyExport()
					}
				} else {
					PrintKeyExport()
				}
			case "list":
				out, err = ListKeyPublic()
			case "generate":
				if len(args) > 1 {
					out, err = GenerateKey(args[1])
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
		printOutput(cmd, out, err)
	},
}

type KeyResponse struct {
	Label      types.String `json:"name,omitempty"`
	PrivateKey types.Bytes  `json:"privateKey,omitempty"`
	PublicKey  types.Bytes  `json:"publicKey,omitempty"`
	Seed       types.Bytes  `json:"seed,omitempty"`
	Mnemonic   types.String `json:"mnemonic,omitempty"`
}

func PrintKeyPublic() {
	fmt.Println("  accumulate key list			List generated keys associated with the wallet")
}

func PrintKeyExport() {
	fmt.Println("  accumulate key export all			            export all keys in wallet")
	fmt.Println("  accumulate key export private [key name]			export the private key by key name")
	fmt.Println("  accumulate key export mnemonic		            export the mnemonic phrase if one was entered")
	fmt.Println("  accumulate key export seed                       export the seed generated from the mnemonic phrase")
}

func PrintKeyGenerate() {
	fmt.Println("  accumulate key generate [key name]     Generate a new key and give it a name in the wallet")
}

func PrintKeyImport() {
	fmt.Println("  accumulate key import mnemonic [mnemonic phrase...]     Import the mneumonic phrase used to generate keys in the wallet")
	fmt.Println("  accumulate key import private [private key hex] [key name]      Import a key and give it a name in the wallet")
	fmt.Println("  accumulate key import lite [private key hex]       Import a key as a lite address")
}

func PrintKey() {
	PrintKeyGenerate()
	PrintKeyPublic()
	PrintKeyImport()

	PrintKeyExport()
}

func GenerateKey(label string) (string, error) {
	if _, err := strconv.ParseInt(label, 10, 64); err == nil {
		return "", fmt.Errorf("key name cannot be a number")
	}

	privKey, err := GeneratePrivateKey()

	if err != nil {
		return "", err
	}

	pubKey := privKey[32:]

	if label == "" {
		ltu, err := protocol.LiteTokenAddress(pubKey, protocol.AcmeUrl().String())
		if err != nil {
			return "", fmt.Errorf("unable to create lite account")
		}
		label = ltu.String()
	}

	_, err = LookupByLabel(label)
	if err == nil {
		return "", fmt.Errorf("key already exists for key name %s", label)
	}

	err = Db.Put(BucketKeys, pubKey, privKey)
	if err != nil {
		return "", err
	}

	err = Db.Put(BucketLabel, []byte(label), pubKey)
	if err != nil {
		return "", err
	}

	if label, ok := LabelForLiteTokenAccount(label); ok {
		err = Db.Put(BucketLabel, []byte(label), pubKey)
		if err != nil {
			return "", err
		}
	}

	if WantJsonOutput {
		a := KeyResponse{}
		a.Label = types.String(label)
		a.PublicKey = pubKey
		return PrintJson(&a)
	}

	return fmt.Sprintf("%s :\t%x", label, pubKey), nil
}

func ListKeyPublic() (out string, err error) {
	out = fmt.Sprintf("%s\t\t\t\t\t\t\t\tKey name\n", "Public Key")
	b, err := Db.GetBucket(BucketLabel)
	if err != nil {
		return "", err
	}

	for _, v := range b.KeyValueList {
		out += fmt.Sprintf("%x\t%s\n", v.Value, v.Key)
	}
	return out, nil
}

// ImportKey will import the private key and assign it to the label
func ImportKey(pkhex string, label string) (out string, err error) {

	var pk ed25519.PrivateKey

	token, err := hex.DecodeString(pkhex)
	if err != nil {
		return "", err
	}

	if len(token) == 32 {
		pk = ed25519.NewKeyFromSeed(token)
	} else {
		pk = token
	}

	if label == "" {
		lt, err := protocol.LiteTokenAddress(pk[32:], protocol.AcmeUrl().String())
		if err != nil {
			return "", fmt.Errorf("no label specified and cannot import as lite account")
		}
		label = lt.String()
	}

	_, err = LookupByLabel(label)
	if err == nil {
		return "", fmt.Errorf("key name is already being used")
	}

	_, err = LookupByPubKey(pk[32:])
	lab := "not found"
	if err == nil {
		b, _ := Db.GetBucket(BucketLabel)
		if b != nil {
			for _, v := range b.KeyValueList {
				if bytes.Equal(v.Value, pk[32:]) {
					lab = string(v.Key)
					break
				}
			}
			return "", fmt.Errorf("private key already exists in wallet by key name of %s", lab)
		}
	}

	err = Db.Put(BucketKeys, pk[32:], pk)
	if err != nil {
		return "", err
	}

	err = Db.Put(BucketLabel, []byte(label), pk[32:])
	if err != nil {
		return "", err
	}

	if WantJsonOutput {
		a := KeyResponse{}
		a.Label = types.String(label)
		a.PublicKey = types.Bytes(pk[32:])
		return PrintJson(&a)
	}

	out = fmt.Sprintf("\tname\t\t:%s\n\tpublic key\t:%x\n", label, pk[32:])
	return out, nil
}

func ExportKey(label string) (string, error) {
	pk, err := LookupByLabel(label)
	if err != nil {
		pubk, err := pubKeyFromString(label)
		if err != nil {
			return "", fmt.Errorf("no private key found for key name %s", label)
		}
		pk, err = LookupByPubKey(pubk)
		if err != nil {
			return "", fmt.Errorf("no private key found for key name %s", label)
		}
		label, err = FindLabelFromPubKey(pubk)
		if err != nil {
			return "", fmt.Errorf("no private key found for key name %s", label)
		}
	}

	if WantJsonOutput {
		a := KeyResponse{}
		a.Label = types.String(label)
		a.PrivateKey = pk[:32]
		a.PublicKey = pk[32:]
		return PrintJson(&a)
	}

	return fmt.Sprintf("name\t\t\t:\t%s\n\tprivate key\t:\t%x\n\tpublic key\t:\t%x\n", label, pk[:32], pk[32:]), nil
}

func ImportMnemonic(mnemonic []string) (string, error) {
	mns := strings.Join(mnemonic, " ")

	if !bip39.IsMnemonicValid(mns) {
		return "", fmt.Errorf("invalid mnemonic provided")
	}

	// Generate a Bip32 HD wallet for the mnemonic and a user supplied password
	seed := bip39.NewSeed(mns, "")

	root, err := Db.Get(BucketMnemonic, []byte("seed"))
	if len(root) != 0 {
		return "", fmt.Errorf("mnemonic seed phrase already exists within wallet")
	}

	err = Db.Put(BucketMnemonic, []byte("seed"), seed)
	if err != nil {
		return "", fmt.Errorf("DB: seed write error, %v", err)
	}

	err = Db.Put(BucketMnemonic, []byte("phrase"), []byte(mns))
	if err != nil {
		return "", fmt.Errorf("DB: phrase write error %s", err)
	}

	return "mnemonic import successful", nil
}

func ExportKeys() (out string, err error) {
	b, err := Db.GetBucket(BucketKeys)
	if err != nil {
		return "", err
	}

	if WantJsonOutput {
		out += "{\"keys\":["
	}
	for i, v := range b.KeyValueList {
		label, err := FindLabelFromPubKey(v.Key)
		if err != nil {
			if WantJsonOutput {
				if i != 0 {
					out += ","
				}
				out += fmt.Sprintf("{\"error\":\"cannot find data for for key %x\"}", v.Key)
			} else {
				out += fmt.Sprintf("Error: Cannot find data for key %x\n", v.Key)
			}
		} else {
			str, err := ExportKey(label)
			if err != nil {
				out += fmt.Sprintf("invalid key for key name %s (error %v)\n", label, err)
			} else {
				if WantJsonOutput && i != 0 {
					out += ","
				}
				out += str
			}
		}
	}
	if WantJsonOutput {
		out += "]}"
		var b bytes.Buffer
		err := json.Indent(&b, []byte(out), "", "    ")
		if err == nil {
			out = b.String()
		}
	}
	return out, nil
}

func ExportSeed() (string, error) {
	seed, err := Db.Get(BucketMnemonic, []byte("seed"))
	if err != nil {
		return "", fmt.Errorf("mnemonic seed not found")
	}
	if WantJsonOutput {
		a := KeyResponse{}
		a.Seed = seed
		return PrintJson(&a)
	}

	return fmt.Sprintf(" seed: %x\n", seed), nil
}

func ExportMnemonic() (string, error) {
	phrase, err := Db.Get(BucketMnemonic, []byte("phrase"))
	if err != nil {
		return "", err
	}
	if WantJsonOutput {
		a := KeyResponse{}
		a.Mnemonic = types.String(phrase)
		return PrintJson(&a)
	}
	return fmt.Sprintf("mnemonic phrase: %s\n", string(phrase)), nil
}
