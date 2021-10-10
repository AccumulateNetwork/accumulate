package abci_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime/trace"
	"testing"

	acctesting "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/AccumulateNetwork/accumulated/smt/storage/badger"
	"github.com/AccumulateNetwork/accumulated/types"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto"
)

func TestAC398(t *testing.T) {
	dir := t.TempDir()
	db := new(badger.DB)
	err := db.InitDB(filepath.Join(dir, "valacc.db"))
	require.NoError(t, err)
	defer db.Close()

	bvcId := sha256.Sum256([]byte("foo bar"))
	sdb := new(state.StateDB)
	sdb.Load(db, bvcId[:], true)

	n := createApp(t, sdb, crypto.Address{})

	n.Batch(func(send func(*transactions.GenTransaction)) {
		gtx := new(transactions.GenTransaction)
		_, err := gtx.UnMarshal(unhex(t, "0101201A74106509CB7C2B231A15A7D922C79CDAEC35931CA5A070ABB438B47E35D969925DC239EF8AC4D227D649AEB19E54C56E7571452FF7FCD7018F4C7DCF4BB1FB601BAEF1B518AB03970535D5A9383F27C97C241047D808D936469C2922FD073561636D652D366261373735633833313432666233343534646462636366313937353161633437383336303833666239323061376635010000009C012E127930701C81E36DFD50BB923D361559C661EBCD034530D3F1C6F5CBF27DFA963561636D652D3930663432323266386664616535653038646631623532623536623536376632353836666531666462653931306664613561636D652D36626137373563383331343266623334353464646263636631393735316163343738333630383366623932306137663506048C273950000764632F41434D45"))
		require.NoError(t, err)
		send(gtx)
	})

	n.client.Wait()

	require.Equal(t, "8DC5D6BC55964886C1075A423C9BDADC465DD10E853711EDD3EF144BEABBEC77", fmt.Sprintf("%X", n.db.RootHash()))

	n.Batch(func(send func(*transactions.GenTransaction)) {
		gtx := new(transactions.GenTransaction)
		_, err := gtx.UnMarshal(unhex(t, "0102201A74106509CB7C2B231A15A7D922C79CDAEC35931CA5A070ABB438B47E35D9B17B580534E379E9D0649344A4EAF566646BE5F45ADD80F53DB669BE4A8F135FF0F36507084E88D35F1DFD076C399AEAEDBA53037A2A5D997567088EF82822063561636D652D3662613737356338333134326662333435346464626363663139373531616334373833363038336662393230613766350000000070033561636D652D366261373735633833313432666233343534646462636366313937353161633437383336303833666239323061376635013561636D652D373936643936326639613538613634393164313334643063643363356161623861643637383432336333313234613133E807"))
		require.NoError(t, err)
		send(gtx)
	})

	n.client.Wait()

	require.Equal(t, "03768128E9EB12BD17ACC813BDEA29DEA766FAF444D893A9508269B0D5DBBB29", fmt.Sprintf("%X", n.db.RootHash()))
}

func TestConsistency(t *testing.T) {
	sponsor := generateKey()
	account := generateKey()
	recipient := generateKey()

	accountUrl := anon.GenerateAcmeAddress(account.PubKey().Bytes())
	recipientUrl := anon.GenerateAcmeAddress(recipient.PubKey().Bytes())

	tx1, err := acctesting.CreateFakeSyntheticDepositTx(sponsor, account)
	require.NoError(t, err)

	tokenTx := api.NewTokenTx(types.String(accountUrl))
	tokenTx.AddToAccount(types.String(recipientUrl), 1000)
	tx2, err := transactions.New(accountUrl, edSigner(account, 1), tokenTx)
	require.NoError(t, err)

	var state1, state2 []byte
	t.Run("Initial", func(t *testing.T) {
		f, err := os.Create("trace-init.log")
		require.NoError(t, err)
		defer f.Close()
		require.NoError(t, trace.Start(f))
		defer trace.Stop()

		n := createAppWithMemDB(t, crypto.Address{})
		defer n.db.GetDB().Close()

		n.Batch(func(send func(*Tx)) { send(tx1) })
		n.client.Wait()
		state1 = n.db.RootHash()

		n.Batch(func(send func(*Tx)) { send(tx2) })
		n.client.Wait()
		state2 = n.db.RootHash()
	})

	for i := 0; i < 2; i++ {
		t.Run(fmt.Sprintf("Replay%d", i), func(t *testing.T) {
			f, err := os.Create(fmt.Sprintf("trace-%d.log", i))
			require.NoError(t, err)
			defer f.Close()
			require.NoError(t, trace.Start(f))
			defer trace.Stop()

			n := createAppWithMemDB(t, crypto.Address{})
			defer n.db.GetDB().Close()

			n.Batch(func(send func(*Tx)) { send(tx1) })
			n.client.Wait()
			require.Equal(t, fmt.Sprintf("%X", state1), fmt.Sprintf("%X", n.db.RootHash()))

			n.Batch(func(send func(*Tx)) { send(tx2) })
			n.client.Wait()
			require.Equal(t, fmt.Sprintf("%X", state2), fmt.Sprintf("%X", n.db.RootHash()))
		})
	}
}
