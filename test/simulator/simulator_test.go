// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator_test

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

func init() {
	acctesting.EnableDebugFeatures()
}

func TestSimulator(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
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

	// Verify
	account := GetAccount[*TokenAccount](t, sim.DatabaseFor(bob), bob.JoinPath("tokens"))
	require.Equal(t, 123, int(account.Balance.Int64()))
}

func TestManual(t *testing.T) {
	const snapDir = "../../.nodes/mainnet"
	const dbDir = "../../.nodes/mainnet/db"
	net := &accumulated.NetworkInit{
		Id: t.Name(),
		Bvns: []*accumulated.BvnInit{
			{
				Id: "Apollo",
				Nodes: []*accumulated.NodeInit{{
					DnnType:          config.Validator,
					BvnnType:         config.Validator,
					BasePort:         26656,
					AdvertizeAddress: "127.0.1.1",
					PrivValKey:       acctesting.GenerateKey("Apollo", "val"),
				}},
			},
			{
				Id: "Yutu",
				Nodes: []*accumulated.NodeInit{{
					DnnType:          config.Validator,
					BvnnType:         config.Validator,
					BasePort:         26656,
					AdvertizeAddress: "127.0.1.2",
					PrivValKey:       acctesting.GenerateKey("Yutu", "val"),
				}},
			},
			{
				Id: "Chandrayaan",
				Nodes: []*accumulated.NodeInit{{
					DnnType:          config.Validator,
					BvnnType:         config.Validator,
					BasePort:         26656,
					AdvertizeAddress: "127.0.1.3",
					PrivValKey:       acctesting.GenerateKey("Chandrayaan", "val"),
				}},
			},
		},
	}
	sim := NewSim(t,
		func(partition string, node int, logger log.Logger) database.Beginner {
			// Open a database
			db, err := database.OpenBadger(filepath.Join(dbDir, fmt.Sprintf("%s-%d.db", partition, node)), logger)
			require.NoError(t, err)

			var restore bool
			Update(t, db, func(batch *database.Batch) {
				_, err := batch.Account(PartitionUrl(partition)).Main().Get()
				switch {
				case err == nil:
					return
				case errors.Is(err, errors.NotFound):
					restore = true
				default:
					require.NoError(t, err)
				}
			})
			if restore {
				// Restore from the snapshot
				f, err := os.Open(filepath.Join(snapDir, fmt.Sprintf("%s.snapshot", partition)))
				require.NoError(t, err)
				defer f.Close()

				fmt.Println("Restoring", partition, node)
				err = snapshot.FullRestore(db, f, logger, &config.Describe{PartitionId: partition})
				require.NoError(t, err)
			}

			// Return a batch so that all the testing is done in memory
			batch := db.Begin(true)
			t.Cleanup(func() { batch.Discard() })

			// Change the validator set
			UpdateAccount(t, batch, PartitionUrl(partition).JoinPath(Network), func(a *DataAccount) {
				entry := a.Entry.(*AccumulateDataEntry)
				network := new(protocol.NetworkDefinition)
				require.NoError(t, network.UnmarshalBinary(entry.Data[0]))

				network.Validators = nil
				for _, bvn := range net.Bvns {
					for _, node := range bvn.Nodes {
						network.AddValidator(node.PrivValKey[32:], bvn.Id, true)
						network.AddValidator(node.PrivValKey[32:], Directory, true)
					}
				}

				b, err := network.MarshalBinary()
				require.NoError(t, err)
				entry.Data[0] = b
			})

			return batch
		},
		net,
		simulator.SnapshotFromDirectory(snapDir),
	)

	env := new(Envelope)
	{
		require.NoError(t, env.UnmarshalJSON([]byte(`{"signatures":[{"type":"delegated","signature":{"type":"delegated","signature":{"type":"ed25519","publicKey":"dcd6722fa93b6b05f1f5b17e8ed1dcdd765e0fd5c7f832bec083a137824d751e","signature":"9bdaa7919e7a1deca617dc72b375e67e3765fe752390c23780aeb49b8183c5f0e9f1ae40ab798c918a916c094f68883b8633bd9d427cd6d98053f1c73036d80e","signer":"acc://ethan.acme/book/1","signerVersion":7,"timestamp":1671468298677,"transactionHash":"6ac622068f9b0020f5bb53ce2229bf6d758742e96ed32c1f1a0292c045f832ac"},"delegator":"acc://highstakes.acme/book/1"},"delegator":"acc://dn.acme/operators/1"}],"transaction":[{"header":{"principal":"acc://acme","initiator":"3128efc0ebcd46a88720b2f47da1f7b5eaff384d9c355da3df6571a5e24d7898"},"body":{"type":"issueTokens","amount":"0","to":[{"url":"acc://AthensNode.acme/rewards","amount":"98052740000"},{"url":"acc://AtlantaNode.acme/rewards","amount":"98052740000"},{"url":"acc://azn1nvas1on.acme/staking","amount":"112231560000"},{"url":"acc://BartowStreetCapital.acme/rewards","amount":"30875320000"},{"url":"acc://ben.acme/staking","amount":"74558430000"},{"url":"acc://BetaNode.acme/rewards","amount":"98052740000"},{"url":"acc://codeforj.acme/rewards","amount":"274352910000"},{"url":"acc://ConquistadorMining.acme/rewards","amount":"39722910000"},{"url":"acc://ConsensusNetworks.acme/staking","amount":"198492210000"},{"url":"acc://CryptoFund.acme/rewards","amount":"30415200000"},{"url":"acc://CryptoPoint.acme/rewards","amount":"1037879500000"},{"url":"acc://CryptoPool.acme/rewards","amount":"30415200000"},{"url":"acc://CryptoStake.acme/rewards","amount":"30415200000"},{"url":"acc://CryptoStash.acme/rewards","amount":"30415200000"},{"url":"acc://cybitron.acme/staking","amount":"328484220000"},{"url":"acc://defacto.acme/staking","amount":"30712180000"},{"url":"acc://defidevs.acme/stake","amount":"31253997610000"},{"url":"acc://dennis.acme/staking-yield","amount":"54747370000"},{"url":"acc://diopter-estrange-009.acme/tokens","amount":"624118040000"},{"url":"acc://diopter-estrange-010.acme/tokens","amount":"520096460000"},{"url":"acc://diopter-estrange-011.acme/tokens","amount":"520096460000"},{"url":"acc://diopter-estrange-012.acme/tokens","amount":"455531340000"},{"url":"acc://diopter-estrange-015.acme/tokens","amount":"312053290000"},{"url":"acc://diopter-estrange-017.acme/tokens","amount":"273736850000"},{"url":"acc://diopter-estrange-019.acme/tokens","amount":"254662070000"},{"url":"acc://diopter-estrange-021.acme/tokens","amount":"206230880000"},{"url":"acc://diopter-estrange-023.acme/tokens","amount":"63068420000"},{"url":"acc://diopter-estrange-025.acme/tokens","amount":"125972820000"},{"url":"acc://diopter-estrange-027.acme/tokens","amount":"151878160000"},{"url":"acc://diopter-estrange-028.acme/tokens","amount":"136868420000"},{"url":"acc://diopter-estrange-029.acme/tokens","amount":"136868420000"},{"url":"acc://diopter-estrange-031.acme/tokens","amount":"104021580000"},{"url":"acc://diopter-estrange-033.acme/tokens","amount":"104021580000"},{"url":"acc://diopter-estrange-036.acme/tokens","amount":"82121050000"},{"url":"acc://diopter-estrange-037.acme/tokens","amount":"82121050000"},{"url":"acc://diopter-estrange-038.acme/tokens","amount":"75933350000"},{"url":"acc://diopter-estrange-039.acme/tokens","amount":"75933620000"},{"url":"acc://diopter-estrange-041.acme/tokens","amount":"75933350000"},{"url":"acc://diopter-estrange-042.acme/tokens","amount":"75933350000"},{"url":"acc://diopter-estrange-043.acme/tokens","amount":"75933350000"},{"url":"acc://diopter-estrange-045.acme/tokens","amount":"75933350000"},{"url":"acc://diopter-estrange-046.acme/tokens","amount":"75933350000"},{"url":"acc://diopter-estrange-049.acme/tokens","amount":"52005060000"},{"url":"acc://diopter-estrange-051.acme/tokens","amount":"38195870000"},{"url":"acc://diopter-estrange-052.acme/tokens","amount":"37966670000"},{"url":"acc://diopter-estrange-053.acme/tokens","amount":"37966670000"},{"url":"acc://diopter-estrange-054.acme/tokens","amount":"37966670000"},{"url":"acc://diopter-estrange-055.acme/tokens","amount":"37966670000"},{"url":"acc://diopter-estrange-056.acme/tokens","amount":"37966670000"},{"url":"acc://diopter-estrange-057.acme/tokens","amount":"37966670000"},{"url":"acc://diopter-estrange-058.acme/tokens","amount":"33127080000"},{"url":"acc://diopter-estrange-061.acme/tokens","amount":"27373680000"},{"url":"acc://diopter-estrange-064.acme/tokens","amount":"689122180000"},{"url":"acc://diopter-estrange-065.acme/tokens","amount":"25463910000"},{"url":"acc://diopter-estrange-066.acme/tokens","amount":"25062810000"},{"url":"acc://diopter-estrange-067.acme/tokens","amount":"22919810000"},{"url":"acc://diopter-estrange-068.acme/tokens","amount":"22782290000"},{"url":"acc://diopter-estrange-069.acme/tokens","amount":"22782290000"},{"url":"acc://diopter-estrange-070.acme/tokens","amount":"8978490000"},{"url":"acc://diopter-estrange-071.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-072.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-073.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-074.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-075.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-076.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-077.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-079.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-081.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-082.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-083.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-084.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-085.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-086.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-087.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-088.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-089.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-090.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-091.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-092.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-093.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-094.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-095.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-097.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-098.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-099.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-100.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-101.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-102.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-104.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-105.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-106.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-107.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-108.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-109.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-110.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-111.acme/tokens","amount":"18977600000"},{"url":"acc://diopter-estrange-115.acme/tokens","amount":"12731950000"},{"url":"acc://diopter-estrange-116.acme/tokens","amount":"12731950000"},{"url":"acc://diopter-estrange-117.acme/tokens","amount":"12731950000"},{"url":"acc://diopter-estrange-118.acme/tokens","amount":"12731950000"},{"url":"acc://diopter-estrange-119.acme/tokens","amount":"12731950000"},{"url":"acc://diopter-estrange-120.acme/tokens","amount":"12731950000"},{"url":"acc://diopter-estrange-122.acme/tokens","amount":"8212100000"},{"url":"acc://diopter-estrange-123.acme/tokens","amount":"6360240000"},{"url":"acc://diopter-estrange-124.acme/tokens","amount":"6360240000"},{"url":"acc://diopter-estrange-125.acme/tokens","amount":"6360240000"},{"url":"acc://diopter-estrange-126.acme/tokens","amount":"6360240000"},{"url":"acc://diopter-estrange-127.acme/tokens","amount":"6360240000"},{"url":"acc://diopter-estrange-128.acme/tokens","amount":"6360240000"},{"url":"acc://diopter-estrange-129.acme/tokens","amount":"6360240000"},{"url":"acc://diopter-estrange-130.acme/tokens","amount":"6360240000"},{"url":"acc://diopter-estrange-131.acme/tokens","amount":"6360240000"},{"url":"acc://diopter-estrange-132.acme/tokens","amount":"5474730000"},{"url":"acc://diopter-estrange-133.acme/tokens","amount":"3816140000"},{"url":"acc://diopter-estrange-134.acme/tokens","amount":"3174390000"},{"url":"acc://Dolphins1972.acme/rewards","amount":"30415200000"},{"url":"acc://DrKyle.acme/Rewards","amount":"73908950000"},{"url":"acc://DRobertB.acme/Tokens","amount":"107171950000"},{"url":"acc://Everest.acme/rewards","amount":"84288720000"},{"url":"acc://Factoshi.acme/rewards","amount":"34116740000"},{"url":"acc://federatethis.acme/rewards","amount":"196572860000"},{"url":"acc://GOI.acme/rewards","amount":"30415200000"},{"url":"acc://HighStakes.acme/CashCow","amount":"167084820000"},{"url":"acc://inveniam.acme/holding","amount":"2362199380000"},{"url":"acc://kompendium.acme/tokens","amount":"912456170000"},{"url":"acc://LunaNova.acme/staking","amount":"270158210000"},{"url":"acc://MetisMining.acme/rewards","amount":"4240008340000"},{"url":"acc://MusicCityNode.acme/rewards","amount":"30652780000"},{"url":"acc://NTC.acme/rewards","amount":"42216850000"},{"url":"acc://NutshellTechnologies.acme/rewards","amount":"219184290000"},{"url":"acc://OrchardHouse.acme/rewards","amount":"136533490000"},{"url":"acc://p16redstone.acme/staking","amount":"33783400000"},{"url":"acc://Pavilion.acme/rewards","amount":"61305570000"},{"url":"acc://pennyrocket.acme/staking","amount":"349621470000"},{"url":"acc://prestigeit.acme/tokens","amount":"121660820000"},{"url":"acc://Quentin.acme/rewards","amount":"54747370000"},{"url":"acc://reesor.acme/rewards","amount":"727046170000"},{"url":"acc://Sphereon.acme/stake","amount":"12470170000"},{"url":"acc://StampIt.acme/rewards","amount":"355560180000"},{"url":"acc://tfa.acme/staking-yield","amount":"202654690000"},{"url":"acc://tfalabs.acme/staking-yield","amount":"51237360000"},{"url":"acc://TurtleBoat.acme/rewards","amount":"51085370000"},{"url":"acc://Vidal.acme/tokens","amount":"54747370000"},{"url":"acc://07025b1118eaaac1ab924b7c2ec781842f48bb4f7ea8f4b5/ACME","amount":"7299648160000"},{"url":"acc://ZekeInvestments.acme/rewards","amount":"58923850000"}]}}]}`)))
	}

	d, err := chain.NormalizeEnvelope(env)
	require.NoError(t, err)
	require.Len(t, d, 1)

	st := sim.SubmitSuccessfully(d[0])
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())
}
