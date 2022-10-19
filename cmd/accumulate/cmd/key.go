package cmd

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/howeyc/gopass"
	"github.com/spf13/cobra"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	tmjson "github.com/tendermint/tendermint/libs/json"
	"github.com/tendermint/tendermint/privval"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/core/wallet/cmd/accumulate/db"
	"gitlab.com/accumulatenetwork/core/wallet/cmd/accumulate/walletd"
	"gitlab.com/accumulatenetwork/core/wallet/cmd/accumulate/walletd/api"
)

func init() {
	keyImportCmd.AddCommand(keyImportPrivateCmd)
	keyImportCmd.AddCommand(keyImportFactoidCmd)
	keyImportCmd.AddCommand(keyImportLiteCmd)
	keyExportCmd.AddCommand(keyExportPrivateCmd)
	keyExportCmd.AddCommand(keyExportMnemonicCmd)
	keyExportCmd.AddCommand(keyExportAllCmd)
	keyExportCmd.AddCommand(keyExportSeedCmd)

	keyCmd.AddCommand(keyImportCmd)
	keyCmd.AddCommand(keyExportCmd)
	keyCmd.AddCommand(keyGenerateCmd)
	keyCmd.AddCommand(keyListCmd)
	keyImportPrivateCmd.Flags().StringVar(&SigType, "sigtype", "ed25519", "Specify the signature type use rcd1 for RCD1 type ; ed25519 for accumulate ed25519 ; btc for Bitcoin ; btclegacy for Legacy Bitcoin  ; eth for Ethereum ")
	keyImportLiteCmd.Flags().StringVar(&SigType, "sigtype", "ed25519", "Specify the signature type use rcd1 for RCD1 type ; ed25519 for accumulate ED25519 ; btc for Bitcoin ; btclegacy for Legacy Bitcoin  ; eth for Ethereum ")
	keyGenerateCmd.Flags().StringVar(&SigType, "sigtype", "ed25519", "Specify the signature type use rcd1 for RCD1 type ; ed25519 for accumulate ED25519 ; btc for Bitcoin ; btclegacy for Legacy Bitcoin  ; eth for Ethereum ")
	keyImportPrivateCmd.Flags().BoolVarP(&flagKeyImport.Force, "force", "f", false, "If there is an existing external key, overwrite it")
}

var flagKeyImport = struct {
	Force bool
}{}

var keyImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import private key from hex or factoid secret address",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Usage:")
		PrintKey()
	},
}

var keyImportPrivateCmd = &cobra.Command{
	Use:   "private [key name/label]",
	Short: "Import private key in hex from terminal input",
	Args:  cobra.RangeArgs(1, 2),
	Run: runCmdFunc2(func(cmd *cobra.Command, args []string) (string, error) {
		if len(args) == 2 {
			return importFilePV(cmd, args[0], args[1])
		}

		var sigType protocol.SignatureType
		var found bool
		if SigType != "" {
			sigType, found = protocol.SignatureTypeByName(SigType)
			if !found {
				return "", fmt.Errorf("unknown signature type %s", SigType)
			}
		}
		return ImportKeyPrompt(cmd, args[0], sigType)
	}),
}

var keyImportFactoidCmd = &cobra.Command{
	Use:   "factoid",
	Short: "Import secret factoid key from terminal input",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := ImportFactoidKey(cmd)
		printOutput(cmd, out, err)
	},
}

var keyImportLiteCmd = &cobra.Command{
	Use:   "lite",
	Short: "Import private key in hex and label the key with a lite address",
	Args:  cobra.ExactArgs(0),
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
		if err == nil {
			out, err = ImportKeyPrompt(cmd, "", sigType)
		}
		printOutput(cmd, out, err)
	},
}

var keyListCmd = &cobra.Command{
	Use:   "list",
	Short: "list keys in the wallet",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := walletd.ListKeyPublic()
		printOutput(cmd, out, err)
	},
}

var keyExportAllCmd = &cobra.Command{
	Use:   "all",
	Short: "export wallet with private keys and accounts",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := ExportKeys()
		printOutput(cmd, out, err)
	},
}

var keyExportMnemonicCmd = &cobra.Command{
	Use:   "all",
	Short: "export mnemonic phrase",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := ExportMnemonic()
		printOutput(cmd, out, err)
	},
}

var keyExportSeedCmd = &cobra.Command{
	Use:   "seed",
	Short: "export key seed",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := ExportSeed()
		printOutput(cmd, out, err)
	},
}

var keyExportPrivateCmd = &cobra.Command{
	Use:   "private",
	Short: "export key private",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := ExportKey(args[0])
		printOutput(cmd, out, err)
	},
}

var keyExportCmd = &cobra.Command{
	Use:   "export",
	Short: "export wallet private data and accounts",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		PrintKeyExport()
	},
}

var keyGenerateCmd = &cobra.Command{
	Use:   "generate [key name/label]",
	Short: "generate key private and give it a name",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := GenerateKey(args[0])
		printOutput(cmd, out, err)
	},
}

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Create and manage Keys for ADI Key Books, and Pages",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Usage:")
		PrintKey()
	},
}

type KeyResponse struct {
	Label       types.String `json:"name,omitempty"`
	PrivateKey  types.Bytes  `json:"privateKey,omitempty"`
	PublicKey   types.Bytes  `json:"publicKey,omitempty"`
	KeyInfo     api.KeyInfo  `json:"keyInfo,omitempty"`
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

func resolvePrivateKey(s string) (*walletd.Key, error) {
	k, err := parseKey(s)
	if err != nil {
		return nil, err
	}

	if k.PrivateKey != nil {
		return k, nil
	}

	return walletd.LookupByPubKey(k.PublicKey)
}

func resolvePublicKey(s string) (*walletd.Key, error) {
	return parseKey(s)
}

func parseKey(s string) (*walletd.Key, error) {
	privKey, err := hex.DecodeString(s)
	if err == nil && len(privKey) == 64 {
		ret := new(walletd.Key)
		err = ret.InitializeFromSeed(privKey, protocol.SignatureTypeED25519, "external")
		if err != nil {
			return nil, err
		}
		return ret, nil
	}

	k, err := pubKeyFromString(s)
	if err == nil {
		return k, nil
	}

	k, err = walletd.LookupByLabel(s)
	if err == nil {
		return k, nil
	}

	b, err := os.ReadFile(s)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve signing key, invalid key specifier: %q is not a label, key, or file", s)
	}

	var pvkey privval.FilePVKey
	if tmjson.Unmarshal(b, &pvkey) == nil {
		if pvkey.PrivKey == nil {
			return nil, fmt.Errorf("invalid private key in %s", s)
		}
		// TODO Check the key type
		ret := new(walletd.Key)
		err = ret.InitializeFromSeed(pvkey.PrivKey.Bytes(), protocol.SignatureTypeED25519, "external")
		if err != nil {
			return nil, err
		}
		return ret, nil
	}

	return nil, fmt.Errorf("cannot resolve signing key, invalid key specifier: %q is in an unsupported format", s)
}

func pubKeyFromString(s string) (*walletd.Key, error) {
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

	ret := new(walletd.Key)
	ret.PublicKey = pubKey[:]
	ret.KeyInfo.Type = protocol.SignatureTypeED25519
	ret.KeyInfo.Derivation = "external"
	return ret, nil
}

func ImportKeyPrompt(cmd *cobra.Command, label string, signatureType protocol.SignatureType) (out string, err error) {
	token, err := getPasswdPrompt(cmd, "Private Key : ", true)
	if err != nil {
		return "", db.ErrInvalidPassword
	}
	tokenBytes, err := hex.DecodeString(token)
	if err != nil {
		return "", err
	}
	return ImportKey(tokenBytes, label, signatureType)
}

func importFilePV(cmd *cobra.Command, label, filepath string) (out string, err error) {
	b, err := os.ReadFile(filepath)
	if err != nil {
		return "", err
	}
	key := new(privval.FilePVKey)
	err = tmjson.Unmarshal(b, key)
	if err != nil {
		return "", fmt.Errorf("error reading PrivValidator key from %v: %w", filepath, err)
	}

	switch key.PrivKey.Type() {
	case tmed25519.KeyType:
		return ImportKey(key.PrivKey.Bytes(), label, protocol.SignatureTypeED25519)
	default:
		return "", fmt.Errorf("unsupported key type %v", key.PrivKey.Type())
	}
}

func getPasswdPrompt(cmd *cobra.Command, prompt string, mask bool) (string, error) {
	rd, ok := cmd.InOrStdin().(gopass.FdReader)
	if ok {
		b, err := gopass.GetPasswdPrompt(prompt, mask, rd, cmd.ErrOrStderr())
		return string(b), err
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
	pk := new(walletd.Key)

	if err := pk.InitializeFromSeed(token, signatureType, "external"); err != nil {
		return "", err
	}

	lt, err := protocol.LiteTokenAddress(pk.PublicKey, protocol.ACME, pk.KeyInfo.Type)
	if err != nil {
		return "", fmt.Errorf("no label specified and cannot import as lite token account")
	}
	liteLabel, _ = walletd.LabelForLiteTokenAccount(lt.String())

	if label == "" {
		label = liteLabel
	}

	//here will change the label if it is a lite account specified, otherwise just use the label
	label, _ = walletd.LabelForLiteTokenAccount(label)

	existing, err := walletd.LookupByLabel(label)
	if err == nil {
		if !flagKeyImport.Force || existing.KeyInfo.Derivation != "external" {
			return "", fmt.Errorf("key name is already being used")
		}
	}

	_, err = walletd.LookupByPubKey(pk.PublicKey)
	lab := "not found"
	if err == nil {
		b, _ := walletd.GetWallet().GetBucket(walletd.BucketLabel)
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
		a.PublicKey = pk.PublicKey
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
	k, err := walletd.LookupByLabel(label)
	if err != nil {
		k, err := pubKeyFromString(label)
		if err != nil {
			return "", fmt.Errorf("no private key found for key name %s", label)
		}
		_, err = walletd.LookupByPubKey(k.PublicKey)
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

	key, err := walletd.GenerateKey(sigtype)
	if err != nil {
		return "", err
	}

	//assign a label if needed
	if label == "" {
		label, err = key.NativeAddress()
		if err != nil {
			return "", err
		}
	}

	//derive a lite label if needed that will reference the Accumulate lite account
	keyHash := key.PublicKeyHash()
	lt, err := protocol.LiteTokenAddressFromHash(keyHash, protocol.ACME)
	if err != nil {
		return "", fmt.Errorf("no label specified and cannot import as lite token account")
	}
	liteLabel, _ := walletd.LabelForLiteTokenAccount(lt.String())

	//here will change the label if it is a lite account specified, otherwise just use the label
	label, _ = walletd.LabelForLiteTokenAccount(label)

	//make sure it doesn't exist
	_, err = walletd.LookupByLabel(label)
	if err == nil {
		return "", fmt.Errorf("key already exists for key name %s", label)
	}

	err = key.Save(label, liteLabel)
	if err != nil {
		return "", err
	}

	if WantJsonOutput {
		a := KeyResponse{}
		a.Label = types.String(label)
		a.PublicKey = key.PublicKey
		a.LiteAccount = lt
		a.KeyInfo = api.KeyInfo{Type: sigtype}
		dump, err := json.Marshal(&a)
		if err != nil {
			return "", err
		}
		out += fmt.Sprintf("%s\n", string(dump))
	} else {
		out += fmt.Sprintf("\tname\t\t:\t%s\n\tlite account\t:\t%s\n\tpublic key\t:\t%x\n\tkey type\t:\t%s\n", label, lt, key.PublicKey, sigtype)
	}
	return out, nil
}

func ExportKeys() (out string, err error) {
	b, err := walletd.GetWallet().GetBucket(walletd.BucketKeys)
	if err != nil {
		return "", err
	}

	if WantJsonOutput {
		out += "{\"keys\":["
	}
	for i, v := range b.KeyValueList {
		label, err := walletd.FindLabelFromPubKey(v.Key)
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
	seed, err := walletd.GetWallet().Get(walletd.BucketMnemonic, []byte("seed"))
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
	phrase, err := walletd.GetWallet().Get(walletd.BucketMnemonic, []byte("phrase"))
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
	token, err := getPasswdPrompt(cmd, "Private Key : ", true)
	if err != nil {
		return "", db.ErrInvalidPassword
	}
	if !strings.Contains(token, "Fs") {
		return "", fmt.Errorf("key to import is not a factoid address")
	}
	label, _, privatekey, err := protocol.GetFactoidAddressRcdHashPkeyFromPrivateFs(token)
	if err != nil {
		return "", err
	}
	return ImportKey(privatekey, label, protocol.SignatureTypeRCD1)
}
