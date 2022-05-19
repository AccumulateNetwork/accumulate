package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var Keyname string

func init() {
	dataCmd.Flags().StringVar(&Keyname, "sign-data", "", "specify this to send random data as a signed & valid entry to data account")
}

var dataCmd = &cobra.Command{
	Use:   "data",
	Short: "Create, add, and query adi data accounts",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) == 0 {
			PrintData()
			printOutput(cmd, out, err)
			return
		}

		switch args[0] {
		case "get":
			switch len(args) {
			case 1:
				PrintDataGet()
			case 2:
				out, err = GetDataEntry(args[1], []string{})
			case 3:
				out, err = GetDataEntry(args[1], args[2:])
			default:
				out, err = GetDataEntrySet(args[1], args[2:])
			}
		case "write":
			if len(args) > 2 {
				out, err = WriteData(args[1], args[2:])
				if err != nil {
					fmt.Println("Usage:")
					PrintDataWrite()
				}
			} else {
				PrintDataWrite()
			}
		case "write-to":
			if len(args) > 2 {
				out, err = WriteDataTo(args[1], args[2:])
				if err != nil {
					fmt.Println("Usage:")
					PrintDataWriteTo()
				}
			} else {
				PrintDataWriteTo()
			}
		default:
			PrintData()
		}
		printOutput(cmd, out, err)
	},
}

func PrintDataGet() {
	fmt.Println("  accumulate data get [DataAccountURL]			  Get most current data entry by URL")
	fmt.Println("  accumulate data get [DataAccountURL] [EntryHash]  Get data entry by entryHash in hex")
	fmt.Println("  accumulate data get [DataAccountURL] [start index] [count] expand(optional) Get a set of data entries starting from start and going to start+count, if \"expand\" is specified, data entries will also be provided")
	//./cli data get acc://actor/dataAccount
	//./cli data get acc://actor/dataAccount entryHash
	//./cli data get acc://actor/dataAccount start limit
}

func PrintDataAccountCreate() {
	//./cli data create acc://actor key idx height acc://actor/dataAccount acc://actor/keyBook (optional)
	fmt.Println("  accumulate account create data [actor adi url] [signing key name] [key index (optional)] [key height (optional)] [adi data account url] [key book (optional)] Create new data account")
	fmt.Println("\t\t example usage: accumulate account create data acc://actor signingKeyName acc://actor/dataAccount acc://actor/book0")

	//scratch data account
	fmt.Println("  accumulate account create data --scratch [actor adi url] [signing key name] [key index (optional)] [key height (optional)] [adi data account url] [key book (optional)] Create new data account")
	fmt.Println("\t\t example usage: accumulate account create data --scratch acc://actor signingKeyName acc://actor/dataAccount acc://actor/book0")
}

func PrintDataWrite() {
	fmt.Println("accumulate data write [data account url] [signingKey] [extid_0 (optional)] ... [extid_n (optional)] [data] Write entry to your data account. Note: extid's and data needs to be a quoted string or hex")
	fmt.Println("accumulate data write [data account url] [signingKey] --sign-data [keyname] [extid_0 (optional)] ... [extid_n (optional)] [data] Write entry to your data account. Note: extid's and data needs to be a quoted string or hex")

}

func PrintDataWriteTo() {
	fmt.Println("accumulate data write-to [account url] [signing key] [lite data account] [extid_0 (optional)] ... [extid_n (optional)] [data]")
}

func PrintDataLiteAccountCreate() {
	fmt.Println("  accumulate account create data lite [lite token account] [name_0] ... [name_n] Create new lite data account creating a chain based upon a name list")
	fmt.Println("  accumulate account create data lite [origin url] [signing key name]  [key index (optional)] [key height (optional)] [name_0] ... [name_n] Create new lite data account creating a chain based upon a name list")
	fmt.Println("\t\t example usage: accumulate account create data lite acc://actor signingKeyName example1 example2 ")
}

func PrintData() {
	PrintDataAccountCreate()
	PrintDataLiteAccountCreate()
	PrintDataGet()
	PrintDataWrite()
	PrintDataWriteTo()
}

func GetDataEntry(accountUrl string, args []string) (string, error) {
	u, err := url.Parse(accountUrl)
	if err != nil {
		return "", err
	}

	params := api.DataEntryQuery{}
	params.Url = u
	if len(args) > 0 {
		n, err := hex.Decode(params.EntryHash[:], []byte(args[0]))
		if err != nil {
			return "", err
		}
		if n != 32 {
			return "", fmt.Errorf("entry hash must be 64 hex characters in length")
		}
	}

	var res QueryResponse

	data, err := json.Marshal(&params)
	if err != nil {
		return "", err
	}

	err = Client.RequestAPIv2(context.Background(), "query-data", json.RawMessage(data), &res)
	if err != nil {
		return "", err
	}

	return PrintChainQueryResponseV2(&res)
}

func GetDataEntrySet(accountUrl string, args []string) (string, error) {
	u, err := url.Parse(accountUrl)
	if err != nil {
		return "", err
	}

	if len(args) > 3 || len(args) < 2 {
		return "", fmt.Errorf("expecting the start index and count parameters with optional expand")
	}

	params := api.DataEntrySetQuery{}
	params.Url = u

	v, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid start argument %s, %v", args[1], err)
	}
	params.Start = uint64(v)

	v, err = strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid count argument %s, %v", args[1], err)
	}
	params.Count = uint64(v)

	if len(args) > 2 {
		if args[2] == "expand" {
			params.Expand = true
		}
	}

	var res api.MultiResponse
	data, err := json.Marshal(&params)
	if err != nil {
		return "", err
	}

	err = Client.RequestAPIv2(context.Background(), "query-data-set", json.RawMessage(data), &res)
	if err != nil {
		return "", err
	}

	return PrintMultiResponse(&res)
}

func CreateLiteDataAccount(origin string, args []string) (string, error) {
	if flagAccount.Scratch {
		return "", fmt.Errorf("lite scratch data accounts are not supported")
	}

	u, err := url.Parse(origin)
	if err != nil {
		return "", err
	}

	args, signer, err := prepareSigner(u, args)
	if err != nil {
		return "", fmt.Errorf("unable to prepare signer, %v", err)
	}

	if len(args) < 1 {
		return "", fmt.Errorf("expecting account url or 'lite' keyword")
	}

	var res *api.TxResponse
	//compute the chain id...
	wdt := protocol.WriteDataTo{}
	wdt.Entry = prepareData(args, true)

	accountId := protocol.ComputeLiteDataAccountId(wdt.Entry)
	addr, err := protocol.LiteDataAddress(accountId)
	if err != nil {
		return "", fmt.Errorf("invalid lite data address created from name(s)")
	}
	wdt.Recipient = addr

	lite, _ := GetUrl(wdt.Recipient.String())
	if lite != nil {
		return "", fmt.Errorf("lite data address already exists %s", addr)
	}

	lde := protocol.FactomDataEntry{}
	copy(lde.AccountId[:], accountId)
	data := wdt.Entry.GetData()
	if len(data) > 0 {
		lde.Data = data[0]
		lde.ExtIds = data[1:]
	}
	entryHash := lde.Hash()

	res, err = dispatchTxAndWait(&wdt, nil, u, signer)
	if err != nil {
		return PrintJsonRpcError(err)
	}

	return ActionResponseFromLiteData(res, addr.String(), accountId, entryHash).Print()
}

func CreateDataAccount(origin string, args []string) (string, error) {
	u, err := url.Parse(origin)
	if err != nil {
		return "", err
	}

	args, signer, err := prepareSigner(u, args)
	if err != nil {
		return "", fmt.Errorf("unable to prepare signer, %v", err)
	}

	if len(args) < 1 {
		return "", fmt.Errorf("expecting account url or 'lite' keyword")
	}

	accountUrl, err := url.Parse(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid account url %s", args[0])
	}
	if u.Authority != accountUrl.Authority {
		return "", fmt.Errorf("account url to create (%s) doesn't match the authority adi (%s)", accountUrl.Authority, u.Authority)
	}

	cda := protocol.CreateDataAccount{}
	cda.Url = accountUrl
	cda.Scratch = flagAccount.Scratch

	if len(args) > 2 {
		keybook, err := url.Parse(args[2])
		if err != nil {
			return "", fmt.Errorf("invalid key book url")
		}
		cda.Authorities = append(cda.Authorities, keybook)
	}

	for _, authUrlStr := range Authorities {
		authUrl, err := url.Parse(authUrlStr)
		if err != nil {
			return "", err
		}
		cda.Authorities = append(cda.Authorities, authUrl)
	}

	return dispatchTxAndPrintResponse(&cda, nil, u, signer)
}

func WriteData(accountUrl string, args []string) (string, error) {
	u, err := url.Parse(accountUrl)
	if err != nil {
		return "", err
	}

	args, signer, err := prepareSigner(u, args)
	if err != nil {
		return "", err
	}

	if len(args) < 1 {
		return "", fmt.Errorf("expecting account url")
	}
	wd := protocol.WriteData{}
	if Keyname != "" {
		key, err := GetWallet().Get(BucketKeys, []byte(Keyname))
		if err != nil {
			return "", fmt.Errorf("Key %s does not exist in the wallet %v", hex.EncodeToString(key), err)
		}
		var privKey *Key
		privKey, err = resolvePrivateKey(Keyname)
		if err != nil {
			return "", fmt.Errorf("Key does not exist in the wallet %v", err)
		}
		argcopy := args
		argcopy[1] = Keyname
		argCopy, kSigner, err := prepareSigner(u, argcopy)
		if err != nil {
			return "", err
		}
		wd.Entry, err = prepareAnyData(argCopy, false, privKey, kSigner)
		if err != nil {
			return PrintJsonRpcError(err)
		}
	} else {
		wd.Entry = prepareData(args, false)
	}
	res, err := dispatchTxAndWait(&wd, nil, u, signer)
	if err != nil {
		return PrintJsonRpcError(err)
	}
	return ActionResponseFromData(res, wd.Entry.Hash()).Print()
}

func prepareData(args []string, isFirstLiteEntry bool) *protocol.AccumulateDataEntry {
	entry := new(protocol.AccumulateDataEntry)
	if isFirstLiteEntry {
		entry.Data = append(entry.Data, []byte{})
	}
	for i := 0; i < len(args); i++ {
		data := make([]byte, len(args[i]))

		//attempt to hex decode it
		n, err := hex.Decode(data, []byte(args[i]))
		if err != nil {
			//if it is not a hex string, then just store the data as-is
			copy(data, args[i])
		} else {
			//clip the padding
			data = data[:n]
		}
		entry.Data = append(entry.Data, data)
	}
	return entry
}

func prepareAnyData(args []string, isFirstLiteEntry bool, key *Key, signer *signing.Builder) (*protocol.AccumulateDataEntry, error) {
	entry := new(protocol.AccumulateDataEntry)

	if isFirstLiteEntry {
		entry.Data = append(entry.Data, []byte{})
	}
	for i := 0; i < len(args); i++ {
		data := make([]byte, len(args[i]))

		//attempt to hex decode it
		n, err := hex.Decode(data, []byte(args[i]))
		if err != nil {
			//if it is not a hex string, then just store the data as-is
			copy(data, args[i])
		} else {
			//clip the padding
			data = data[:n]
		}

		entry.Data = append(entry.Data, data)

	}
	var signData *protocol.DataSigningInfo
	signData.PublicKey = key.PublicKey
	signData.Salt = strconv.FormatInt(time.Now().UnixNano(), 10)
	fullDat := []byte{}
	for _, d := range entry.Data {
		fullDat = append(fullDat, d...)
	}
	saltHash := sha256.Sum256(append(fullDat, []byte(signData.Salt)...))
	sig, err := signer.Sign(saltHash[:])
	if err != nil {
		return nil, err
	}
	sign, err := sig.MarshalBinary()
	if err != nil {
		return nil, err
	}
	signData.Signature = sign
	finData, err := signData.MarshalBinary()
	if err != nil {
		return nil, err
	}
	entry.Data = append(entry.Data, finData)
	return entry, nil
}

func WriteDataTo(accountUrl string, args []string) (string, error) {
	u, err := url.Parse(accountUrl)
	if err != nil {
		return "", err
	}

	args, signer, err := prepareSigner(u, args)
	if err != nil {
		return "", err
	}

	if len(args) < 1 {
		return "", fmt.Errorf("expecting lite data account url")
	}

	wd := protocol.WriteDataTo{}
	r, err := url.Parse(args[0])
	if err != nil {
		return "", fmt.Errorf("unable to parse lite token account url")
	}

	accountId, err := protocol.ParseLiteDataAddress(r)
	if err != nil {
		return "", fmt.Errorf("invalid lite data account url")
	}

	wd.Recipient = r

	if len(args) < 2 {
		return "", fmt.Errorf("expecting data")
	}

	wd.Entry = prepareData(args[1:], false)

	res, err := dispatchTxAndWait(&wd, nil, u, signer)
	if err != nil {
		return "", err
	}

	lda := protocol.LiteDataAccount{}
	q, err := GetUrl(wd.Recipient.String())
	if err == nil {
		err = Remarshal(q.Data, &lda)
		if err != nil {
			return "", err
		}
	}

	lde := protocol.FactomDataEntry{}
	copy(lde.AccountId[:], append(accountId, lda.Tail...))
	data := wd.Entry.GetData()
	if len(data) > 0 {
		lde.Data = data[0]
		lde.ExtIds = data[1:]
	}
	return ActionResponseFromLiteData(res, wd.Recipient.String(), lde.AccountId[:], wd.Entry.Hash()).Print()
}
