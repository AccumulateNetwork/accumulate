package cmd

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/howeyc/gopass"
	"github.com/spf13/cobra"
	tmjson "github.com/tendermint/tendermint/libs/json"
	"github.com/tendermint/tendermint/privval"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/db"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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
		var sigType protocol.SignatureType
		var found bool
		if SigType != "" {
			sigType, found = protocol.SignatureTypeByName(SigType)
			if !found {
				err = fmt.Errorf("unknown signature type %s", SigType)
			}
		}

		if len(args) > 0 && err == nil {
			switch arg := args[0]; arg {
			case "import":
				if len(args) == 3 {
					switch args[1] {
					// case "mnemonic":
					// 	out, err = ImportMnemonic(args[2:])
					case "private":
						// log.Panicln(args)
						out, err = ImportKeyPrompt(cmd, args[2], sigType)
					case "public":
						//reserved for future use.
						fallthrough
					case "lite":
						out, err = ImportKeyPrompt(cmd, "", sigType)
					case "factoid":
						out, err = ImportFactoidKey(cmd)
					default:
						PrintKeyImport()
					}
				} else if len(args) > 3 {
					switch args[1] {
					case "mnemonic":
						out, err = ImportMnemonic(args[2:])
					case "private":
						out, err = ImportKeyPrompt(cmd, args[2], sigType)
					case "public":
						//reserved for future use.
						fallthrough
					default:
						PrintKeyImport()
					}
				} else if len(args) == 2 {
					if args[1] == "factoid" {
						out, err = ImportFactoidKey(cmd)
					} else {
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
	Use:   "update [key page url] [key name[@key book or page]] [new key name]",
	Short: "Self-update a key",
	Args:  cobra.RangeArgs(3, 5),
	Run:   runCmdFunc(UpdateKey),
}

type KeyResponse struct {
	Label       types.String `json:"name,omitempty"`
	PrivateKey  types.Bytes  `json:"privateKey,omitempty"`
	PublicKey   types.Bytes  `json:"publicKey,omitempty"`
	KeyInfo     KeyInfo      `json:"keyInfo,omitempty"`
	LiteAccount *url.URL     `json:"liteAccount,omitempty"`
	Seed        types.Bytes  `json:"seed,omitempty"`
	Mnemonic    types.String `json:"mnemonic,omitempty"`
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
	fmt.Println("  accumulate key import private [key name]      Import a key and give it a name in the wallet, prompt for key")
	fmt.Println("  accumulate key import factoid   Import a factoid private address, prompt for key")

	fmt.Println("  accumulate key import lite        Import a key as a lite address, prompt for key")
}

func PrintKey() {
	PrintKeyGenerate()
	PrintKeyPublic()
	PrintKeyImport()

	PrintKeyExport()
}

func resolvePrivateKey(s string) (*Key, error) {
	k, err := parseKey(s)
	if err != nil {
		return nil, err
	}

	if k.PrivateKey != nil {
		return k, nil
	}

	return LookupByPubKey(k.PublicKey)
}

func resolvePublicKey(s string) (*Key, error) {
	return parseKey(s)
}

func parseKey(s string) (*Key, error) {
	privKey, err := hex.DecodeString(s)
	if err == nil && len(privKey) == 64 {
		return &Key{PrivateKey: privKey, PublicKey: privKey[32:], KeyInfo: KeyInfo{Type: protocol.SignatureTypeED25519}}, nil
	}

	k, err := pubKeyFromString(s)
	if err == nil {
		return k, nil
	}

	k, err = LookupByLabel(s)
	if err == nil {
		return k, nil
	}

	b, err := ioutil.ReadFile(s)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve signing key, invalid key specifier: %q is not a label, key, or file", s)
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
		// TODO Check the key type
		return &Key{PrivateKey: priv, PublicKey: pub, KeyInfo: KeyInfo{Type: protocol.SignatureTypeED25519}}, nil
	}

	return nil, fmt.Errorf("cannot resolve signing key, invalid key specifier: %q is in an unsupported format", s)
}

func pubKeyFromString(s string) (*Key, error) {
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

	return &Key{PublicKey: pubKey[:], KeyInfo: KeyInfo{Type: protocol.SignatureTypeED25519}}, nil
}

func LookupByLiteTokenUrl(lite string) (*Key, error) {
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

func LookupByLiteIdentityUrl(lite string) (*Key, error) {
	liteKey, isLite := LabelForLiteIdentity(lite)
	if !isLite {
		return nil, fmt.Errorf("invalid lite identity %s", liteKey)
	}

	label, err := GetWallet().Get(BucketLite, []byte(liteKey))
	if err != nil {
		return nil, fmt.Errorf("lite identity account not found %s", lite)
	}

	return LookupByLabel(string(label))
}

func LookupByLabel(label string) (*Key, error) {
	k := new(Key)
	return k, k.LoadByLabel(label)
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

// LabelForLiteIdentity returns the label of the LiteIdentity if label
// is a valid LiteIdentity account URL. Otherwise, LabelForLiteIdentity returns the
// original value.
func LabelForLiteIdentity(label string) (string, bool) {
	u, err := url.Parse(label)
	if err != nil {
		return label, false
	}

	key, err := protocol.ParseLiteIdentity(u)
	if key == nil || err != nil {
		return label, false
	}

	return u.Hostname(), true
}

func LookupByPubKey(pubKey []byte) (*Key, error) {
	k := new(Key)
	return k, k.LoadByPublicKey(pubKey)
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

	k := new(Key)
	k.PrivateKey = privKey
	k.PublicKey = pubKey
	k.KeyInfo.Type = sigtype
	err = k.Save(label, liteLabel)
	if err != nil {
		return "", err
	}

	if WantJsonOutput {
		a := KeyResponse{}
		a.Label = types.String(label)
		a.PublicKey = pubKey
		a.LiteAccount = lt
		a.KeyInfo = KeyInfo{Type: sigtype}
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

func ImportKeyPrompt(cmd *cobra.Command, label string, signatureType protocol.SignatureType) (out string, err error) {
	token, err := getPasswdPrompt(cmd, "Private Key : ", false)
	if err != nil {
		return "", db.ErrInvalidPassword
	}
	tokenBytes, err := hex.DecodeString(token)
	if err != nil {
		return "", err
	}
	return ImportKey(tokenBytes, label, signatureType)
}

func getPasswdPrompt(cmd *cobra.Command, prompt string, mask bool) (string, error) {
	rd, ok := cmd.InOrStdin().(gopass.FdReader)
	if ok {
		bytes, err := gopass.GetPasswdPrompt(prompt, mask, rd, cmd.ErrOrStderr())
		return string(bytes), err
	}

	_, err := fmt.Fprint(cmd.OutOrStdout(), prompt)
	if err != nil {
		return "", err
	}
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(line, "\n"), nil
}

// ImportKey will import the private key and assign it to the label
func ImportKey(token []byte, label string, signatureType protocol.SignatureType) (out string, err error) {

	var liteLabel string
	pk := new(Key)

	if err := pk.Initialize(token, signatureType); err != nil {
		return "", err
	}
	pk.KeyInfo.Derivation = "external"

	lt, err := protocol.LiteTokenAddress(pk.PublicKey, protocol.ACME, pk.KeyInfo.Type)
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

	_, err = LookupByPubKey(pk.PublicKey)
	lab := "not found"
	if err == nil {
		b, _ := GetWallet().GetBucket(BucketLabel)
		if b != nil {
			for _, v := range b.KeyValueList {
				if bytes.Equal(v.Value, pk.PublicKey) {
					lab = string(v.Key)
					break
				}
			}
			return "", fmt.Errorf("private key already exists in wallet by key name of %s", lab)
		}
	}

	err = pk.Save(label, liteLabel)
	if err != nil {
		return "", err
	}

	if WantJsonOutput {
		a := KeyResponse{}
		a.Label = types.String(label)
		a.PublicKey = types.Bytes(pk.PublicKey)
		a.LiteAccount = lt
		a.KeyInfo = pk.KeyInfo
		dump, err := json.Marshal(&a)
		if err != nil {
			return "", err
		}
		out = fmt.Sprintf("%s\n", string(dump))
	} else {
		out = fmt.Sprintf("\tname\t\t:\t%s\n\tlite account\t:\t%s\n\tpublic key\t:\t%x\n\tkey type\t:\t%s\n\tderivation\t:\t%s\n", label, lt, pk.PublicKey, pk.KeyInfo.Type, pk.KeyInfo.Derivation)
	}
	return out, nil
}

func ExportKey(label string) (string, error) {
	k, err := LookupByLabel(label)
	if err != nil {
		k, err := pubKeyFromString(label)
		if err != nil {
			return "", fmt.Errorf("no private key found for key name %s", label)
		}
		_, err = LookupByPubKey(k.PublicKey)
		if err != nil {
			return "", fmt.Errorf("no private key found for key name %s", label)
		}
	}

	if WantJsonOutput {
		a := KeyResponse{}
		a.Label = types.String(label)
		a.PrivateKey = k.PrivateKey
		a.PublicKey = k.PublicKey
		a.KeyInfo = k.KeyInfo
		dump, err := json.Marshal(&a)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s\n", string(dump)), nil
	} else {
		return fmt.Sprintf("name\t\t\t:\t%s\n\tprivate key\t:\t%x\n\tpublic key\t:\t%x\nkey type\t\t:\t%s\n\tderivation\t:\t%s\n", label, k.PrivateKey, k.PublicKey, k.KeyInfo.Type, k.KeyInfo.Derivation), nil
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

func ImportFactoidKey(cmd *cobra.Command) (out string, err error) {
	token, err := getPasswdPrompt(cmd, "Private Key : ", false)
	if err != nil {
		return "", db.ErrInvalidPassword
	}
	if !strings.Contains(token, "Fs") {
		return "", fmt.Errorf("key to import is not a factoid address")
	}
	label, _, privatekey, err := protocol.GetFactoidAddressRcdHashPkeyFromPrivateFs(string(token))
	if err != nil {
		return "", err
	}
	return ImportKey(privatekey, label, protocol.SignatureTypeRCD1)
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

	k, err := resolvePublicKey(args[0])
	if err != nil {
		return "", err
	}

	txn := new(protocol.UpdateKey)
	txn.NewKeyHash = k.PublicKeyHash()
	return dispatchTxAndPrintResponse(txn, principal, signer)
}
