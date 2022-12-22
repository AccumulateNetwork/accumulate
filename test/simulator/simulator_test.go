// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator_test

import (
	"encoding/json"
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
	const snapDir = "../../../accumulate/.nodes/mainnet"
	const dbDir = "../../../accumulate/.nodes/mainnet/db"
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
	require.NoError(t, env.UnmarshalJSON([]byte(`{"signatures": [{"delegator": "acc://dn.acme/operators/1","signature": {"delegator": "acc://highstakes.acme/book/1","signature": {"publicKey": "dcd6722fa93b6b05f1f5b17e8ed1dcdd765e0fd5c7f832bec083a137824d751e","signature": "f5fa9a6a18adfb227badbf1391fa63428db3b5b93bfd9844a022acfe46176fe04d813d0ce4d2bcc719a301e42a697936cab8443dda54a22639279e31b6fd8a02","signer": "acc://ethan.acme/book/1","signerVersion": 7,"timestamp": 1671580382545,"transactionHash": "62ed36fb447bc4b724ddd779ecda1b50ff542700550f93382e96aaa44a350804","type": "ed25519"},"type": "delegated"},"type": "delegated"}],"transaction": [{"body": {"operation": [{"entry": {"delegate": "acc://pennyrocket.acme/book","keyHash": "a56a871b99f378ba4e8fb50ec303b5789bd3eedb67e1a464d5b53b2d3ace69d0"},"type": "add"},{"threshold": 21,"type": "setThreshold"}],"type": "updateKeyPage"},"header": {"initiator": "1d4183326712c83ac45e6a2586806286641a6c23f9331c7a443579ab6001fe3b","principal": "acc://dn.acme/operators/1"}}]}`)))

	d, err := chain.NormalizeEnvelope(env)
	require.NoError(t, err)
	require.Len(t, d, 1)

	st := sim.SubmitSuccessfully(d[0])
	sim.StepUntil(
		Txn(st.TxID).IsPending())

	env = new(Envelope)
	require.NoError(t, env.UnmarshalJSON([]byte(`{"signatures":{"delegator": "acc://dn.acme/operators/book/1","signature": {"delegator": "acc://tfa.acme/book/1","signature": {"publicKey": "26091c9bb45fc2ff83c02406ccbbacff50c2a8e7aab0da3dbeaa578152b7bf31","signature": "2d898b6dbd199f63c42a5d6a6d1dd849b8575c5e5bb0a430f0e683278dc5db812f471bbe67465fb0e7bcdf00f56f9ca5fcf06da3dd8343fd694b12a79f466b05","signer": "acc://dennis.acme/book/1","signerVersion": 3,"timestamp": 1671658502672,"transactionHash": "62ed36fb447bc4b724ddd779ecda1b50ff542700550f93382e96aaa44a350804","type": "ed25519"},"type": "delegated"},"type": "delegated"},"transaction": [{"body": {"operation": [{"entry": {"delegate": "acc://pennyrocket.acme/book","keyHash": "a56a871b99f378ba4e8fb50ec303b5789bd3eedb67e1a464d5b53b2d3ace69d0"},"type": "add"},{"threshold": 21,"type": "setThreshold"}],"type": "updateKeyPage"},"header": {"initiator": "1d4183326712c83ac45e6a2586806286641a6c23f9331c7a443579ab6001fe3b","principal": "acc://dn.acme/operators/1"}}]}`)))

	d, err = chain.NormalizeEnvelope(env)
	require.NoError(t, err)
	require.Len(t, d, 1)

	st = sim.SubmitSuccessfully(d[0])
	sim.StepN(50)

	View(t, sim.Database(Directory), func(batch *database.Batch) {
		h := st.TxID.Hash()
		s, err := batch.Transaction(h[:]).ReadSignatures(url.MustParse("dn.acme/operators/1"))
		require.NoError(t, err)
		for _, e := range s.Entries() {
			sig, err := batch.Transaction(e.SignatureHash[:]).Main().Get()
			require.NoError(t, err)
			b, _ := json.Marshal(sig.Signature)
			fmt.Println(string(b))
		}
	})
}
