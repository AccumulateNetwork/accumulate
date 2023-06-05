// Copyright 2023 The Accumulate Authors
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

	"github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/sync/errgroup"
)

type InitOpts struct {
	PartitionId     string
	NetworkType     protocol.PartitionType
	GenesisTime     time.Time
	Logger          log.Logger
	FactomAddresses func() (io.Reader, error)
	Snapshots       []func() (ioutil2.SectionReader, error)
	GenesisGlobals  *core.GlobalValues
	OperatorKeys    [][]byte

	IncludeHistoryFromSnapshots bool
}

func Init(snapshotWriter io.WriteSeeker, opts InitOpts) ([]byte, error) {
	// Initialize globals
	gg := core.NewGlobals(opts.GenesisGlobals)

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
		db:          database.New(store, opts.Logger.With("module", "database")),
		dataRecords: make([]DataRecord, 0),
		records:     make([]protocol.Account, 0),
		acmeIssued:  new(big.Int),
		omitHistory: map[[32]byte]bool{},
		partition:   config.NetworkUrl{URL: protocol.PartitionUrl(opts.PartitionId)},
	}
	b.db.SetObserver(execute.NewDatabaseObserver())

	// Create the router
	var err error
	b.router, err = routing.NewStaticRouter(gg.Routing, b.Logger)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Unpack snapshots
	err = b.unpackSnapshots()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	exec, err := block.NewGenesisExecutor(b.db, opts.Logger, &config.Describe{
		NetworkType: opts.NetworkType,
		PartitionId: opts.PartitionId,
	}, gg, b.router)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
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
		return nil, errors.UnknownError.Wrap(err)
	}

	err = b.block.Batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Wait for background tasks
	err = errg.Wait()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Preserve history in the Genesis snapshot
	batch := b.db.Begin(false)
	defer batch.Discard()

	header := new(snapshot.Header)
	header.Height = protocol.GenesisBlock

	w, err := snapshot.Collect(batch, header, snapshotWriter, snapshot.CollectOptions{
		Logger: b.Logger,
		PreserveAccountHistory: func(account *database.Account) (bool, error) {
			return !b.omitHistory[account.Url().AccountID32()], nil
		},
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("collect snapshot: %w", err)
	}

	err = snapshot.CollectAnchors(w, batch, exec.Describe.PartitionUrl())
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	h, err := batch.BPT().GetRootHash()
	return h[:], err
}

type bootstrap struct {
	InitOpts
	networkAuthority *url.URL
	partition        config.NetworkUrl
	localAuthority   *url.URL

	kvdb        keyvalue.Beginner
	db          *database.Database
	block       *block.Block
	urls        []*url.URL
	records     []protocol.Account
	dataRecords []DataRecord
	router      routing.Router

	accountsFromSnapshots []*url.URL
	acmeIssued            *big.Int
	omitHistory           map[[32]byte]bool
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
	ledger.ExecutorVersion = b.GenesisGlobals.ExecutorVersion
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
	var accounts []*url.URL
	for i, open := range b.Snapshots {
		_ = i
		file, err := open()
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		if c, ok := file.(io.Closer); ok {
			defer c.Close()
		}

		err = b.db.Restore(file, &database.RestoreOptions{
			BatchRecordLimit: 50_000,
			SkipHashCheck:    true,
			Predicate: func(v record.TerminalRecord) (bool, error) {
				switch v.Key().Get(0) {
				case "Account":
					// Track ACME issued
					if v.Key().Len() == 3 && v.Key().Get(2) == "Main" {
						u, _, err := v.GetValue()
						switch {
						case err == nil:
							if acct, ok := u.(protocol.AccountWithTokens); ok && protocol.AcmeUrl().Equal(acct.GetTokenUrl()) {
								b.acmeIssued.Add(b.acmeIssued, acct.TokenBalance())
							}
						case !errors.Is(err, errors.NotFound):
							return false, errors.UnknownError.Wrap(err)
						}
					}

					// Is this record for an account that belongs to this
					// partition?
					u := v.Key().Get(1).(*url.URL)
					partition, err := b.router.RouteAccount(u)
					if err != nil {
						return false, errors.InternalError.WithFormat("route %v: %w", u, err)
					}
					if !strings.EqualFold(partition, b.PartitionId) {
						return false, nil
					}

					accounts = append(accounts, u)
					return true, nil

				default:
					// Ignore everything else on this pass
					return false, nil
				}
			},
		})
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// Find relevant chain entries
	hashes := map[[32]byte]bool{}
	batch := b.db.Begin(false)
	defer batch.Discard()
	for _, u := range accounts {
		a := batch.Account(u)
		chains, err := a.Chains().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load chains index: %w", err)
		}
		for _, chain := range chains {
			if chain.Type != merkle.ChainTypeTransaction {
				continue
			}

			c, err := a.ChainByName(chain.Name)
			if err != nil {
				return errors.InvalidRecord.Wrap(err)
			}

			head, err := c.Head().Get()
			if err != nil {
				return errors.UnknownError.WithFormat("load %s chain head: %w", c.Name(), err)
			}
			if head.Count == 0 {
				return nil
			}

			entries, err := c.Inner().GetRange(0, head.Count)
			if err != nil {
				return errors.UnknownError.WithFormat("load %s chain entries: %w", c.Name(), err)
			}
			for _, h := range entries {
				hashes[*(*[32]byte)(h)] = true
			}
		}
	}

	// Restore messages
	for _, open := range b.Snapshots {
		file, err := open()
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		if c, ok := file.(io.Closer); ok {
			defer c.Close()
		}

		b.db.Restore(file, &database.RestoreOptions{
			BatchRecordLimit: 50_000,
			Predicate: func(v record.TerminalRecord) (bool, error) {
				switch v.Key().Get(0) {
				case "Transaction", "Message":
					h := v.Key().Get(1).([32]byte)
					return hashes[h], nil

				default:
					// Ignore everything else on this pass
					return false, nil
				}
			},
		})
	}

	b.accountsFromSnapshots = accounts
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
