package block_test

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestBatchDump(t *testing.T) {
	// Initialize
	router := SetupExecNetwork(t)
	router.InitChain()

	// Create a lite address
	alice := acctesting.GenerateTmKey(t.Name())
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)

	// Fund the lite account
	faucet := protocol.Faucet.Signer()
	env := acctesting.NewTransaction().
		WithPrincipal(protocol.FaucetUrl).
		WithTimestamp(faucet.Timestamp()).
		WithBody(&protocol.AcmeFaucet{Url: aliceUrl}).
		Faucet()

	x := router.Executors[router.Network.Subnets[1].ID]
	block := new(block.Block)
	block.IsLeader = true
	ExecuteBlockNoCommit(t, x.Database, x.Executor, block, env)

	filename := filepath.Join(t.TempDir(), "batch-dump.json")
	block.Batch.Dump(filename)

	b, err := ioutil.ReadFile(filename)
	require.NoError(t, err)
	t.Log(string(b))
}
