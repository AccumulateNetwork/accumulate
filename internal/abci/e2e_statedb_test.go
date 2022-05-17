package abci_test

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/badger"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
	"path/filepath"
	"testing"
)

func init() { acctesting.EnableDebugFeatures() }

func TestStateDBConsistency(t *testing.T) {
	acctesting.SkipPlatformCI(t, "darwin", "flaky")

	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0, false)
	stores := map[*accumulated.Daemon]storage.KeyValueStore{}
	for _, netName := range subnets {
		for _, daemon := range daemons[netName] {
			// store, err := badger.New(filepath.Join(daemon.Config.RootDir, "badger.db"), nil)
			// require.NoError(t, err)
			// stores[daemon] = store
			store := memory.New(nil)
			stores[daemon] = store

			// Call during test cleanup. This ensures that the app client is shutdown
			// before the database is closed.
			t.Cleanup(func() { store.Close() })
		}
	}

	getDb := func(d *accumulated.Daemon) (*database.Database, error) { return database.New(stores[d], d.Logger), nil }
	nodes := RunTestNet(t, subnets, daemons, getDb, true, nil)
	n := nodes[subnets[1]][0]

	credits := 40.0
	n.testLiteTx(10, 1, credits)

	for _, nodes := range nodes {
		for _, node := range nodes {
			node.client.Shutdown()
		}
	}

	ledger := n.network.NodeUrl(protocol.Ledger)
	var ledger1 *protocol.InternalLedger
	batch := n.db.Begin(false)
	require.NoError(t, batch.Account(ledger).GetStateAs(&ledger1))
	anchor, err := batch.GetMinorRootChainAnchor(n.network)
	if err != nil {
		panic(fmt.Errorf("failed to get anchor: %v", err))
	}
	rootHash := anchor
	batch.Discard()

	// Reopen the database
	db := database.New(stores[daemons[subnets[1]][0]], nil)

	// Block 6 does not make changes so is not saved
	batch = db.Begin(false)
	var ledger2 *protocol.InternalLedger
	anchor, err = batch.GetMinorRootChainAnchor(n.network)
	require.NoError(t, err)
	require.NoError(t, batch.Account(ledger).GetStateAs(&ledger2))
	require.Equal(t, ledger1, ledger2, "Ledger does not match after load from disk")
	require.Equal(t, fmt.Sprintf("%X", rootHash), fmt.Sprintf("%X", anchor), "Hash does not match after load from disk")
	batch.Discard()

	// Recreate the app and try to do more transactions
	nodes = RunTestNet(t, subnets, daemons, getDb, false, nil)
	n = nodes[subnets[1]][0]

	n.testLiteTx(10, 1, credits)
}

//TestAccumulatedEnvelopes is a test to overwhelm the state database Commit operation with nested envelopes exceeding transaction limits
func TestAccumulatedEnvelopes(t *testing.T) {
	acctesting.SkipPlatform(t, "windows", "flaky")
	acctesting.SkipPlatform(t, "darwin", "flaky")
	acctesting.SkipPlatformCI(t, "darwin", "requires setting up localhost aliases")

	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0, false)

	stores := map[*accumulated.Daemon]storage.KeyValueStore{}
	for _, netName := range subnets {
		for _, daemon := range daemons[netName] {
			//use a real badger database for this test.
			store, err := badger.New(filepath.Join(daemon.Config.RootDir, "badger.db"), nil)
			require.NoError(t, err)
			stores[daemon] = store

			// Call during test cleanup. This ensures that the app client is shutdown
			// before the database is closed.
			t.Cleanup(func() { store.Close() })
		}
	}

	getDb := func(d *accumulated.Daemon) (*database.Database, error) { return database.New(stores[d], d.Logger), nil }
	nodes := RunTestNet(t, subnets, daemons, getDb, true, nil)
	n := nodes[subnets[1]][0]

	//give us a bunch of credits...
	const initialCredits = 1e16

	// Setup keys and the lite account
	liteKey := generateKey()
	//	keyHash := sha256.Sum256(adiKey.PubKey().Bytes())
	batch := n.db.Begin(true)
	sponsor := protocol.LiteAuthorityForKey(liteKey.PubKey().Bytes(), protocol.SignatureTypeED25519)
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, liteKey, protocol.AcmeFaucetAmount, initialCredits))
	require.NoError(t, batch.Commit())
	//liteId := acctesting.AcmeLiteAddressTmPriv(liteKey).RootIdentity()
	//create a lite data chain
	var chainId [32]byte
	var err error
	n.MustExecuteAndWait(func(send func(*Tx)) {
		wd := new(protocol.WriteDataTo)
		fde := protocol.NewFactomDataEntry()
		fde.ExtIds = append(fde.ExtIds, []byte("test"))
		fde.ExtIds = append(fde.ExtIds, []byte("data"))
		fde.ExtIds = append(fde.ExtIds, []byte("overload"))
		copy(chainId[:], protocol.ComputeLiteDataAccountId(fde))
		wd.Recipient, err = protocol.LiteDataAddress(chainId[:])
		require.NoError(t, err)

		e := newTxn(sponsor.String()).
			WithSigner(sponsor, 1).
			WithBody(wd).
			WithCurrentTimestamp().
			Initiate(protocol.SignatureTypeED25519, liteKey.Bytes()).
			Build()
		send(e)
	})

	n.MustExecuteAndWait(func(send func(*Tx)) {
		wd := new(protocol.WriteDataTo)
		fde := protocol.NewFactomDataEntry()
		fde.Data = make([]byte, 10240)
		_, err := rand.Read(fde.Data)
		require.NoError(t, err)
		wd.Entry = fde
		wd.Recipient, err = protocol.LiteDataAddress(chainId[:])
		require.NoError(t, err)

		send(newTxn(sponsor.String()).
			WithSigner(sponsor, 1).
			WithBody(wd).
			WithCurrentTimestamp().
			Initiate(protocol.SignatureTypeED25519, liteKey.Bytes()).
			Build())
	})

	// Give it a second for the DN to send its anchor
	//time.Sleep(5 * time.Second)
	//
	////tx := acctesting.NewTransaction().
	////	WithPrincipal(u).
	////	WithTimestampVar(&globalNonce).
	////	WithSigner(u.RootIdentity(), 1)
	//
	//var total int64
	//var txids [][]byte
	//for i := 0; i < 10; i++ {
	//	if i > 2 && testing.Short() {
	//		break
	//	}
	//
	//	exch := new(protocol.SendTokens)
	//	for i := 0; i < 10; i++ {
	//		if i > 2 && testing.Short() {
	//			break
	//		}
	//		recipient := recipients[s.rand.Intn(len(recipients))]
	//		exch.AddRecipient(recipient, big.NewInt(int64(1000)))
	//		total += 1000
	//	}
	//
	//	nonce++
	//	tx := acctesting.NewTransaction().
	//		WithPrincipal(senderUrl).
	//		WithSigner(senderUrl, s.dut.GetRecordHeight(senderUrl.String())).
	//		WithTimestamp(nonce).
	//		WithBody(exch).
	//		Initiate(protocol.SignatureTypeLegacyED25519, sender).
	//		Build()
	//	s.dut.SubmitTxn(tx)
	//	txids = append(txids, tx.Transaction[0].GetHash())
	//}
	//total += amtAcmeToBuyCredits
	//
	//s.dut.WaitForTxns(txids...)

}
