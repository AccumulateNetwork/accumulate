package cmd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/mdp/qrterminal"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/db"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

func init() {
	accountCmd.AddCommand(
		accountGetCmd,
		accountCreateCmd,
		accountQrCmd,
		accountGenerateCmd,
		accountListCmd,
		accountRestoreCmd)

	accountCreateCmd.AddCommand(
		accountCreateTokenCmd,
		accountCreateDataCmd)

	accountCreateDataCmd.AddCommand(
		accountCreateDataLiteCmd)

	accountCreateDataCmd.Flags().BoolVar(&flagAccount.Lite, "lite", false, "Create a lite data account")
	accountGenerateCmd.Flags().StringVar(&SigType, "sigtype", "ed25519", "Specify the signature type use rcd1 for RCD1 type ; ed25519 for ED25519 ; legacyed25519 for LegacyED25519 ; btc for Bitcoin ; btclegacy for LegacyBitcoin  ; eth for Ethereum ")
	accountCreateDataCmd.Flags().StringVar(&flagAccount.LiteData, "lite-data", "", "Add first entry data to lite data account")

}

var flagAccount = struct {
	Lite     bool
	LiteData string
}{}

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Create and get token accounts",
}

var accountGetCmd = &cobra.Command{
	Use:   "get [url]",
	Short: "Get an account by URL",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := GetTokenAccount(args[0])
		printOutput(cmd, out, err)
	},
}

var accountCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an account",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Deprecation Warning!\nTo create a token account, in future please specify either \"token\" or \"data\"\n\n")
		//this will be removed in future release and replaced with usage: PrintAccountCreate()
		out, err := CreateAccount(cmd, args[0], args[1:])
		printOutput(cmd, out, err)
	},
}

var accountCreateTokenCmd = &cobra.Command{
	Use:   "token [actor adi] [signing key name] [key index (optional)] [key height (optional)] [new token account url] [tokenUrl] --authority keyBook (optional)",
	Short: "Create an ADI token account",
	Args:  cobra.MinimumNArgs(4),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := CreateAccount(cmd, args[0], args[1:])
		printOutput(cmd, out, err)
	},
}

var accountCreateDataCmd = &cobra.Command{
	Use:   "data",
	Short: "Create a data account",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if flagAccount.Lite {
			if len(args) < 2 {
				PrintDataLiteAccountCreate()
				return
			}
			out, err = CreateLiteDataAccount(args[0], args[1:])
		} else {
			if len(args) < 3 {
				PrintDataAccountCreate()
				PrintDataLiteAccountCreate()
				return
			}
			out, err = CreateDataAccount(args[0], args[1:])
		}
		printOutput(cmd, out, err)
	},
}

var accountCreateDataLiteCmd = &cobra.Command{
	Use:   "lite",
	Short: "Create a lite data account",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Deprecation Warning!\nTo create a lite data account, use `accumulate account create data --lite ...`\n\n")
		if len(args) < 2 {
			PrintDataLiteAccountCreate()
			return
		}
		out, err := CreateLiteDataAccount(args[0], args[1:])
		printOutput(cmd, out, err)
	},
}

var accountQrCmd = &cobra.Command{
	Use:   "qr [url]",
	Short: "Display QR code for lite token account URL",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := QrAccount(args[0])
		printOutput(cmd, out, err)
	},
}

var accountGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a random lite token account or a lite account derived previously imported/created key",
	// validate the arguments passed to the command
	Args: cobra.OnlyValidArgs,
	Run: func(cmd *cobra.Command, args []string) {
		out, err := GenerateAccount(cmd, args[0:])
		printOutput(cmd, out, err)
	},
}

var accountListCmd = &cobra.Command{
	Use:   "list",
	Short: "Display all lite token accounts",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		out, err := ListAccounts()
		printOutput(cmd, out, err)
	},
}

var accountRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore old lite token accounts",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		out, err := RestoreAccounts()
		printOutput(cmd, out, err)
	},
}

func GetTokenAccount(url string) (string, error) {
	res, err := GetUrl(url)
	if err != nil {
		return "", err
	}

	if res.Type != protocol.AccountTypeTokenAccount.String() && res.Type != protocol.AccountTypeLiteTokenAccount.String() &&
		res.Type != protocol.AccountTypeLiteIdentity.String() &&
		res.Type != protocol.AccountTypeDataAccount.String() && res.Type != protocol.AccountTypeLiteDataAccount.String() {
		return "", fmt.Errorf("expecting token account or data account but received %v", res.Type)
	}

	return PrintChainQueryResponseV2(res)
}

func QrAccount(s string) (string, error) {
	u, err := url2.Parse(s)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid Accumulate URL: %v\n", s, err)
	}

	b := bytes.NewBufferString("")
	qrterminal.GenerateWithConfig(u.String(), qrterminal.Config{
		Level:          qrterminal.M,
		Writer:         b,
		HalfBlocks:     true,
		BlackChar:      qrterminal.BLACK_BLACK,
		BlackWhiteChar: qrterminal.BLACK_WHITE,
		WhiteChar:      qrterminal.WHITE_WHITE,
		WhiteBlackChar: qrterminal.WHITE_BLACK,
		QuietZone:      2,
	})

	r, err := io.ReadAll(b)
	return string(r), err
}

//CreateAccount account create url labelOrPubKeyHex height index tokenUrl keyBookUrl
func CreateAccount(cmd *cobra.Command, origin string, args []string) (string, error) {
	u, err := url2.Parse(origin)
	if err != nil {
		_ = cmd.Usage()
		return "", err
	}

	args, signer, err := prepareSigner(u, args)
	if err != nil {
		return "", err
	}
	if len(args) < 2 {
		return "", fmt.Errorf("not enough arguments")
	}

	accountUrl, err := url2.Parse(args[0])
	if err != nil {
		_ = cmd.Usage()
		return "", fmt.Errorf("invalid account url %s", args[0])
	}
	if u.Authority != accountUrl.Authority {
		return "", fmt.Errorf("account url to create (%s) doesn't match the authority adi (%s)", accountUrl.Authority, u.Authority)
	}
	tok, err := url2.Parse(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid token url")
	}

	//make sure this is a valid token account
	req := new(api.GeneralQuery)
	req.Url = tok
	resp := new(api.ChainQueryResponse)
	token := protocol.TokenIssuer{}
	resp.Data = &token
	err = Client.RequestAPIv2(context.Background(), "query", req, resp)
	if err != nil || resp.Type != protocol.AccountTypeTokenIssuer.String() {
		return "", fmt.Errorf("invalid token type %v", err)
	}

	tac := protocol.CreateTokenAccount{}
	tac.Url = accountUrl
	tac.TokenUrl = tok

	err = proveTokenIssuerExistence(&tac)
	if err != nil {
		return "", fmt.Errorf("unable to prove account state: %x", err)
	}

	for _, authUrlStr := range Authorities {
		authUrl, err := url2.Parse(authUrlStr)
		if err != nil {
			return "", err
		}
		tac.Authorities = append(tac.Authorities, authUrl)
	}

	return dispatchTxAndPrintResponse(&tac, u, signer)
}

func proveTokenIssuerExistence(body *protocol.CreateTokenAccount) error {
	if body.Url.LocalTo(body.TokenUrl) {
		return nil // Don't need a proof if the issuer is local
	}

	if protocol.AcmeUrl().Equal(body.TokenUrl) {
		return nil // Don't need a proof for ACME
	}

	// Get a proof of the create transaction
	req := new(api.GeneralQuery)
	req.Url = body.TokenUrl.WithFragment("transaction/0")
	req.Prove = true
	resp1 := new(api.TransactionQueryResponse)
	err := Client.RequestAPIv2(context.Background(), "query", req, resp1)
	if err != nil {
		return err
	}
	create, ok := resp1.Transaction.Body.(*protocol.CreateToken)
	if !ok {
		return fmt.Errorf("first transaction of %v is %v, expected %v", body.TokenUrl, resp1.Transaction.Body.Type(), protocol.TransactionTypeCreateToken)
	}

	// Start with a proof from the body hash to the transaction hash
	receipt := new(managed.Receipt)

	b, err := resp1.Transaction.Body.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal transaction header: %w", err)
	}
	headerHash := sha256.Sum256(b)
	receipt.Start = headerHash[:]

	b, err = resp1.Transaction.Header.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal transaction header: %w", err)
	}
	bodyHash := sha256.Sum256(b)
	receipt.Entries = []*managed.ReceiptEntry{{Hash: bodyHash[:]}}
	receipt.Anchor = resp1.Transaction.GetHash()

	// Add the proof from the issuer's main chain
	var gotProof bool
	for _, r := range resp1.Receipts {
		if !r.Account.Equal(body.TokenUrl) {
			continue
		}
		if r.Error != "" {
			return fmt.Errorf("get proof of %x: %s", resp1.TransactionHash[:4], r.Error)
		}
		gotProof = true
		receipt, err = receipt.Combine(&r.Proof)
		if err != nil {
			return err
		}
	}
	if !gotProof {
		return fmt.Errorf("missing proof for first transaction of %v", body.TokenUrl)
	}

	// Get a proof of the BVN anchor
	req = new(api.GeneralQuery)
	req.Url = protocol.DnUrl().JoinPath(protocol.AnchorPool).WithFragment(fmt.Sprintf("anchor/%x", receipt.Anchor))
	resp2 := new(api.ChainQueryResponse)
	err = Client.RequestAPIv2(context.Background(), "query", req, resp2)
	if err != nil {
		return err
	}
	if resp2.Receipt.Error != "" {
		return fmt.Errorf("failed to get proof of anchor: %s", resp2.Receipt.Error)
	}
	receipt, err = receipt.Combine(&resp2.Receipt.Proof)
	if err != nil {
		return err
	}

	body.Proof = new(protocol.TokenIssuerProof)
	body.Proof.Transaction = create
	body.Proof.Receipt = receipt
	return nil
}

func GenerateAccount(cmd *cobra.Command, args []string) (string, error) {
	// validate the amount arguments passed to the command
	if len(args) > 1 {
		return "", fmt.Errorf("too many arguments")
	}
	return GenerateKey("")
}

func ListAccounts() (string, error) {
	b, err := GetWallet().GetBucket(BucketLite)
	if err != nil {
		//no accounts so nothing to do...
		return "", fmt.Errorf("no lite accounts have been generated\n")
	}
	var out string

	if WantJsonOutput {
		out += "{\"liteAccounts\":["
	}
	for i, v := range b.KeyValueList {
		pubKey, err := GetWallet().Get(BucketLabel, v.Value)
		if err != nil {
			return "", err
		}

		st, err := GetWallet().Get(BucketSigType, pubKey)
		if err != nil {
			return "", err
		}
		s, _ := common.BytesUint64(st)
		var sigType protocol.SignatureType
		if !sigType.SetEnumValue(s) {
			return "", fmt.Errorf("invalid signature type")
		}

		k := new(Key)
		err = k.LoadByPublicKey(pubKey)
		if err != nil {
			return "", err
		}
		lt, err := protocol.LiteTokenAddressFromHash(k.PublicKeyHash(), protocol.ACME)
		if err != nil {
			return "", err
		}
		kr := KeyResponse{}
		kr.LiteAccount = lt
		kr.KeyType = sigType
		kr.PublicKey = pubKey
		*kr.Label.AsString() = string(v.Value)
		if WantJsonOutput {
			if i > 0 {
				out += ","
			}
			d, err := json.Marshal(&kr)
			if err != nil {
				return "", err
			}
			out += string(d)
		} else {
			out += fmt.Sprintf("\n\tkey name\t:\t%s\n\tlite account\t:\t%s\n\tpublic key\t:\t%x\n\tkey type\t:\t%s\n", v.Value, kr.LiteAccount, pubKey, sigType)
		}
	}
	if WantJsonOutput {
		out += "]}"
	}
	//TODO: this probably should also list out adi accounts as well
	return out, nil
}

func RestoreAccounts() (out string, err error) {
	walletVersion, err := GetWallet().GetRaw(db.BucketConfig, []byte("version"))
	if err == nil {
		var v db.Version
		v.FromBytes(walletVersion)
		//if there is no error getting version, check to see if it is the right version
		if db.WalletVersion.Compare(v) == 0 {
			//no need to update
			return "", nil
		}
		if db.WalletVersion.Compare(v) < 0 {
			return "", fmt.Errorf("cannot update wallet to an older version, wallet database version is %v, cli version is %v", v.String(), db.WalletVersion.String())
		}
	}

	anon, err := GetWallet().GetBucket(BucketAnon)
	if err == nil {
		for _, v := range anon.KeyValueList {
			u, err := url2.Parse(string(v.Key))
			if err != nil {
				out += fmt.Sprintf("%q is not a valid URL\n", v.Key)
			}
			if u != nil {
				key, _, err := protocol.ParseLiteTokenAddress(u)
				if err != nil {
					out += fmt.Sprintf("%q is not a valid lite account: %v\n", v.Key, err)
				} else if key == nil {
					out += fmt.Sprintf("%q is not a lite account\n", v.Key)
				}
			}

			label, _ := LabelForLiteTokenAccount(string(v.Key))
			v.Key = []byte(label)

			privKey := ed25519.PrivateKey(v.Value)
			pubKey := privKey.Public().(ed25519.PublicKey)
			out += fmt.Sprintf("Converting %s : %x\n", v.Key, pubKey)

			err = GetWallet().Put(BucketLabel, v.Key, pubKey)
			if err != nil {
				log.Fatal(err)
			}
			err = GetWallet().Put(BucketKeys, pubKey, privKey)
			if err != nil {
				return "", err
			}
			err = GetWallet().DeleteBucket(BucketAnon)
			if err != nil {
				return "", err
			}
		}
	}

	//fix the labels... there can be only one key one label.
	//should not have multiple labels to the same public key
	labelz, err := GetWallet().GetBucket(BucketLabel)
	if err != nil {
		//nothing to do...
		return
	}
	for _, v := range labelz.KeyValueList {
		label, isLite := LabelForLiteTokenAccount(string(v.Key))
		if isLite {
			//if we get here, then that means we have a bogus label.
			bogusLiteLabel := string(v.Key)
			//so check to see if it is in our regular key bucket
			otherPubKey, err := GetWallet().Get(BucketLabel, []byte(label))
			if err != nil {
				//key isn't found, so let's add it
				out += fmt.Sprintf("Converting %s to %s : %x\n", v.Key, label, v.Value)
				//so it doesn't exist, map the good label to the public key
				err = GetWallet().Put(BucketLabel, []byte(label), v.Value)
				if err != nil {
					return "", err
				}

				//now delete the bogus label
				err = GetWallet().Delete(BucketLabel, []byte(bogusLiteLabel))
				if err != nil {
					return "", err
				}
			} else {
				//ok so it does exist, now need to know if public key is the same, it is
				//an error if they don't match so warn user
				if !bytes.Equal(v.Value, otherPubKey) {
					out += fmt.Sprintf("public key stored for %v, doesn't match what is expected for a lite account: %s (%x != %x)\n",
						bogusLiteLabel, label, v.Value, otherPubKey)
				} else {
					//key isn't found, so let's add it
					out += fmt.Sprintf("Removing duplicate %s / %s : %x\n", v.Key, label, v.Value)
					//now delete the bogus label
					err = GetWallet().Delete(BucketLabel, []byte(bogusLiteLabel))
					if err != nil {
						return "", err
					}
				}
			}
		}
	}

	//build the map of lite accounts to key labels
	labelz, err = GetWallet().GetBucket(BucketLabel)
	if err != nil {
		//nothing to do...
		return
	}
	for _, v := range labelz.KeyValueList {
		k := new(Key)
		err = k.LoadByPublicKey(v.Value)
		if err != nil {
			return "", err
		}
		liteAccount, err := protocol.LiteTokenAddressFromHash(k.PublicKeyHash(), protocol.ACME)
		if err != nil {
			return "", err
		}

		//check to see if the key type has been assigned, if not set it to the ed25519Legacy...
		_, err = GetWallet().Get(BucketSigType, v.Value)
		if err != nil {
			//add the default key type
			out += fmt.Sprintf("assigning default key type %s for key name %v\n", k.Type, string(v.Key))

			err = GetWallet().Put(BucketSigType, v.Value, common.Uint64Bytes(k.Type.GetEnumValue()))
			if err != nil {
				return "", err
			}
		}

		liteLabel, _ := LabelForLiteTokenAccount(liteAccount.String())
		_, err = GetWallet().Get(BucketLite, []byte(liteLabel))
		if err == nil {
			continue
		}

		out += fmt.Sprintf("lite identity %v mapped to key name %v\n", liteLabel, string(v.Key))

		err = GetWallet().Put(BucketLite, []byte(liteLabel), v.Key)
		if err != nil {
			return "", err
		}
	}

	//update wallet version
	err = GetWallet().PutRaw(db.BucketConfig, []byte("version"), db.WalletVersion.Bytes())
	if err != nil {
		return "", err
	}

	return out, nil
}
