// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block_test

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestStateSaveAndRestore(t *testing.T) {
	if !protocol.IsTestNet {
		t.Skip("Faucet")
	}

	var timestamp uint64

	// Initialize
	t.Log("Setup")
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Prepare the ADI
	name := AccountUrl("foobarbaz")
	key := acctesting.GenerateKey(t.Name(), name)
	SetupIdentity(sim, name, key, &timestamp)

	// Create snapshots
	t.Log("Save")
	dir := t.TempDir()
	filename := func(partition string) string {
		return filepath.Join(dir, fmt.Sprintf("%s.bpt", partition))
	}
	for _, partition := range sim.S.Partitions() {
		x := sim.Partition(partition.ID)
		batch := x.Database.Begin(false)
		defer batch.Discard()
		f, err := os.Create(filename(partition.ID))
		require.NoError(t, err)
		require.NoError(t, snapshot.FullCollect(batch, f, config.NetworkUrl{URL: PartitionUrl(partition.ID)}, nil, false))
		require.NoError(t, f.Close())
	}

	// Create a new network
	t.Log("Restore")
	sim = simulator.New(t, 3)
	sim.InitFromSnapshot(filename)

	// Send tokens to a lite account
	liteUrl := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey(sim.TB.Name(), "Recipient"))
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(name.JoinPath("tokens")).
			WithSigner(name.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&SendTokens{
				To: []*TokenRecipient{
					{Url: liteUrl, Amount: *big.NewInt(68)},
				},
			}).
			Initiate(SignatureTypeED25519, key).
			Build(),
	)...)
}

func SetupIdentity(sim *simulator.Simulator, name *url.URL, key []byte, timestamp *uint64) {
	// Fund a lite account
	liteKey := acctesting.GenerateKey(sim.TB.Name(), "SetupIdentity", name)
	liteUrl := acctesting.AcmeLiteAddressStdPriv(liteKey)
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(FaucetUrl).
			WithBody(&AcmeFaucet{Url: liteUrl}).
			Faucet(),
	)...)

	// Add credits to the lite account
	const liteCreditAmount = 1 * AcmePrecision
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(liteUrl).
			WithSigner(liteUrl, 1).
			WithTimestampVar(timestamp).
			WithBody(&AddCredits{
				Recipient: liteUrl,
				Amount:    *big.NewInt(liteCreditAmount),
				Oracle:    InitialAcmeOracleValue,
			}).
			Initiate(SignatureTypeED25519, liteKey).
			Build(),
	)...)

	// Create the ADI
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(liteUrl).
			WithSigner(liteUrl, 1).
			WithTimestampVar(timestamp).
			WithBody(&CreateIdentity{
				Url:        name,
				KeyBookUrl: name.JoinPath("book"),
				KeyHash:    doSha256(key[32:]),
			}).
			Initiate(SignatureTypeED25519, liteKey).
			Build(),
	)...)

	// Add credits to the key page
	const tokenAccountAmount = 5 * AcmePrecision
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(liteUrl).
			WithSigner(liteUrl, 1).
			WithTimestampVar(timestamp).
			WithBody(&AddCredits{
				Recipient: name.JoinPath("book", "1"),
				Amount:    *big.NewInt(AcmePrecision*AcmeFaucetAmount - liteCreditAmount - tokenAccountAmount),
				Oracle:    InitialAcmeOracleValue,
			}).
			Initiate(SignatureTypeED25519, liteKey).
			Build(),
	)...)

	// Create a token account
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(name).
			WithSigner(name.JoinPath("book", "1"), 1).
			WithTimestampVar(timestamp).
			WithBody(&CreateTokenAccount{
				Url:      name.JoinPath("tokens"),
				TokenUrl: AcmeUrl(),
			}).
			Initiate(SignatureTypeED25519, key).
			Build(),
	)...)

	// Send tokens to the ADI token account
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(liteUrl).
			WithSigner(liteUrl, 1).
			WithTimestampVar(timestamp).
			WithBody(&SendTokens{
				To: []*TokenRecipient{
					{Url: name.JoinPath("tokens"), Amount: *big.NewInt(tokenAccountAmount)},
				},
			}).
			Initiate(SignatureTypeED25519, liteKey).
			Build(),
	)...)
}
