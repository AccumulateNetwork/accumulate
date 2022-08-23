package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var Keyname string
var WriteState bool
var Scratch bool

func init() {
	dataCmd.Flags().StringVar(&Keyname, "sign-data", "", "specify this to send random data as a signed & valid entry to data account")
	dataCmd.PersistentFlags().BoolVar(&WriteState, "write-state", false, "Write to the account's state")
	dataCmd.Flags().BoolVar(&Scratch, "scratch", false, "Write to the scratch chain")

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
	//./cli data get acc://actor.acme/dataAccount
	//./cli data get acc://actor.acme/dataAccount entryHash
	//./cli data get acc://actor.acme/dataAccount start limit
}

func PrintDataAccountCreate() {
	//./cli data create acc://actor.acme key idx height acc://actor.acme/dataAccount acc://actor.acme/keyBook (optional)
	fmt.Println("  accumulate account create data [actor adi url] [key name[@key book or page]]  [adi data account url] --authority key book (optional) Create new data account")
	fmt.Println("\t\t example usage: accumulate account create data acc://actor.acme signingKeyName acc://actor.acme/dataAccount --authority acc://actor.acme/book0")

	//scratch data account
	fmt.Println("  accumulate account create data --scratch [actor adi url] [key name[@key book or page]]  [adi data account url] --authority key book (optional) Create new data account")
	fmt.Println("\t\t example usage: accumulate account create data --scratch acc://actor.acme signingKeyName acc://actor.acme/dataAccount --authority acc://actor.acme/book0")
}

func PrintDataWrite() {
	fmt.Println("accumulate data write [data account url] [signingKey] --scratch (optional) [extid_0 (optional)] ... [extid_n (optional)] [data] Write entry to your data account. Note: extid's and data needs to be a quoted string or hex")
	fmt.Println("accumulate data write [data account url] [signingKey] --scratch (optional) --sign-data [keyname] [extid_0 (optional)] ... [extid_n (optional)] [data] Write entry to your data account. Note: extid's and data needs to be a quoted string or hex")

}

func PrintDataWriteTo() {
	fmt.Println("accumulate data write-to [account url] [signing key] [lite data account] [extid_0 (optional)] ... [extid_n (optional)] [data]")
}

func PrintDataLiteAccountCreate() {
	fmt.Println("  accumulate account create data lite [lite token account] [name_0] ... [name_n] Create new lite data account creating a chain based upon a name list")
	fmt.Println("  accumulate account create data lite [origin] [key name[@key book or page]] [name_0] ... [name_n] Create new lite data account creating a chain based upon a name list")
	fmt.Println("\t\t example usage: accumulate account create data lite acc://actor.acme signingKeyName example1 example2 ")
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
	u, err := url.Parse(origin)
	if err != nil {
		return "", err
	}

	args, signers, err := prepareSigner(u, args)
	if err != nil {
		return "", fmt.Errorf("unable to prepare signers, %v", err)
	}

	if len(args) < 1 {
		return "", fmt.Errorf("expecting account url or 'lite' keyword")
	}

	var res *api.TxResponse
	//compute the chain id...
	wdt := protocol.WriteDataTo{}

	wdt.Entry, err = prepareData(args, true, nil)
	if err != nil {
		return "", err
	}

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
	res, resps, err := dispatchTxAndWait(&wdt, u, signers)
	if err != nil {
		return PrintJsonRpcError(err)
	}
	ar := ActionResponseFromLiteData(res, addr.String(), accountId, entryHash)
	ar.Flow = resps
	return ar.Print()
}

func CreateDataAccount(origin string, args []string) (string, error) {
	u, err := url.Parse(origin)
	if err != nil {
		return "", err
	}

	args, signers, err := prepareSigner(u, args)
	if err != nil {
		return "", fmt.Errorf("unable to prepare signers, %v", err)
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

	for _, authUrlStr := range Authorities {
		authUrl, err := url.Parse(authUrlStr)
		if err != nil {
			return "", err
		}
		cda.Authorities = append(cda.Authorities, authUrl)
	}

	return dispatchTxAndPrintResponse(&cda, u, signers)
}

func WriteData(accountUrl string, args []string) (string, error) {
	u, err := url.Parse(accountUrl)
	if err != nil {
		return "", err
	}

	args, signers, err := prepareSigner(u, args)
	if err != nil {
		return "", err
	}

	if len(args) < 1 {
		return "", fmt.Errorf("expecting account url")
	}
	wd := protocol.WriteData{}
	wd.WriteToState = WriteState
	wd.Scratch = Scratch

	var kSigners []*signing.Builder
	if Keyname != "" {
		u, err := url.Parse(Keyname)
		if err != nil {
			return "", err
		}
		key, err := walletd.LookupByLabel(Keyname)
		if err != nil {
			return "", err
		}
		signer := new(signing.Builder)
		signer.Type = key.KeyInfo.Type
		signer.SetTimestampToNow()
		signer.Url = u.RootIdentity()
		signer.Version = 1
		signer.SetPrivateKey(key.PrivateKey)
		kSigners = append(kSigners, signer)
	}

	wd.Entry, err = prepareData(args, false, kSigners)
	if err != nil {
		return PrintJsonRpcError(err)
	}
	res, resps, err := dispatchTxAndWait(&wd, u, signers)
	if err != nil {
		return "", err
	}
	ar := ActionResponseFromData(res, wd.Entry.Hash())
	ar.Flow = resps
	return ar.Print()
}

func prepareData(args []string, isFirstLiteEntry bool, signers []*signing.Builder) (*protocol.AccumulateDataEntry, error) {
	entry := new(protocol.AccumulateDataEntry)
	if isFirstLiteEntry {
		data := []byte{}
		if flagAccount.LiteData != "" {
			n, err := hex.Decode(data, []byte(flagAccount.LiteData))
			if err != nil {
				//if it is not a hex string, then just store the data as-is
				copy(data, flagAccount.LiteData)
			} else {
				//clip the padding
				data = data[:n]
			}
		}
		entry.Data = append(entry.Data, data)
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

	if len(signers) > 0 {
		fullDat := []byte{}
		for _, d := range entry.Data {
			fullDat = append(fullDat[:], d...)
		}
		fullDatHash := sha256.Sum256(fullDat[:])
		sig, err := signers[0].SetTimestampToNow().Sign(fullDatHash[:])

		if err != nil {
			return nil, err
		}

		finData, err := sig.MarshalBinary()

		if err != nil {
			return nil, err
		}
		dataCopy := [][]byte{}
		dataCopy = append(dataCopy, finData)
		dataCopy = append(dataCopy, entry.Data...)
		entry.Data = dataCopy
	}
	return entry, nil
}

func WriteDataTo(accountUrl string, args []string) (string, error) {
	u, err := url.Parse(accountUrl)
	if err != nil {
		return "", err
	}

	args, signers, err := prepareSigner(u, args)
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

	// Remove the recipient from the arg list
	args = args[1:]
	if len(args) == 0 {
		return "", fmt.Errorf("expecting data")
	}

	var kSigners []*signing.Builder
	if Keyname != "" {
		u, err := url.Parse(Keyname)
		if err != nil {
			return "", err
		}
		key, err := walletd.LookupByLabel(Keyname)
		if err != nil {
			return "", err
		}
		signer := new(signing.Builder)
		signer.Type = key.KeyInfo.Type
		signer.SetTimestampToNow()
		signer.Url = u.RootIdentity()
		signer.Version = 1
		signer.SetPrivateKey(key.PrivateKey)
		kSigners = append(kSigners, signer)

	}

	// args[0] is the
	wd.Entry, err = prepareData(args, false, kSigners)
	if err != nil {
		return PrintJsonRpcError(err)
	}
	res, resps, err := dispatchTxAndWait(&wd, u, signers)
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
	copy(lde.AccountId[:], accountId)
	data := wd.Entry.GetData()
	if len(data) > 0 {
		lde.Data = data[0]
		lde.ExtIds = data[1:]
	}

	ar := ActionResponseFromLiteData(res, wd.Recipient.String(), lde.AccountId[:], wd.Entry.Hash())
	ar.Flow = resps
	return ar.Print()
}
