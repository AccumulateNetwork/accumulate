package encoding

import (
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestGenerateDbTestdata(t *testing.T) {
	// t.Skip("Only run manually")

	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	logger := acctesting.NewTestLogger(t)
	store := memory.New(logger)
	db := database.New(store, logger)

	// Initialize
	sim := NewSim(t,
		func(partition string, _ int, logger log.Logger) database.Beginner {
			if strings.EqualFold(partition, Directory) {
				return database.OpenInMemory(logger)
			}
			return db
		},
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Execute
	st := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "tokens").
			SendTokens(123, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey)))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// Export
	f, err := os.Create("../testdata/database.json")
	require.NoError(t, err)
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	err = enc.Encode(store)
	require.NoError(t, err)

	f, err = os.Create("../testdata/database.snapshot")
	require.NoError(t, err)
	defer f.Close()
	batch := db.Begin(false)
	defer batch.Discard()
	_, err = snapshot.Collect(batch, new(snapshot.Header), f, snapshot.CollectOptions{
		Logger: logger,
	})
	require.NoError(t, err)
}

func TestDbEncoding(t *testing.T) {
	b, err := os.ReadFile("../testdata/database-v1.0.0.json")
	require.NoError(t, err)
	logger := acctesting.NewTestLogger(t)
	store := memory.New(logger)
	err = store.UnmarshalJSON(b)
	require.NoError(t, err)

	db := database.New(store, logger)
	batch := db.Begin(false)
	defer batch.Discard()
	buf := new(ioutil2.Buffer)
	_, err = snapshot.Collect(batch, new(snapshot.Header), buf, snapshot.CollectOptions{
		Logger: logger,
	})
	require.NoError(t, err)

	b, err = os.ReadFile("../testdata/database-v1.0.0.snapshot")
	require.NoError(t, err)
	require.Equal(t, b, buf.Bytes())
}
