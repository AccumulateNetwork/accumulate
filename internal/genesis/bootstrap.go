package genesis

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type InitOpts struct {
	Network             config.Network
	Validators          []tmtypes.GenesisValidator
	GenesisTime         time.Time
	Logger              log.Logger
	Router              routing.Router
	FactomAddressesFile string
	Keys                [][]byte
}

func Init(kvdb storage.KeyValueStore, opts InitOpts) ([]byte, error) {
	db := database.New(kvdb, opts.Logger.With("module", "database"))

	exec, err := block.NewGenesisExecutor(db, opts.Logger, opts.Network, opts.Router)
	if err != nil {
		return nil, err
	}

	block := new(block.Block)
	block.Index = protocol.GenesisBlock
	block.Time = opts.GenesisTime
	block.Batch = db.Begin(true)
	defer block.Batch.Discard()

	err = exec.Genesis(block, bootstrap{opts})
	if err != nil {
		return nil, err
	}

	err = block.Batch.Commit()
	if err != nil {
		return nil, err
	}

	batch := db.Begin(false)
	defer batch.Discard()
	return batch.BptRoot(), nil
}

type bootstrap struct{ InitOpts }

var _ chain.TransactionExecutor = bootstrap{}
var _ chain.PrincipalValidator = bootstrap{}

func (bootstrap) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticDepositTokens
}

func (b bootstrap) AllowMissingPrincipal(*protocol.Transaction) (allow, fallback bool) {
	return true, false
}

func (b bootstrap) Execute(st *chain.StateManager, tx *chain.Delivery) (protocol.TransactionResult, error) {
	return b.Validate(st, tx)
}

func (b bootstrap) Validate(st *chain.StateManager, tx *chain.Delivery) (protocol.TransactionResult, error) {
	var records []protocol.Account

	// Create the ADI
	uAdi := b.Network.NodeUrl()
	uVal := uAdi.JoinPath(protocol.ValidatorBook)
	// uOper := uAdi.JoinPath(protocol.OperatorBook)

	adi := new(protocol.ADI)
	adi.Url = uAdi
	adi.AddAuthority(uVal)
	records = append(records, adi)

	valBook, valPage := createValidatorBook(uVal, b.Validators)
	records = append(records, valBook, valPage)

	// set the initial price to 1/5 fct price * 1/4 market cap dilution = 1/20 fct price
	// for this exercise, we'll assume that 1 FCT = $1, so initial ACME price is $0.05
	oraclePrice := uint64(protocol.InitialAcmeOracleValue)

	// Create the main ledger
	ledger := new(protocol.InternalLedger)
	ledger.Url = uAdi.JoinPath(protocol.Ledger)
	ledger.ActiveOracle = oraclePrice
	ledger.PendingOracle = oraclePrice
	ledger.Index = protocol.GenesisBlock
	records = append(records, ledger)

	// Create the synth ledger
	synthLedger := new(protocol.SyntheticLedger)
	synthLedger.Url = uAdi.JoinPath(protocol.Synthetic)
	records = append(records, synthLedger)

	// Create the anchor pool
	anchors := new(protocol.Anchor)
	anchors.Url = uAdi.JoinPath(protocol.AnchorPool)
	anchors.AddAuthority(uVal)
	records = append(records, anchors)

	// Create records and directory entries
	urls := make([]*url.URL, len(records))
	for i, r := range records {
		urls[i] = r.GetUrl()
	}

	type DataRecord struct {
		Account *url.URL
		Entry   protocol.DataEntry
	}
	var dataRecords []DataRecord

	//create a vote scratch chain
	wd := new(protocol.WriteData)
	lci := types.LastCommitInfo{}
	data, err := json.Marshal(&lci)
	if err != nil {
		return nil, err
	}
	wd.Entry = &protocol.AccumulateDataEntry{Data: [][]byte{data}}

	da := new(protocol.DataAccount)
	da.Scratch = true
	da.Url = uAdi.JoinPath(protocol.Votes)
	da.AddAuthority(uVal)

	records = append(records, da)
	urls = append(urls, da.Url)
	dataRecords = append(dataRecords, DataRecord{da.Url, wd.Entry})

	//create an evidence scratch chain
	da = new(protocol.DataAccount)
	da.Scratch = true
	da.Url = uAdi.JoinPath(protocol.Evidence)
	da.AddAuthority(uVal)

	records = append(records, da)
	urls = append(urls, da.Url)

	//create a new Globals account
	global := new(protocol.DataAccount)
	global.Url = uAdi.JoinPath(protocol.Globals)
	wg := new(protocol.WriteData)
	threshold := new(protocol.NetworkGlobals)
	threshold.ValidatorThreshold.Numerator = 2
	threshold.ValidatorThreshold.Denominator = 3
	data, err = threshold.MarshalBinary()
	if err != nil {
		return nil, err
	}
	wg.Entry = &protocol.AccumulateDataEntry{Data: [][]byte{data}}
	global.AddAuthority(uVal)
	records = append(records, global)
	urls = append(urls, global.Url)
	dataRecords = append(dataRecords, DataRecord{global.Url, wg.Entry})

	// var uOperPage *url.URL
	switch b.Network.Type {
	case config.Directory:
		// uOperPage = protocol.FormatKeyPageUrl(uOper, 0)

		operBook, page1 := createDNOperatorBook(b.Network.NodeUrl(), b.Validators)
		records = append(records, operBook, page1)

		oracle := new(protocol.AcmeOracle)
		oracle.Price = oraclePrice
		wd := new(protocol.WriteData)
		data, err = json.Marshal(&oracle)
		if err != nil {
			return nil, err
		}
		wd.Entry = &protocol.AccumulateDataEntry{Data: [][]byte{data}}
		da := new(protocol.DataAccount)
		da.Url = uAdi.JoinPath(protocol.Oracle)
		da.AddAuthority(uVal)

		records = append(records, da)
		urls = append(urls, da.Url)
		dataRecords = append(dataRecords, DataRecord{da.Url, wd.Entry})

		acme := new(protocol.TokenIssuer)
		acme.AddAuthority(uVal)
		acme.Url = protocol.AcmeUrl()
		acme.Precision = 8
		acme.Symbol = "ACME"
		records = append(records, acme)

		if protocol.IsTestNet {
			// On the TestNet, set the issued amount to the faucet balance
			acme.Issued.SetString(protocol.AcmeFaucetBalance, 10)
		} else {
			// On the MainNet, set the supply limit
			acme.SupplyLimit = big.NewInt(protocol.AcmeSupplyLimit * protocol.AcmePrecision)
		}

	case config.BlockValidator:
		// uOperPage = protocol.FormatKeyPageUrl(uOper, 1)

		// Test with `${ID}` not `bvn-${ID}` because the latter will fail
		// with "bvn-${ID} is reserved"
		if err := protocol.IsValidAdiUrl(&url.URL{Authority: b.Network.LocalSubnetID}); err != nil {
			panic(fmt.Errorf("%q is not a valid subnet ID: %v", b.Network.LocalSubnetID, err))
		}

		operBook, page1, page2 := createBVNOperatorBook(b.Network.NodeUrl(), b.Validators)
		records = append(records, operBook, page1, page2)

		subnet, err := routing.RouteAccount(&b.Network, protocol.FaucetUrl)
		if err == nil && subnet == b.Network.LocalSubnetID {
			liteId := new(protocol.LiteIdentity)
			liteId.Url = protocol.FaucetUrl.RootIdentity()

			liteToken := new(protocol.LiteTokenAccount)
			liteToken.Url = protocol.FaucetUrl
			liteToken.TokenUrl = protocol.AcmeUrl()
			liteToken.Balance.SetString(protocol.AcmeFaucetBalance, 10)
			records = append(records, liteId, liteToken)
		}
		if b.FactomAddressesFile != "" {
			factomAddresses, err := LoadFactomAddressesAndBalances(b.FactomAddressesFile)
			if err != nil {
				return nil, err
			}
			for _, factomAddress := range factomAddresses {
				subnet, err := routing.RouteAccount(&b.Network, factomAddress.Address)
				if err == nil && subnet == b.Network.LocalSubnetID {
					lite := new(protocol.LiteTokenAccount)
					lite.Url = factomAddress.Address
					lite.TokenUrl = protocol.AcmeUrl()
					lite.Balance = *big.NewInt(5 * factomAddress.Balance)
					records = append(records, lite)
				}
			}
		}
	}

	err = st.Create(records...)
	if err != nil {
		return nil, fmt.Errorf("failed to create records: %w", err)
	}

	var timestamp uint64
	for _, wd := range dataRecords {
		body := new(protocol.WriteData)
		body.Entry = wd.Entry
		txn := new(protocol.Transaction)
		txn.Header.Principal = wd.Account
		txn.Body = body
		// sigs, err := b.sign(txn, uOperPage, &timestamp)
		sigs, err := b.sign(txn, protocol.FormatKeyPageUrl(uVal, 0), &timestamp)
		if err != nil {
			return nil, err
		}
		st.State.ProcessAdditionalTransaction(tx.NewChild(txn, sigs))
	}

	return nil, st.AddDirectoryEntry(adi.Url, urls...)
}

func (b bootstrap) sign(txn *protocol.Transaction, signer *url.URL, timestamp *uint64) ([]protocol.Signature, error) {
	sig, err := new(signing.Builder).
		UseSimpleHash().
		SetUrl(signer).
		SetVersion(1).
		SetPrivateKey(b.Keys[0]).
		SetType(protocol.SignatureTypeED25519).
		SetTimestampWithVar(timestamp).
		Initiate(txn)
	if err != nil {
		return nil, err
	}

	sigs := []protocol.Signature{sig}
	for _, key := range b.Keys[1:] {
		sig, err := new(signing.Builder).
			UseSimpleHash().
			SetUrl(signer).
			SetVersion(1).
			SetPrivateKey(key).
			SetType(protocol.SignatureTypeED25519).
			SetTimestampWithVar(timestamp).
			Sign(txn.GetHash())
		if err != nil {
			return nil, err
		}
		sigs = append(sigs, sig)
	}

	return sigs, nil
}

func createValidatorBook(uBook *url.URL, operators []tmtypes.GenesisValidator) (*protocol.KeyBook, *protocol.KeyPage) {
	book := new(protocol.KeyBook)
	book.Url = uBook
	book.BookType = protocol.BookTypeValidator
	book.AddAuthority(uBook)
	book.PageCount = 1

	return book, createOperatorPage(uBook, 0, operators, true)
}

func createDNOperatorBook(nodeUrl *url.URL, operators []tmtypes.GenesisValidator) (*protocol.KeyBook, *protocol.KeyPage) {
	book := new(protocol.KeyBook)
	book.Url = nodeUrl.JoinPath(protocol.OperatorBook)
	book.AddAuthority(book.Url)
	book.PageCount = 1

	return book, createOperatorPage(book.Url, 0, operators, false)
}

func createBVNOperatorBook(nodeUrl *url.URL, operators []tmtypes.GenesisValidator) (*protocol.KeyBook, *protocol.KeyPage, *protocol.KeyPage) {
	book := new(protocol.KeyBook)
	book.Url = nodeUrl.JoinPath(protocol.OperatorBook)
	book.AddAuthority(book.Url)
	book.PageCount = 2

	page1 := new(protocol.KeyPage)
	page1.Url = protocol.FormatKeyPageUrl(book.Url, 0)
	page1.AcceptThreshold = protocol.GetValidatorsMOfN(len(operators), protocol.FallbackValidatorThreshold)
	page1.Version = 1
	page1.Keys = make([]*protocol.KeySpec, 1)
	spec := new(protocol.KeySpec)
	spec.Owner = protocol.DnUrl().JoinPath(protocol.OperatorBook)
	page1.Keys[0] = spec

	page2 := createOperatorPage(book.Url, 1, operators, false)
	blacklistTxsForPage(page2, protocol.TransactionTypeUpdateKeyPage, protocol.TransactionTypeUpdateAccountAuth)

	return book, page1, page2
}

func createOperatorPage(uBook *url.URL, pageIndex uint64, operators []tmtypes.GenesisValidator, validatorsOnly bool) *protocol.KeyPage {
	page := new(protocol.KeyPage)
	page.Url = protocol.FormatKeyPageUrl(uBook, pageIndex)
	page.AcceptThreshold = protocol.GetValidatorsMOfN(len(operators), protocol.FallbackValidatorThreshold)
	page.Version = 1

	for _, operator := range operators {
		/* TODO
		Determine which operators are also validators and which not. Followers should be omitted,
		but DNs which also don't have voting power not.	(DNs need to sign Oracle updates)
		*/
		isValidator := true
		if isValidator || !validatorsOnly {
			spec := new(protocol.KeySpec)
			kh := sha256.Sum256(operator.PubKey.Bytes())
			spec.PublicKeyHash = kh[:]
			page.Keys = append(page.Keys, spec)
		}
	}
	return page
}

func blacklistTxsForPage(page *protocol.KeyPage, txTypes ...protocol.TransactionType) {
	page.TransactionBlacklist = new(protocol.AllowedTransactions)
	for _, txType := range txTypes {
		bit, ok := txType.AllowedTransactionBit()
		if !ok {
			panic(fmt.Errorf("failed to blacklist %v", txType))
		}
		page.TransactionBlacklist.Set(bit)
	}
}
