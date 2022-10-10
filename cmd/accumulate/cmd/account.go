package cmd

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/mdp/qrterminal"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/db"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd"
	api2 "gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/api"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	url2 "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

func init() {
	accountCmd.AddCommand(
		accountGetCmd,
		accountCreateCmd,
		accountQrCmd,
		accountGenerateCmd,
		accountListCmd,
		accountLockCmd,
	)

	accountCreateCmd.AddCommand(
		accountCreateTokenCmd,
		accountCreateDataCmd)

	accountCreateDataCmd.AddCommand(
		accountCreateDataLiteCmd)

	accountCreateTokenCmd.Flags().BoolVar(&flagAccount.Lite, "lite", false, "Create a lite token account")
	accountCreateDataCmd.Flags().BoolVar(&flagAccount.Lite, "lite", false, "Create a lite data account")
	accountGenerateCmd.Flags().StringVar(&SigType, "sigtype", "ed25519", "Specify the signature type use rcd1 for RCD1 type ; ed25519 for accumulate ED25519 ; btc for Bitcoin ; btclegacy for LegacyBitcoin  ; eth for Ethereum ")
	accountCreateDataCmd.Flags().StringVar(&flagAccount.LiteData, "lite-data", "", "Add first entry data to lite data account")
	accountLockCmd.Flags().BoolVarP(&flagAccount.Force, "force", "f", false, "Do not prompt the user")
}

var flagAccount = struct {
	Lite     bool
	LiteData string
	Force    bool
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
		runTxnCmdFunc(CreateTokenAccount)(cmd, args)
	},
}

var accountCreateTokenCmd = &cobra.Command{
	Use: "token [actor adi] [key name[@key book or page]]  [new token account url] [tokenUrl]",
	// Or token --lite [lite token account url] --sign-with [key name[@key book or page]]
	Short: "Create an ADI token account",
	Args:  cobra.RangeArgs(1, 6),
	Run:   runTxnCmdFunc(CreateTokenAccount),
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
	Use:   "generate --sigtype (optional)",
	Short: "Generate a random lite token account or a lite account with previously specified signature type use",
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

var accountLockCmd = &cobra.Command{
	Use:   "lock [account url] [signing key name] [height]",
	Short: "Lock the account until the given block height",
	Args:  cobra.ExactArgs(2),
	Run:   runTxnCmdFunc(lockAccount),
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
		return "", fmt.Errorf("%q is not a valid Accumulate URL: %v", s, err)
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

//CreateTokenAccount account create url labelOrPubKeyHex height index tokenUrl keyBookUrl
func CreateTokenAccount(principal *url2.URL, signers []*signing.Builder, args []string) (string, error) {
	if flagAccount.Lite {
		return CreateLiteTokenAccount(principal, signers, args)
	}

	if len(args) < 2 {
		return "", fmt.Errorf("wrong number of arguments")
	}

	accountUrl, err := url2.Parse(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid account url %s", args[0])
	}
	if principal.Authority != accountUrl.Authority {
		return "", fmt.Errorf("account url to create (%s) doesn't match the authority adi (%s)", accountUrl.Authority, principal.Authority)
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

	return dispatchTxAndPrintResponse(&tac, principal, signers)
}

// CreateLiteTokenAccount usage is:
// accumulate account create token --lite ${LTA} --sign-with ${KEY}@${SIGNER}
func CreateLiteTokenAccount(principal *url2.URL, signers []*signing.Builder, args []string) (string, error) {
	if len(args) != 0 {
		return "", fmt.Errorf("wrong number of arguments")
	}

	if len(signers) == 0 || !signers[0].Url.Equal(principal.RootIdentity()) {
		log.Fatal("Internal error: expected first signer to be the lite identity")
	}
	signers = signers[1:]
	if len(signers) == 0 {
		return "", fmt.Errorf("an additional signer must be specified by --sign-with")
	}
	signers[0].SetTimestampToNow()

	key, tok, err := protocol.ParseLiteTokenAddress(principal)
	if err != nil {
		return "", fmt.Errorf("invalid lite token address: %w", err)
	} else if key == nil {
		return "", fmt.Errorf("not a lite token address: %v", principal)
	}

	if !protocol.AcmeUrl().Equal(tok) {
		return "", fmt.Errorf("create lite token account does not support creating non-ACME accounts")
	}

	body := new(protocol.CreateLiteTokenAccount)
	return dispatchTxAndPrintResponse(body, principal, signers)
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

func GenerateAccount(_ *cobra.Command, args []string) (string, error) {
	// validate the amount arguments passed to the command
	if len(args) > 1 {
		return "", fmt.Errorf("too many arguments")
	}
	return GenerateKey("")
}

func ListAccounts() (string, error) {
	b, err := walletd.GetWallet().GetBucket(walletd.BucketLite)
	if err != nil {
		//no accounts so nothing to do...
		return "", fmt.Errorf("no lite accounts have been generated")
	}
	var out string

	if WantJsonOutput {
		out += "{\"liteAccounts\":["
	}
	for i, v := range b.KeyValueList {
		k := new(walletd.Key)
		err = k.LoadByLabel(string(v.Value))
		if err != nil {
			return "", err
		}

		lt, err := protocol.LiteTokenAddressFromHash(k.PublicKeyHash(), protocol.ACME)
		if err != nil {
			return "", err
		}
		kr := KeyResponse{}
		kr.LiteAccount = lt
		kr.KeyInfo = k.KeyInfo
		kr.PublicKey = k.PublicKey
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
			out += fmt.Sprintf("\n\tkey name\t:\t%s\n\tlite account\t:\t%s\n\tpublic key\t:\t%x\n\tkey type\t:\t%s\n\tderivation\t:\t%s\n", v.Value, kr.LiteAccount, k.PublicKey, k.KeyInfo.Type, k.KeyInfo.Derivation)
		}
	}
	if WantJsonOutput {
		out += "]}"
	}
	//TODO: this probably should also list out adi accounts as well
	return out, nil
}

func lockAccount(principal *url2.URL, signers []*signing.Builder, args []string) (string, error) {
	var err error
	body := new(protocol.LockAccount)
	body.Height, err = strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid height argument: %v", err)
	}

	if flagAccount.Force {
		return dispatchTxAndPrintResponse(body, principal, signers)
	}

	req := new(api.MajorBlocksQuery)
	req.Url = protocol.DnUrl()
	req.Start = 0
	req.Count = 0
	res, err := Client.QueryMajorBlocks(context.Background(), req)
	if err != nil {
		return PrintJsonRpcError(err)
	}

	var latest *api.MajorQueryResponse
	if res.Total == 0 {
		latest = new(api.MajorQueryResponse)
	} else {
		req.Start = res.Total
		req.Count = 1
		res, err = Client.QueryMajorBlocks(context.Background(), req)
		if err != nil {
			return PrintJsonRpcError(err)
		}
		if len(res.Items) == 0 {
			return "", fmt.Errorf("failed to query latest major block: empty response")
		}
		err = Remarshal(res.Items[0], latest)
		if err != nil {
			return "", fmt.Errorf("failed to parse query response: %w", err)
		}

		if body.Height <= latest.MajorBlockIndex {
			return "", fmt.Errorf("specified height (%d) is before or the same as the current major block height (%d)", body.Height, latest.MajorBlockIndex)
		}
	}

	days := float64(body.Height-latest.MajorBlockIndex) / 2
	fmt.Printf("This will lock your account for %.1f days. Are you sure [yN]? ", days)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", nil
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		return "", nil
	}

	return dispatchTxAndPrintResponse(body, principal, signers)
}

func ExportAccounts(filePath string) error {
	res := api2.Wallet{}
	walletVersion, err := walletd.GetWallet().GetRaw(db.BucketConfig, []byte("version"))
	if err != nil {
		return err
	}
	var v db.Version
	v.FromBytes(walletVersion)
	res.Version.Commit = uint64(v.Commit())
	res.Version.Major = uint64(v.Major())
	res.Version.Minor = uint64(v.Minor())
	res.Version.Revision = uint64(v.Revision())

	b, err := walletd.GetWallet().GetBucket(walletd.BucketLabel)
	if err != nil {
		//no accounts so nothing to do...
		return fmt.Errorf("no lite accounts have been generated")
	}
	//var res []*KeyResponse
	for i := range b.KeyValueList {
		k := new(walletd.Key)
		err = k.LoadByPublicKey(b.KeyValueList[i].Value)
		if err != nil {
			log.Printf("cannot load key by label %s with public key %x, %v", b.KeyValueList[i].Key, b.KeyValueList[i].Value, err)
			continue
		}
		res.Keys = append(res.Keys, k.Key)
		kn := api2.KeyName{}
		kn.Name = string(b.KeyValueList[i].Key)
		kn.PublicKey = b.KeyValueList[i].Value
		res.KeyNames = append(res.KeyNames, kn)
	}

	l, err := walletd.GetWallet().GetBucket(walletd.BucketLite)
	if err != nil {
		log.Println(err)
	}
	for i := range l.KeyValueList {
		label := api2.LiteLabel{}
		label.LiteName = string(l.KeyValueList[i].Key)
		label.KeyName = string(l.KeyValueList[i].Value)
		res.LiteLabels = append(res.LiteLabels, label)
	}

	a, err := walletd.GetWallet().GetBucket(walletd.BucketAdi)
	if err == nil {
		for _, v := range a.KeyValueList {
			u, err := url2.Parse(string(v.Key))
			if err != nil {
				a := api2.Adi{}
				p := api2.Page{}
				a.Url = *u
				//page url's aren't currently stored
				a.Pages = append(a.Pages, p)
				lab, err := walletd.FindLabelFromPubKey(v.Value)
				if err != nil {
					p.KeyNames = append(p.KeyNames, hex.EncodeToString(v.Value))
				} else {
					p.KeyNames = append(p.KeyNames, lab)
				}
				res.Adis = append(res.Adis, a)
			}
		}
	}

	bucket, err := walletd.GetWallet().GetBucket(walletd.BucketMnemonic)
	if err != nil {
		log.Println("mnemonic bucket doesn't exist")
	} else {
		for _, v := range bucket.KeyValueList {
			switch string(v.Key) {
			case "phrase":
				phrase, err := walletd.GetWallet().Get(walletd.BucketMnemonic, []byte("phrase"))
				if err != nil {
					log.Println("mnemonic seed doesn't exist, thus was not exported")
				}
				if phrase != nil {
					res.SeedInfo.Mnemonic = string(phrase)
				}
			case "seed":
				res.SeedInfo.Seed, err = walletd.GetWallet().Get(walletd.BucketMnemonic, []byte("seed"))
				if err != nil {
					log.Println("mnemonic seed doesn't exist, thus was not exported")
				}
			default:
				//these are the sig types
				dc := api2.DerivationCount{}
				dc.Count = uint64(binary.LittleEndian.Uint32(v.Value))
				var found bool
				dc.Type, found = protocol.SignatureTypeByName(string(v.Key))
				if found {
					res.SeedInfo.Derivations = append(res.SeedInfo.Derivations, dc)
				}
			}
		}
	}

	bin, err := json.MarshalIndent(&res, "", "  ")

	if err != nil {
		log.Printf("cannot convert export to json for export, error: %v", err)
	}

	if _, err := os.Stat(filePath); err == nil {
		opt, err := promptOverwrite()
		if err != nil {
			return err
		}
		if strings.EqualFold(opt, "yes") {
			file, err := os.OpenFile(filePath, os.O_RDWR|os.O_TRUNC, 0644)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(file, strings.NewReader(string(bin)))
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("operation skipped")
		}

	} else if errors.Is(err, os.ErrNotExist) {
		out, err := os.Create(filePath)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, strings.NewReader(string(bin)))
		if err != nil {
			return err
		}
	} else {
		return err
	}
	return nil
}

func ImportAccounts(filePath string) error {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed reading data from file: %s", err)
	}
	var req *api2.Wallet
	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}

	mnemonics := strings.Split(req.SeedInfo.Mnemonic, " ")
	_, err = walletd.ImportMnemonic(mnemonics)
	if err != nil {
		return fmt.Errorf("failed importing mnemonic: %s", err)
	}
	//import the sig type derivation address count
	for _, v := range req.SeedInfo.Derivations {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], uint32(v.Count))
		err = walletd.GetWallet().Put(walletd.BucketMnemonic, []byte(v.Type.String()), b[:])
		if err != nil {
			log.Printf("failed to set derivation counter for %s", v.Type.String())
		}
	}

	for _, v := range req.Keys {
		b, err := v.KeyInfo.MarshalBinary()
		if err != nil {
			log.Printf("invalid key info provided for key %x, %v", v.PublicKey, err)
			continue
		}
		err = walletd.GetWallet().Put(walletd.BucketKeys, v.PublicKey, v.PrivateKey)
		if err != nil {
			log.Printf("failed to store private key for key %x, %v", v.PublicKey, err)
			continue
		}
		err = walletd.GetWallet().Put(walletd.BucketKeyInfo, v.PublicKey, b)
		if err != nil {
			log.Printf("failed to store key info for key %x, %v", v.PublicKey, err)
		}
	}

	for _, v := range req.KeyNames {
		err = walletd.GetWallet().Put(walletd.BucketLabel, []byte(v.Name), v.PublicKey)
		if err != nil {
			log.Printf("failed to store bucket label %s for key %x, %v", v.Name, v.PublicKey, err)
		}
	}

	for _, v := range req.LiteLabels {
		err = walletd.GetWallet().Put(walletd.BucketLite, []byte(v.LiteName), []byte(v.KeyName))
		if err != nil {
			log.Printf("failed to store bucket lite label %s for key name %s, %v", v.LiteName, v.KeyName, err)
		}
	}

	for _, adi := range req.Adis {
		if len(adi.Pages) == 0 {
			log.Printf("skipping adi import, %s, missing key page", adi.Url.String())
			continue
		}

		//pages for adi in version 1 is just a key since we don't actually store keys mapped to pages yet
		if len(adi.Pages[0].KeyNames) == 0 {
			log.Printf("skipping adi import, %s, missing key in page", adi.Url.String())
			continue
		}

		k, err := walletd.LookupByLabel(adi.Pages[0].KeyNames[0])
		if err != nil {
			log.Printf("skipping adi import, %s, cannot find key for name %s", adi.Url.String(), adi.Pages[0].KeyNames[0])
			continue
		}

		err = walletd.GetWallet().Put(walletd.BucketAdi, []byte(adi.Url.Authority), k.PublicKey)
		if err != nil {
			log.Printf("skipping adi import for %s with key %s DB error: %v", adi.Url.String(), adi.Pages[0].KeyNames[0], err)
		}
	}

	version := db.NewVersion(int(req.Version.Commit), int(req.Version.Major), int(req.Version.Minor), int(req.Version.Revision))
	err = walletd.GetWallet().PutRaw(db.BucketConfig, []byte("version"), version.Bytes())
	if err != nil {
		log.Printf("failed to store version info %v", err)
	}

	return nil
}

func promptOverwrite() (string, error) {
	pc := promptContent{
		"",
		"File already exists. Do you want to overwrite?",
	}
	items := []string{"Yes", "No"}
	index := -1
	var result string
	var err error

	for index < 0 {
		prompt := promptui.SelectWithAdd{
			Label: pc.label,
			Items: items,
		}
		index, result, err = prompt.Run()
		if index == -1 {
			items = append(items, result)
		}
	}

	if err != nil {
		return "", err
	}

	return result, nil
}
