package cmd

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	tmjson "github.com/tendermint/tendermint/libs/json"
	"github.com/tendermint/tendermint/privval"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/db"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

func init() {
	keyCmd.AddCommand(keyUpdateCmd)
	keyCmd.Flags().StringVar(&SigType, "sigtype", "ed25519", "Specify the signature type use rcd1 for RCD1 type ; ed25519 for ED25519 ; legacyed25519 for LegacyED25519 ; btc for Bitcoin ; btclegacy for Legacy Bitcoin  ; eth for Ethereum ")
}

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Create and manage Keys for ADI Key Books, and Pages",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error

		//set the default signature type
		sigType := protocol.SignatureTypeED25519
		if SigType != "" {
			sigType, err = ValidateSigType(SigType)
		}

		if len(args) > 0 && err == nil {
			switch arg := args[0]; arg {
			case "import":
				if len(args) == 3 {
					if args[1] == "lite" {

						out, err = ImportKey(args[2], "", sigType)
					} else if args[1] == "factoid" {
						out, err = ImportFactoidKey(args[2])
					} else {
						PrintKeyImport()
					}
				} else if len(args) > 3 {
					switch args[1] {
					case "mnemonic":
						out, err = ImportMnemonic(args[2:])
					case "private":
						out, err = ImportKey(args[2], args[3], sigType)
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

var keyUpdateCmd = &cobra.Command{
	Use:   "update [key page url] [original key name] [key index (optional)] [key height (optional)] [new key name]",
	Short: "Self-update a key",
	Args:  cobra.RangeArgs(3, 5),
	Run:   runCmdFunc(UpdateKey),
}

type KeyResponse struct {
	Label       types.String           `json:"name,omitempty"`
	PrivateKey  types.Bytes            `json:"privateKey,omitempty"`
	PublicKey   types.Bytes            `json:"publicKey,omitempty"`
	KeyType     protocol.SignatureType `json:"keyType,omitempty"`
	LiteAccount *url.URL               `json:"liteAccount,omitempty"`
	Seed        types.Bytes            `json:"seed,omitempty"`
	Mnemonic    types.String           `json:"mnemonic,omitempty"`
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
	fmt.Println("  accumulate key import factoid [factoid private address]  Import a factoid private address")

	fmt.Println("  accumulate key import lite [private key hex]       Import a key as a lite address")
}

func PrintKey() {
	PrintKeyGenerate()
	PrintKeyPublic()
	PrintKeyImport()

	PrintKeyExport()
}

func resolveKeyTypeAndHash(pubKey []byte) (protocol.SignatureType, []byte, error) {
	sig, err := GetWallet().Get(BucketSigType, pubKey)
	switch {
	case err == nil:
		// Ok

	case errors.Is(err, db.ErrNotFound),
		errors.Is(err, db.ErrNoBucket):
		// Default to legacy ED25519
		hash := sha256.Sum256(pubKey)
		return protocol.SignatureTypeLegacyED25519, hash[:], nil

	default:
		return 0, nil, err
	}

	t, _ := common.BytesUint64(sig)
	var sigType protocol.SignatureType
	if !sigType.SetEnumValue(t) {
		return 0, nil, fmt.Errorf("unknown signature type %d", t)
	}

	switch sigType {
	case protocol.SignatureTypeLegacyED25519,
		protocol.SignatureTypeED25519:
		hash := sha256.Sum256(pubKey)
		return sigType, hash[:], nil

	case protocol.SignatureTypeRCD1:
		hash := protocol.GetRCDHashFromPublicKey(pubKey, 1)
		return sigType, hash, nil
	case protocol.SignatureTypeBTC, protocol.SignatureTypeBTCLegacy:
		hash := protocol.BTCHash(pubKey)
		return sigType, hash, nil
	case protocol.SignatureTypeETH:
		hash := protocol.ETHhash(pubKey)
		return sigType, hash, nil
	default:
		return 0, nil, fmt.Errorf("unsupported signature type %v", sigType)
	}
}

func resolvePrivateKey(s string) ([]byte, error) {
	pub, priv, err := parseKey(s)
	if err != nil {
		return nil, err
	}

	if priv != nil {
		return priv, nil
	}

	return LookupByPubKey(pub)
}

func resolvePublicKey(s string) (pubKey, keyHash []byte, sigType protocol.SignatureType, err error) {
	pub, _, err := parseKey(s)
	if err != nil {
		return nil, nil, 0, err
	}

	sigType, keyHash, err = resolveKeyTypeAndHash(pub)
	if err != nil {
		return nil, nil, 0, err
	}

	return pub, keyHash, sigType, nil
}

func parseKey(s string) (pubKey, privKey []byte, err error) {
	pubKey, err = pubKeyFromString(s)
	if err == nil {
		return pubKey, nil, nil
	}

	privKey, err = LookupByLabel(s)
	if err == nil {
		// Assume ED25519
		return privKey[32:], privKey, nil
	}

	b, err := ioutil.ReadFile(s)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot resolve signing key, invalid key specifier: %q is not a label, key, or file", s)
	}

	var pvkey privval.FilePVKey
	if tmjson.Unmarshal(b, &pvkey) == nil {
		var pub, priv []byte
		if pvkey.PubKey != nil {
			pub = pvkey.PubKey.Bytes()
		}
		if pvkey.PrivKey != nil {
			priv = pvkey.PrivKey.Bytes()
		}
		return pub, priv, nil
	}

	return nil, nil, fmt.Errorf("cannot resolve signing key, invalid key specifier: %q is in an unsupported format", s)
}

func pubKeyFromString(s string) ([]byte, error) {
	var pubKey types.Bytes32
	if len(s) != 64 {
		return nil, fmt.Errorf("invalid public key or wallet key name")
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

func LookupByLite(lite string) ([]byte, error) {
	liteKey, isLite := LabelForLiteTokenAccount(lite)
	if !isLite {
		return nil, fmt.Errorf("invalid lite account %s", liteKey)
	}

	label, err := GetWallet().Get(BucketLite, []byte(liteKey))
	if err != nil {
		return nil, fmt.Errorf("lite account not found %s", lite)
	}

	return LookupByLabel(string(label))
}

func LookupByLabel(label string) ([]byte, error) {
	label, _ = LabelForLiteTokenAccount(label)

	pubKey, err := GetWallet().Get(BucketLabel, []byte(label))
	if err != nil {
		return nil, fmt.Errorf("valid key not found for %s", label)
	}
	return LookupByPubKey(pubKey)
}

// LabelForLiteTokenAccount returns the identity of the token account if label
// is a valid token account URL. Otherwise, LabelForLiteTokenAccount returns the
// original value.
func LabelForLiteTokenAccount(label string) (string, bool) {
	u, err := url.Parse(label)
	if err != nil {
		return label, false
	}

	key, _, err := protocol.ParseLiteTokenAddress(u)
	if key == nil || err != nil {
		return label, false
	}

	return u.Hostname(), true
}

func LookupByPubKey(pubKey []byte) ([]byte, error) {
	return GetWallet().Get(BucketKeys, pubKey)
}

func GenerateKey(label string) (string, error) {
	var out string
	if _, err := strconv.ParseInt(label, 10, 64); err == nil {
		return "", fmt.Errorf("key name cannot be a number")
	}

	if label != "" {
		_, _, _, errFs := protocol.GetFactoidAddressRcdHashPkeyFromPrivateFs(label)
		_, errFA := protocol.GetRCDFromFactoidAddress(label)
		if errFs == nil || errFA == nil {
			return "", fmt.Errorf("key name cannot be a factoid address")
		}
		u, err := url.Parse(label)
		if err != nil {
			return "", err
		}
		_, _, err = protocol.ParseLiteTokenAddress(u)
		if err != nil {
			return "", fmt.Errorf("key name cannot look like an account")
		}
	}

	sigtype, err := ValidateSigType(SigType)
	if err != nil {
		return "", err
	}

	var privKey []byte
	var pubKey []byte

	if sigtype == protocol.SignatureTypeBTCLegacy || sigtype == protocol.SignatureTypeETH {
		privKey, pubKey = protocol.SECP256K1UncompressedKeypair()
	} else if sigtype == protocol.SignatureTypeBTC {
		privKey, pubKey = protocol.SECP256K1Keypair()
	} else {

		privKey, err = GeneratePrivateKey()
		if err != nil {
			return "", err
		}

		pubKey = privKey[32:]
	}

	var keyHash []byte
	if sigtype == protocol.SignatureTypeRCD1 {
		keyHash = protocol.GetRCDHashFromPublicKey(pubKey, 1)
		if label == "" {
			label, err = protocol.GetFactoidAddressFromRCDHash(keyHash)
			if err != nil {
				return "", err
			}
		}
	} else if sigtype == protocol.SignatureTypeBTC || sigtype == protocol.SignatureTypeBTCLegacy {
		keyHash = protocol.BTCHash(pubKey)
		if label == "" {
			label = protocol.BTCaddress(pubKey)
		}
	} else if sigtype == protocol.SignatureTypeETH {
		keyHash = protocol.ETHhash(pubKey)
		if label == "" {
			label = protocol.ETHaddress(pubKey)
		}
	} else {
		h := sha256.Sum256(pubKey)
		keyHash = h[:]
	}

	lt, err := protocol.LiteTokenAddressFromHash(keyHash, protocol.ACME)
	if err != nil {
		return "", fmt.Errorf("no label specified and cannot import as lite token account")
	}
	liteLabel, _ := LabelForLiteTokenAccount(lt.String())

	if label == "" {
		label = liteLabel
	}

	//here will change the label if it is a lite account specified, otherwise just use the label
	label, _ = LabelForLiteTokenAccount(label)

	_, err = LookupByLabel(label)
	if err == nil {
		return "", fmt.Errorf("key already exists for key name %s", label)
	}

	err = GetWallet().Put(BucketKeys, pubKey, privKey)
	if err != nil {
		return "", err
	}

	err = GetWallet().Put(BucketLabel, []byte(label), pubKey)
	if err != nil {
		return "", err
	}

	err = GetWallet().Put(BucketLite, []byte(liteLabel), []byte(label))
	if err != nil {
		return "", err
	}

	err = GetWallet().Put(BucketSigType, privKey[32:], common.Uint64Bytes(sigtype.GetEnumValue()))
	if err != nil {
		return "", err
	}

	if WantJsonOutput {
		a := KeyResponse{}
		a.Label = types.String(label)
		a.PublicKey = pubKey
		a.LiteAccount = lt
		a.KeyType = sigtype
		dump, err := json.Marshal(&a)
		if err != nil {
			return "", err
		}
		out += fmt.Sprintf("%s\n", string(dump))
	} else {
		out += fmt.Sprintf("\tname\t\t:\t%s\n\tlite account\t:\t%s\n\tpublic key\t:\t%x\n\tkey type\t:\t%s\n", label, lt, pubKey, sigtype)
	}
	return out, nil
}

func ListKeyPublic() (out string, err error) {
	out = "Public Key\t\t\t\t\t\t\t\tKey name\n"
	b, err := GetWallet().GetBucket(BucketLabel)
	if err != nil {
		return "", err
	}

	for _, v := range b.KeyValueList {
		out += fmt.Sprintf("%x\t%s\n", v.Value, v.Key)
	}
	return out, nil
}

func FindLabelFromPublicKeyHash(pubKeyHash []byte) (lab string, err error) {
	b, err := GetWallet().GetBucket(BucketLabel)
	if err != nil {
		return lab, err
	}

	for _, v := range b.KeyValueList {
		keyHash := sha256.Sum256(v.Value)
		if bytes.Equal(keyHash[:], pubKeyHash) {
			lab = string(v.Key)
			break
		}
	}

	if lab == "" {
		err = fmt.Errorf("key name not found for key hash %x", pubKeyHash)
	}
	return lab, err
}

func FindLabelFromPubKey(pubKey []byte) (lab string, err error) {
	b, err := GetWallet().GetBucket(BucketLabel)
	if err != nil {
		return lab, err
	}

	for _, v := range b.KeyValueList {
		if bytes.Equal(v.Value, pubKey) {
			lab = string(v.Key)
			break
		}
	}

	if lab == "" {
		err = fmt.Errorf("key name not found for %x", pubKey)
	}
	return lab, err
}

// ImportKey will import the private key and assign it to the label
func ImportKey(pkhex string, label string, signatureType protocol.SignatureType) (out string, err error) {

	var liteLabel string
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

	lt, err := protocol.LiteTokenAddress(pk[32:], protocol.ACME, signatureType)
	if err != nil {
		return "", fmt.Errorf("no label specified and cannot import as lite token account")
	}
	liteLabel, _ = LabelForLiteTokenAccount(lt.String())

	if label == "" {
		label = liteLabel
	}

	//here will change the label if it is a lite account specified, otherwise just use the label
	label, _ = LabelForLiteTokenAccount(label)

	_, err = LookupByLabel(label)
	if err == nil {
		return "", fmt.Errorf("key name is already being used")
	}

	_, err = LookupByPubKey(pk[32:])
	lab := "not found"
	if err == nil {
		b, _ := GetWallet().GetBucket(BucketLabel)
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

	publicKey := pk[32:]
	err = GetWallet().Put(BucketKeys, publicKey, pk)
	if err != nil {
		return "", err
	}

	err = GetWallet().Put(BucketLabel, []byte(label), pk[32:])
	if err != nil {
		return "", err
	}

	err = GetWallet().Put(BucketLite, []byte(liteLabel), []byte(label))
	if err != nil {
		return "", err
	}

	err = GetWallet().Put(BucketSigType, publicKey, common.Uint64Bytes(signatureType.GetEnumValue()))
	if err != nil {
		return "", err
	}

	if WantJsonOutput {
		a := KeyResponse{}
		a.Label = types.String(label)
		a.PublicKey = types.Bytes(pk[32:])
		a.LiteAccount = lt
		a.KeyType = signatureType
		dump, err := json.Marshal(&a)
		if err != nil {
			return "", err
		}
		out = fmt.Sprintf("%s\n", string(dump))
	} else {
		out = fmt.Sprintf("\tname\t\t:\t%s\n\tlite account\t:\t%s\n\tpublic key\t:\t%x\n\tkey type\t:\t%s\n", label, lt, pk[32:], signatureType)
	}
	return out, nil
}

func ExportKey(label string) (string, error) {
	pk, err := LookupByLabel(label)
	var sigType protocol.SignatureType
	if err != nil {
		pubk, err := pubKeyFromString(label)
		if err != nil {
			return "", fmt.Errorf("no private key found for key name %s", label)
		}
		pk, err = LookupByPubKey(pubk)
		if err != nil {
			return "", fmt.Errorf("no private key found for key name %s", label)
		}
		_, err = FindLabelFromPubKey(pubk)
		if err != nil {
			return "", fmt.Errorf("no private key found for key name %s", label)
		}
		sigType, _, err = resolveKeyTypeAndHash(pubk)
		if err != nil {
			return "", fmt.Errorf("no key type found")
		}
	}

	if WantJsonOutput {
		a := KeyResponse{}
		a.Label = types.String(label)
		a.PrivateKey = pk[:32]
		a.PublicKey = pk[32:]
		a.KeyType = sigType
		dump, err := json.Marshal(&a)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s\n", string(dump)), nil
	} else {
		return fmt.Sprintf("name\t\t\t:\t%s\n\tprivate key\t:\t%x\n\tpublic key\t:\t%x\nkey type\t\t:\t%s\n", label, pk[:32], pk[32:], sigType), nil
	}
}

func GeneratePrivateKey() ([]byte, error) {
	seed, err := lookupSeed()
	if err != nil {
		//if private key seed doesn't exist, just create a key
		_, privKey, err := ed25519.GenerateKey(nil)
		if err != nil {
			return nil, err
		}
		return privKey, nil
	}

	//if we do have a seed, then create a new key
	masterKey, _ := bip32.NewMasterKey(seed)

	ct, err := getKeyCountAndIncrement()
	if err != nil {
		return nil, err
	}

	newKey, err := masterKey.NewChildKey(ct)
	if err != nil {
		return nil, err
	}
	privKey := ed25519.NewKeyFromSeed(newKey.Key)
	return privKey, nil
}

func getKeyCountAndIncrement() (count uint32, err error) {

	ct, _ := GetWallet().Get(BucketMnemonic, []byte("count"))
	if ct != nil {
		count = binary.LittleEndian.Uint32(ct)
	}

	ct = make([]byte, 8)
	binary.LittleEndian.PutUint32(ct, count+1)
	err = GetWallet().Put(BucketMnemonic, []byte("count"), ct)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func lookupSeed() (seed []byte, err error) {
	seed, err = GetWallet().Get(BucketMnemonic, []byte("seed"))
	if err != nil {
		return nil, fmt.Errorf("mnemonic seed doesn't exist")
	}

	return seed, nil
}

func ImportMnemonic(mnemonic []string) (string, error) {
	mns := strings.Join(mnemonic, " ")

	if !bip39.IsMnemonicValid(mns) {
		return "", fmt.Errorf("invalid mnemonic provided")
	}

	// Generate a Bip32 HD wallet for the mnemonic and a user supplied password
	seed := bip39.NewSeed(mns, "")

	root, _ := GetWallet().Get(BucketMnemonic, []byte("seed"))
	if len(root) != 0 {
		return "", fmt.Errorf("mnemonic seed phrase already exists within wallet")
	}

	err := GetWallet().Put(BucketMnemonic, []byte("seed"), seed)
	if err != nil {
		return "", fmt.Errorf("DB: seed write error, %v", err)
	}

	err = GetWallet().Put(BucketMnemonic, []byte("phrase"), []byte(mns))
	if err != nil {
		return "", fmt.Errorf("DB: phrase write error %s", err)
	}

	return "mnemonic import successful", nil
}

func ExportKeys() (out string, err error) {
	b, err := GetWallet().GetBucket(BucketKeys)
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
	seed, err := GetWallet().Get(BucketMnemonic, []byte("seed"))
	if err != nil {
		return "", fmt.Errorf("mnemonic seed not found")
	}
	if WantJsonOutput {
		a := KeyResponse{}
		a.Seed = seed
		dump, err := json.Marshal(&a)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s\n", string(dump)), nil
	} else {
		return fmt.Sprintf(" seed: %x\n", seed), nil
	}
}

func ExportMnemonic() (string, error) {
	phrase, err := GetWallet().Get(BucketMnemonic, []byte("phrase"))
	if err != nil {
		return "", err
	}
	if WantJsonOutput {
		a := KeyResponse{}
		a.Mnemonic = types.String(phrase)
		dump, err := json.Marshal(&a)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s\n", string(dump)), nil
	} else {
		return fmt.Sprintf("mnemonic phrase: %s\n", string(phrase)), nil
	}
}

func ImportFactoidKey(factoidkey string) (out string, err error) {
	if !strings.Contains(factoidkey, "Fs") {
		return "", fmt.Errorf("key to import is not a factoid address")
	}
	label, _, privatekey, err := protocol.GetFactoidAddressRcdHashPkeyFromPrivateFs(factoidkey)
	if err != nil {
		return "", err
	}
	return ImportKey(hex.EncodeToString(privatekey), label, protocol.SignatureTypeRCD1)
}

func UpdateKey(args []string) (string, error) {
	principal, err := url.Parse(args[0])
	if err != nil {
		return "", err
	}

	args, signer, err := prepareSigner(principal, args[1:])
	if err != nil {
		return "", err
	}

	newPubKey, _, _, err := resolvePublicKey(args[0])
	if err != nil {
		return "", err
	}

	newPubKeyHash := sha256.Sum256(newPubKey)
	txn := new(protocol.UpdateKey)
	txn.NewKeyHash = newPubKeyHash[:]
	return dispatchTxAndPrintResponse("update-key", txn, nil, principal, signer)
}
