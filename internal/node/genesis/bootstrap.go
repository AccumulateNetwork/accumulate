// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/cometbft/cometbft/abci/types"
	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/libs/log"
	tmtypes "github.com/cometbft/cometbft/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/sync/errgroup"
)

type InitOpts struct {
	Logger log.Logger

	NetworkID      string
	PartitionId    string
	NetworkType    protocol.PartitionType
	GenesisTime    time.Time
	GenesisGlobals *core.GlobalValues
	OperatorKeys   [][]byte

	// For Tendermint
	ConsensusParams *tmtypes.ConsensusParams

	// Preloaded data
	FactomAddresses func() (io.Reader, error)
	Snapshots       []func(*core.GlobalValues) (ioutil2.SectionReader, error)

	// Flags
	IncludeHistoryFromSnapshots bool
}

func Init(snapshotWriter io.WriteSeeker, opts InitOpts) error {
	// Initialize globals
	gg := core.NewGlobals(opts.GenesisGlobals)
	opts.GenesisGlobals = gg

	// Build the routing table
	var bvns []string
	for _, partition := range gg.Network.Partitions {
		if partition.Type == protocol.PartitionTypeBlockValidator {
			bvns = append(bvns, partition.ID)
		}
	}
	if gg.Routing == nil {
		gg.Routing = new(protocol.RoutingTable)
	}
	if gg.Routing.Routes == nil {
		gg.Routing.Routes = routing.BuildSimpleTable(bvns)
	}
	gg.Routing.AddOverride(protocol.AcmeUrl(), protocol.Directory)
	for _, partition := range gg.Network.Partitions {
		if partition.Type == protocol.PartitionTypeBlockSummary {
			continue
		}
		gg.Routing.AddOverride(protocol.PartitionUrl(partition.ID), partition.ID)
	}

	store := memory.New(nil)
	b := &bootstrap{
		InitOpts:    opts,
		kvdb:        store,
		db:          coredb.New(store, opts.Logger.With("module", "database")),
		dataRecords: make([]DataRecord, 0),
		records:     make([]protocol.Account, 0),
		acmeIssued:  new(big.Int),
		partition:   config.NetworkUrl{URL: protocol.PartitionUrl(opts.PartitionId)},
	}
	b.db.SetObserver(execute.NewDatabaseObserver())

	// Create the router
	b.router = routing.NewRouter(routing.RouterOptions{Initial: gg.Routing, Logger: b.Logger})

	// Unpack snapshots
	err := b.unpackSnapshots()
	if err != nil {
		return errors.UnknownError.WithFormat("unpack snapshots: %w", err)
	}

	exec, err := block.NewGenesisExecutor(b.db, opts.Logger, &config.Describe{
		NetworkType: opts.NetworkType,
		PartitionId: opts.PartitionId,
	}, gg, b.router)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Capture background tasks
	errg := new(errgroup.Group)
	exec.BackgroundTaskLauncher = func(f func()) { errg.Go(func() error { f(); return nil }) }

	b.block = new(block.Block)
	b.block.Index = protocol.GenesisBlock

	b.block.Time = b.GenesisTime
	b.block.Batch = b.db.Begin(true)
	defer b.block.Batch.Discard()

	err = exec.Genesis(b.block, b)
	if err != nil {
		return errors.UnknownError.WithFormat("execute: %w", err)
	}

	err = b.block.Batch.Commit()
	if err != nil {
		return errors.UnknownError.WithFormat("commit: %w", err)
	}

	// Wait for background tasks
	err = errg.Wait()
	if err != nil {
		return errors.UnknownError.WithFormat("tasks: %w", err)
	}

	// Create the snapshot
	_, err = b.db.Collect(snapshotWriter, b.partition.URL, &coredb.CollectOptions{
		DidWriteHeader: func(w *snapshot.Writer) error {
			// Convert the consensus parameters
			doc := new(cometbft.GenesisDoc)
			doc.ChainID = opts.NetworkID + "." + opts.PartitionId
			doc.Params = (*cometbft.ConsensusParams)(opts.ConsensusParams)
			for _, v := range opts.GenesisGlobals.Network.Validators {
				if !v.IsActiveOn(opts.PartitionId) {
					continue
				}

				var name string
				if v.Operator == nil {
					name = fmt.Sprintf("Validator-%x", v.PublicKeyHash[:4])
				} else {
					name = v.Operator.ShortString()
				}

				key := tmed25519.PubKey(v.PublicKey)
				doc.Validators = append(doc.Validators, &cometbft.Validator{
					Address: key.Address(),
					PubKey:  key,
					Type:    protocol.SignatureTypeED25519,
					Power:   1,
					Name:    name,
				})
			}

			// Write it
			b, err := doc.MarshalBinary()
			if err != nil {
				return err
			}
			sw, err := w.OpenRaw(snapshot.SectionTypeConsensus)
			if err != nil {
				return err
			}
			_, err = sw.Write(b)
			if err != nil {
				return err
			}
			return sw.Close()
		},
	})
	if err != nil {
		return errors.UnknownError.WithFormat("collect: %w", err)
	}
	return nil
}

type bootstrap struct {
	InitOpts
	networkAuthority *url.URL
	partition        config.NetworkUrl
	localAuthority   *url.URL

	kvdb        keyvalue.Beginner
	db          *coredb.Database
	block       *block.Block
	urls        []*url.URL
	records     []protocol.Account
	dataRecords []DataRecord
	router      routing.Router

	accountsFromSnapshots []*url.URL
	acmeIssued            *big.Int
}

type DataRecord struct {
	Account *url.URL
	Entry   protocol.DataEntry
}

var _ chain.TransactionExecutor = &bootstrap{}
var _ chain.PrincipalValidator = &bootstrap{}

func (bootstrap) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticDepositTokens
}

func (b *bootstrap) AllowMissingPrincipal(*protocol.Transaction) bool {
	return true
}

func (b *bootstrap) Execute(st *chain.StateManager, tx *chain.Delivery) (protocol.TransactionResult, error) {
	return b.Validate(st, tx)
}

func (b *bootstrap) Validate(st *chain.StateManager, tx *chain.Delivery) (protocol.TransactionResult, error) {
	b.networkAuthority = protocol.DnUrl().JoinPath(protocol.Operators)
	b.localAuthority = b.partition.Operators()

	// Verify that the BVN ID will make a valid partition URL
	if err := protocol.IsValidAdiUrl(protocol.PartitionUrl(b.PartitionId), true); err != nil {
		panic(fmt.Errorf("%q is not a valid partition ID: %v", b.PartitionId, err))
	}

	// Create network variable accounts
	err := b.GenesisGlobals.InitializeDataAccounts(b.partition.URL, func(accountUrl *url.URL, target interface{}) error {
		da := new(protocol.DataAccount)
		da.Url = accountUrl
		da.AddAuthority(b.networkAuthority)
		return encoding.SetPtr(da, target)
	}, func(account protocol.Account) error {
		b.WriteRecords(account)
		return nil
	})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Create accounts
	err = b.addGenesisToSnapshotAccounts(st)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	err = b.maybeCreateFactomAccounts()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	b.createIdentity()
	b.createMainLedger()
	b.createSyntheticLedger()
	b.createAnchorPool()
	b.createOperatorBook()
	b.createEvidenceChain()
	b.maybeCreateFaucet()
	b.maybeCreateAcme()

	err = b.createVoteScratchChain()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Persist accounts
	err = st.Create(b.records...)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store records: %w", err)
	}

	// Update the directory index
	err = st.AddDirectoryEntry(b.partition.Identity(), b.urls...)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Write data entries
	for _, wd := range b.dataRecords {
		body := new(protocol.SystemWriteData)
		body.Entry = wd.Entry
		txn := new(protocol.Transaction)
		txn.Header.Principal = wd.Account
		txn.Body = body
		st.State.ProcessAdditionalTransaction(tx.NewInternal(txn))
	}

	return nil, nil
}

func (b *bootstrap) createIdentity() {
	// Create the ADI
	adi := new(protocol.ADI)
	adi.Url = b.partition.Identity()
	adi.AddAuthority(b.localAuthority)
	b.WriteRecords(adi)
}

func (b *bootstrap) createMainLedger() {
	// Create the main ledger
	ledger := new(protocol.SystemLedger)
	ledger.Url = b.partition.Ledger()
	ledger.Index = protocol.GenesisBlock
	ledger.Timestamp = b.InitOpts.GenesisTime
	ledger.ExecutorVersion = b.GenesisGlobals.ExecutorVersion
	if b.GenesisGlobals.ExecutorVersion.V2VandenbergEnabled() {
		for _, bvn := range b.GenesisGlobals.BvnNames() {
			ledger.SetBvnExecutorVersion(bvn, b.GenesisGlobals.ExecutorVersion)
		}
	}
	b.WriteRecords(ledger)
}

func (b *bootstrap) createSyntheticLedger() {
	// Create the synth ledger
	synthLedger := new(protocol.SyntheticLedger)
	synthLedger.Url = b.partition.Synthetic()
	b.WriteRecords(synthLedger)
}

func (b *bootstrap) createAnchorPool() {
	// Create the anchor pool
	anchorLedger := new(protocol.AnchorLedger)
	anchorLedger.Url = b.partition.AnchorPool()

	if b.NetworkType == protocol.PartitionTypeDirectory {
		// Initialize the last major block time to prevent a major block from
		// being created immediately once the network boots
		anchorLedger.MajorBlockTime = b.GenesisTime
	}

	b.WriteRecords(anchorLedger)
}

func (b *bootstrap) createVoteScratchChain() error {
	//create a vote scratch chain
	wd := new(protocol.WriteData)
	lci := types.CommitInfo{}
	data, err := json.Marshal(&lci)
	if err != nil {
		return errors.InternalError.WithFormat("marshal last commit info: %w", err)
	}
	if b.GenesisGlobals.ExecutorVersion.DoubleHashEntriesEnabled() {
		wd.Entry = &protocol.DoubleHashDataEntry{Data: [][]byte{data}}
	} else {
		wd.Entry = &protocol.AccumulateDataEntry{Data: [][]byte{data}}
	}
	wd.Scratch = true

	da := new(protocol.DataAccount)
	da.Url = b.partition.URL.JoinPath(protocol.Votes)
	da.AddAuthority(b.localAuthority)
	b.writeDataRecord(da, da.Url, DataRecord{da.Url, wd.Entry})
	return nil
}

func (b *bootstrap) createEvidenceChain() {
	//create an evidence chain
	da := new(protocol.DataAccount)
	da.Url = b.partition.JoinPath(protocol.Evidence)
	da.AddAuthority(b.localAuthority)
	b.WriteRecords(da)
	b.urls = append(b.urls, da.Url)
}

func (b *bootstrap) maybeCreateAcme() {
	if !b.shouldCreate(protocol.AcmeUrl()) {
		return
	}

	acme := new(protocol.TokenIssuer)
	acme.AddAuthority(b.networkAuthority)
	acme.Url = protocol.AcmeUrl()
	acme.Precision = 8
	acme.Symbol = "ACME"
	acme.Issued = *b.acmeIssued
	acme.SupplyLimit = big.NewInt(protocol.AcmeSupplyLimit * protocol.AcmePrecision)
	b.WriteRecords(acme)
}

func (b *bootstrap) maybeCreateFaucet() {
	if !protocol.IsTestNet {
		return
	}

	// Always do this so the DN has the proper issued amount
	amount := new(big.Int)
	amount.SetUint64(protocol.AcmeFaucetBalance * protocol.AcmePrecision)
	b.acmeIssued.Add(b.acmeIssued, amount)

	if !b.shouldCreate(protocol.FaucetUrl) {
		return
	}

	liteId := new(protocol.LiteIdentity)
	liteId.Url = protocol.FaucetUrl.RootIdentity()

	liteToken := new(protocol.LiteTokenAccount)
	liteToken.Url = protocol.FaucetUrl
	liteToken.TokenUrl = protocol.AcmeUrl()
	liteToken.Balance = *amount

	// Lock forever
	liteToken.LockHeight = math.MaxUint64

	b.WriteRecords(liteId, liteToken)
}

func (b *bootstrap) maybeCreateFactomAccounts() error {
	if b.FactomAddresses == nil {
		return nil
	}

	rd, err := b.FactomAddresses()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if c, ok := rd.(io.Closer); ok {
		defer c.Close()
	}

	factomAddresses, err := LoadFactomAddressesAndBalances(rd)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	for _, fa := range factomAddresses {
		if !b.shouldCreate(fa.Address) {
			continue
		}

		lid := new(protocol.LiteIdentity)
		lid.Url = fa.Address.RootIdentity()

		lite := new(protocol.LiteTokenAccount)
		lite.Url = fa.Address
		lite.TokenUrl = protocol.AcmeUrl()
		lite.Balance = *big.NewInt(5 * fa.Balance)

		b.acmeIssued.Add(b.acmeIssued, &lite.Balance)

		b.WriteRecords(lid, lite)
	}
	return nil
}

func (b *bootstrap) unpackSnapshots() error {
	// Nothing is routed to the DN, but the DN needs to know how much ACME has
	// been issued so this still needs to be run for the DN

	// Restore accounts
	for _, open := range b.Snapshots {
		file, err := open(b.GenesisGlobals)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		if c, ok := file.(io.Closer); ok {
			defer c.Close()
		}

		accounts, err := Extract(b.db, file, func(u *url.URL) bool {
			// Does the account belong to this partition?
			partition, err := b.router.RouteAccount(u)
			if err != nil {
				panic(errors.InternalError.WithFormat("route %v: %w", u, err))
			}
			return strings.EqualFold(partition, b.PartitionId)
		})
		if err != nil {
			return err
		}

		// Track ACME issued
		for _, account := range accounts {
			if acct, ok := account.Main.(protocol.AccountWithTokens); ok && protocol.AcmeUrl().Equal(acct.GetTokenUrl()) {
				b.acmeIssued.Add(b.acmeIssued, acct.TokenBalance())
			}
		}

		for _, a := range accounts {
			if a.Keep {
				b.accountsFromSnapshots = append(b.accountsFromSnapshots, a.Url)
			}
		}
	}

	return nil
}

func (b *bootstrap) addGenesisToSnapshotAccounts(st *chain.StateManager) error {
	// Add Genesis to the accounts' main chain. This also anchors the imported
	// transactions. This *must* be done outside of ImportFactomSnapshot.
	// Attempting this within the callback leads to a mismatch between the
	// account state and the hash in the snapshot.
	for _, account := range b.accountsFromSnapshots {
		chain := st.GetBatch().Account(account).MainChain()
		err := st.State.ChainUpdates.AddChainEntry(st.GetBatch(), chain, st.GetHash(), 0, 0)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	return nil
}

func (b *bootstrap) shouldCreate(url *url.URL) bool {
	partition, err := b.router.RouteAccount(url)
	if err != nil {
		panic(err) // An account should never be unroutable
	}

	return strings.EqualFold(b.PartitionId, partition)
}

func (b *bootstrap) createOperatorBook() {
	book := new(protocol.KeyBook)
	book.Url = b.partition.Operators()
	book.AddAuthority(b.networkAuthority)
	book.PageCount = 1

	page := new(protocol.KeyPage)
	page.Url = book.Url.JoinPath("1")
	page.Version = 1

	for _, operator := range b.OperatorKeys {
		spec := new(protocol.KeySpec)
		kh := sha256.Sum256(operator)
		spec.PublicKeyHash = kh[:]
		page.AddKeySpec(spec)
	}

	page.AcceptThreshold = b.GenesisGlobals.Globals.OperatorAcceptThreshold.Threshold(len(page.Keys))
	b.WriteRecords(book, page)
}

func (b *bootstrap) WriteRecords(record ...protocol.Account) {
	b.records = append(b.records, record...)
	for _, rec := range record {
		b.urls = append(b.urls, rec.GetUrl())
	}
}

func (b *bootstrap) writeDataRecord(account *protocol.DataAccount, url *url.URL, dataRecord DataRecord) {
	b.records = append(b.records, account)
	b.urls = append(b.urls, url)
	b.dataRecords = append(b.dataRecords, dataRecord)
}
