package genesis

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"path"
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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

type InitOpts struct {
	Network             config.Network
	Configs             []*config.Config
	Validators          []tmtypes.GenesisValidator
	NetworkValidatorMap NetworkValidatorMap
	GenesisTime         time.Time
	Logger              log.Logger
	Router              routing.Router
	FactomAddressesFile string
}

type DataRecord struct {
	Account *protocol.DataAccount
	Entry   protocol.DataEntry
}

type NetworkValidatorMap map[string][]tmtypes.GenesisValidator

type genesis struct {
	opts         InitOpts
	kvdb         storage.KeyValueStore
	db           *database.Database
	block        *block.Block
	adiUrl       *url.URL
	authorityUrl *url.URL
	urls         []*url.URL
	records      []protocol.Account
	dataRecords  []DataRecord
	genesisExec  *block.Executor
}

type Genesis interface {
	Execute() error
	Discard()
	GetDBState() ([]byte, error)
}

func Init(kvdb storage.KeyValueStore, opts InitOpts) (Genesis, error) {
	g := &genesis{
		kvdb: kvdb,
		opts: opts,
		db:   database.New(kvdb, opts.Logger.With("module", "database")),
	}

	// Add validator keys to NetworkValidatorMap when not there
	if g.opts.NetworkValidatorMap == nil {
		panic("NetworkValidatorMap is not present")
	}
	if _, ok := g.opts.NetworkValidatorMap[g.opts.Network.LocalSubnetID]; !ok {
		g.opts.NetworkValidatorMap[g.opts.Network.LocalSubnetID] = g.opts.Validators
	}

	exec, err := block.NewGenesisExecutor(g.db, opts.Logger, opts.Network, opts.Router)
	if err != nil {
		return nil, err
	}
	g.genesisExec = exec

	g.block = new(block.Block)
	g.block.Index = protocol.GenesisBlock
	g.block.Time = opts.GenesisTime
	g.block.Batch = g.db.Begin(true)

	return g, nil
}

func (g *genesis) Execute() error {
	err := g.genesisExec.Genesis(g.block, func(st *chain.StateManager) error {
		g.adiUrl = g.opts.Network.NodeUrl()
		g.authorityUrl = g.adiUrl.JoinPath(protocol.OperatorBook)

		g.createADI()
		g.createValidatorBook()

		// set the initial price to 1/5 fct price * 1/4 market cap dilution = 1/20 fct price
		// for this exercise, we'll assume that 1 FCT = $1, so initial ACME price is $0.05
		oraclePrice := uint64(protocol.InitialAcmeOracleValue)

		g.createMainLedger(oraclePrice)
		g.createSyntheticLedger()
		g.createAnchorPool()

		err := g.createVoteScratchChain()
		if err != nil {
			return err
		}

		g.createEvidenceChain()

		err = g.createGlobals()
		if err != nil {
			return err
		}

		switch g.opts.Network.Type {
		case config.Directory:
			err = g.initDN(oraclePrice)
			if err != nil {
				return err
			}
			err = g.generateNetworkDefinition()
			if err != nil {
				return err
			}
		case config.BlockValidator:
			err = g.initBVN()
			if err != nil {
				return err
			}
		}

		err = st.Create(g.records...)
		if err != nil {
			return fmt.Errorf("failed to create records: %w", err)
		}

		for _, wd := range g.dataRecords {
			st.UpdateData(wd.Account, wd.Entry.Hash(), wd.Entry)
		}
		return st.AddDirectoryEntry(g.adiUrl, g.urls...)
	})
	if err != nil {
		return err
	}

	err = g.block.Batch.Commit()
	if err != nil {
		return err
	}

	batch := g.db.Begin(false)
	defer batch.Discard()
	err = g.writeGenesisFile(batch.BptRoot())
	if err != nil {
		return err
	}
	return nil
}

func (g *genesis) GetDBState() ([]byte, error) {
	memDb, ok := g.kvdb.(*memory.DB)

	var state []byte
	var err error
	if ok {
		state, err = memDb.MarshalJSON()
		if err != nil {
			return nil, nil
		}
	}

	return state, err
}

func (g *genesis) createADI() {
	// Create the ADI
	adi := new(protocol.ADI)
	adi.Url = g.adiUrl
	adi.AddAuthority(g.authorityUrl)
	g.WriteRecords(adi)
}

func (g *genesis) createValidatorBook() {
	book := new(protocol.KeyBook)
	book.Url = g.adiUrl.JoinPath(protocol.ValidatorBook)
	book.BookType = protocol.BookTypeValidator
	book.AddAuthority(g.authorityUrl)
	book.PageCount = 2

	page1 := new(protocol.KeyPage)
	page1.Url = protocol.FormatKeyPageUrl(book.Url, 0)
	page1.AcceptThreshold = protocol.GetMOfN(len(g.opts.Validators), protocol.FallbackValidatorThreshold)
	page1.Version = 1
	page1.Keys = make([]*protocol.KeySpec, 1)
	spec := new(protocol.KeySpec)
	spec.Owner = g.authorityUrl
	page1.Keys[0] = spec

	page2 := createOperatorPage(book.Url, 1, g.opts.Validators, false)
	blacklistTxsForPage(page2, protocol.TransactionTypeUpdateKeyPage, protocol.TransactionTypeUpdateAccountAuth)

	g.WriteRecords(book, page1, page2)
}

func (g *genesis) createMainLedger(oraclePrice uint64) {
	// Create the main ledger
	ledger := new(protocol.InternalLedger)
	ledger.Url = g.adiUrl.JoinPath(protocol.Ledger)
	ledger.ActiveOracle = oraclePrice
	ledger.PendingOracle = oraclePrice
	ledger.Index = protocol.GenesisBlock
	g.WriteRecords(ledger)
}

func (g *genesis) createSyntheticLedger() {
	// Create the synth ledger
	synthLedger := new(protocol.SyntheticLedger)
	synthLedger.Url = g.adiUrl.JoinPath(protocol.Synthetic)
	g.WriteRecords(synthLedger)
}

func (g *genesis) createAnchorPool() {
	// Create the anchor pool
	anchors := new(protocol.Anchor)
	anchors.Url = g.adiUrl.JoinPath(protocol.AnchorPool)
	anchors.AddAuthority(g.authorityUrl)
	g.WriteRecords(anchors)

}

func (g *genesis) createVoteScratchChain() error {
	//create a vote scratch chain
	wd := new(protocol.WriteData)
	lci := types.LastCommitInfo{}
	data, err := json.Marshal(&lci)
	if err != nil {
		return err
	}
	wd.Entry = &protocol.AccumulateDataEntry{Data: [][]byte{data}}

	da := new(protocol.DataAccount)
	da.Scratch = true
	da.Url = g.adiUrl.JoinPath(protocol.Votes)
	da.AddAuthority(g.authorityUrl)
	g.writeDataRecord(da, da.Url, DataRecord{da, wd.Entry})
	return nil
}

func (g *genesis) createEvidenceChain() {
	//create an evidence scratch chain
	da := new(protocol.DataAccount)
	da.Scratch = true
	da.Url = g.adiUrl.JoinPath(protocol.Evidence)
	da.AddAuthority(g.authorityUrl)
	g.WriteRecords(da)
	g.urls = append(g.urls, da.Url)
}

func (g *genesis) createGlobals() error {
	//create a new Globals account
	global := new(protocol.DataAccount)
	global.Url = g.adiUrl.JoinPath(protocol.Globals)
	wg := new(protocol.WriteData)
	threshold := new(protocol.NetworkGlobals)
	threshold.OperatorAcceptThreshold.Numerator = 2
	threshold.OperatorAcceptThreshold.Denominator = 3
	data, err := threshold.MarshalBinary()
	if err != nil {
		return err
	}
	wg.Entry = &protocol.AccumulateDataEntry{Data: [][]byte{data}}
	global.AddAuthority(g.authorityUrl)
	g.writeDataRecord(global, global.Url, DataRecord{global, wg.Entry})
	return nil
}

func (g *genesis) initDN(oraclePrice uint64) error {
	g.createDNOperatorBook()

	oracle := new(protocol.AcmeOracle)
	oracle.Price = oraclePrice
	wd := new(protocol.WriteData)
	data, err := json.Marshal(&oracle)
	if err != nil {
		return err
	}
	wd.Entry = &protocol.AccumulateDataEntry{Data: [][]byte{data}}
	daOracle := new(protocol.DataAccount)
	daOracle.Url = g.adiUrl.JoinPath(protocol.Oracle)
	daOracle.AddAuthority(g.authorityUrl)
	g.writeDataRecord(daOracle, daOracle.Url, DataRecord{daOracle, wd.Entry})

	acme := new(protocol.TokenIssuer)
	acme.AddAuthority(g.authorityUrl)
	acme.Url = protocol.AcmeUrl()
	acme.Precision = 8
	acme.Symbol = "ACME"
	g.WriteRecords(acme)

	if protocol.IsTestNet {
		// On the TestNet, set the issued amount to the faucet balance
		acme.Issued.SetString(protocol.AcmeFaucetBalance, 10)
	} else {
		// On the MainNet, set the supply limit
		acme.SupplyLimit = big.NewInt(protocol.AcmeSupplyLimit * protocol.AcmePrecision)
	}
	return nil
}

func (g *genesis) initBVN() error {
	// Test with `${ID}` not `bvn-${ID}` because the latter will fail
	// with "bvn-${ID} is reserved"
	network := g.opts.Network
	if err := protocol.IsValidAdiUrl(&url.URL{Authority: network.LocalSubnetID}); err != nil {
		panic(fmt.Errorf("%q is not a valid subnet ID: %v", network.LocalSubnetID, err))
	}

	g.createBVNOperatorBook()

	subnet, err := routing.RouteAccount(&network, protocol.FaucetUrl)
	if err == nil && subnet == network.LocalSubnetID {
		liteId := new(protocol.LiteIdentity)
		liteId.Url = protocol.FaucetUrl.RootIdentity()

		liteToken := new(protocol.LiteTokenAccount)
		liteToken.Url = protocol.FaucetUrl
		liteToken.TokenUrl = protocol.AcmeUrl()
		liteToken.Balance.SetString(protocol.AcmeFaucetBalance, 10)
		g.WriteRecords(liteId, liteToken)
	}
	if g.opts.FactomAddressesFile != "" {
		factomAddresses, err := LoadFactomAddressesAndBalances(g.opts.FactomAddressesFile)
		if err != nil {
			return err
		}
		for _, factomAddress := range factomAddresses {
			subnet, err := routing.RouteAccount(&network, factomAddress.Address)
			if err == nil && subnet == network.LocalSubnetID {
				lite := new(protocol.LiteTokenAccount)
				lite.Url = factomAddress.Address
				lite.TokenUrl = protocol.AcmeUrl()
				lite.Balance = *big.NewInt(5 * factomAddress.Balance)
				g.WriteRecords(lite)
			}
		}
	}
	return nil
}

func (g *genesis) createDNOperatorBook() {
	book := new(protocol.KeyBook)
	book.Url = g.adiUrl.JoinPath(protocol.OperatorBook)
	book.BookType = protocol.BookTypeOperator
	book.AddAuthority(book.Url)
	book.PageCount = 1

	page := createOperatorPage(book.Url, 0, g.opts.Validators, false)
	g.WriteRecords(book, page)
}

func (g *genesis) createBVNOperatorBook() {
	book := new(protocol.KeyBook)
	book.Url = g.adiUrl.JoinPath(protocol.OperatorBook)
	book.BookType = protocol.BookTypeOperator
	book.AddAuthority(book.Url)
	book.PageCount = 2

	page1 := new(protocol.KeyPage)
	page1.Url = protocol.FormatKeyPageUrl(book.Url, 0)
	page1.AcceptThreshold = protocol.GetMOfN(len(g.opts.Validators), protocol.FallbackValidatorThreshold)
	page1.Version = 1
	page1.Keys = make([]*protocol.KeySpec, 1)
	spec := new(protocol.KeySpec)
	spec.Owner = protocol.DnUrl().JoinPath(protocol.OperatorBook)
	page1.Keys[0] = spec

	page2 := createOperatorPage(book.Url, 1, g.opts.Validators, false)
	blacklistTxsForPage(page2, protocol.TransactionTypeUpdateKeyPage, protocol.TransactionTypeUpdateAccountAuth)
	g.WriteRecords(book, page1, page2)
}

func createOperatorPage(uBook *url.URL, pageIndex uint64, operators []tmtypes.GenesisValidator, validatorsOnly bool) *protocol.KeyPage {
	page := new(protocol.KeyPage)
	page.Url = protocol.FormatKeyPageUrl(uBook, pageIndex)
	page.AcceptThreshold = protocol.GetMOfN(len(operators), protocol.FallbackValidatorThreshold)
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

func (g *genesis) generateNetworkDefinition() error {
	if g.opts.Network.Type != config.Directory {
		return fmt.Errorf("generateNetworkDefinition is only allowed for DNs")
	}
	networkDefs := g.buildNetworkDefinition()
	wd := new(protocol.WriteData)
	data, err := json.Marshal(&networkDefs)
	if err != nil {
		return err
	}
	wd.Entry = &protocol.AccumulateDataEntry{Data: [][]byte{data}}

	da := new(protocol.DataAccount)
	da.Url = g.adiUrl.JoinPath(protocol.Network)
	da.AddAuthority(g.authorityUrl)
	g.writeDataRecord(da, da.Url, DataRecord{da, wd.Entry})
	return nil
}

func (g *genesis) WriteRecords(record ...protocol.Account) {
	g.records = append(g.records, record...)
	for _, rec := range record {
		g.urls = append(g.urls, rec.GetUrl())
	}
}

func (g *genesis) writeDataRecord(account *protocol.DataAccount, url *url.URL, dataRecord DataRecord) {
	g.records = append(g.records, account)
	g.urls = append(g.urls, url)
	g.dataRecords = append(g.dataRecords, dataRecord)
}

func (g *genesis) writeGenesisFile(appHash []byte) error {
	state, err := g.GetDBState()
	if err != nil {
		return err
	}

	genDoc := &tmtypes.GenesisDoc{
		ChainID:         g.opts.Network.LocalSubnetID,
		GenesisTime:     g.opts.GenesisTime,
		InitialHeight:   protocol.GenesisBlock + 1,
		Validators:      g.opts.Validators,
		ConsensusParams: tmtypes.DefaultConsensusParams(),
		AppState:        state,
		AppHash:         appHash,
	}

	for _, config := range g.opts.Configs {
		if err := genDoc.SaveAs(path.Join(config.RootDir, config.BaseConfig.Genesis)); err != nil {
			return fmt.Errorf("failed to save gen doc: %v", err)
		}
	}
	return nil
}

func (g *genesis) Discard() {
	g.block.Batch.Discard()
}

func (g *genesis) buildNetworkDefinition() *protocol.NetworkDefinition {
	netDef := new(protocol.NetworkDefinition)

	for _, subnet := range g.opts.Network.Subnets {

		// Add the validator hashes from the subnet's genesis doc
		var vkHashes [][32]byte
		for _, validator := range g.opts.NetworkValidatorMap[subnet.ID] {
			pkh := sha256.Sum256(validator.PubKey.Bytes())
			vkHashes = append(vkHashes, pkh)
		}

		subnetDef := protocol.SubnetDefinition{
			SubnetID:           subnet.ID,
			ValidatorKeyHashes: vkHashes,
		}
		netDef.Subnets = append(netDef.Subnets, subnetDef)
	}
	return netDef
}
