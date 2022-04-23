package block_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

func updateSubnetFor(sim *simulator.Simulator, account *url.URL, fn func(batch *database.Batch)) {
	_ = sim.SubnetFor(account).Database.Update(func(batch *database.Batch) error {
		fn(batch)
		return nil
	})
}

func viewSubnetFor(sim *simulator.Simulator, account *url.URL, fn func(batch *database.Batch)) {
	_ = sim.SubnetFor(account).Database.View(func(batch *database.Batch) error {
		fn(batch)
		return nil
	})
}

func TestDelegatedSignature_Local(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	// Setup
	alice := url.MustParse("alice")
	key1, key2 := acctesting.GenerateKey(), acctesting.GenerateKey()
	updateSubnetFor(sim, alice, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key1), types.String(alice.String()), 1e9))
		require.NoError(t, acctesting.CreateKeyBook(batch, types.String(alice.JoinPath("book1").String()), tmed25519.PubKey(key2[32:])))
		require.NoError(t, acctesting.AddCredits(batch, alice.JoinPath("book1", "1"), 1e9))
		require.NoError(t, acctesting.CreateAccount(batch, &DataAccount{Url: alice.JoinPath("data")}))
		require.NoError(t, acctesting.UpdateKeyPage(batch, alice.JoinPath("book0", "1"), func(page *KeyPage) {
			page.Keys = append(page.Keys, &KeySpec{Owner: alice.JoinPath("book1", "1")})
		}))
	})

	// Execute
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithBody(&WriteData{
				Entry: DataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			WithSigner(alice.JoinPath("book1", "1"), 1).
			WithDelegator(alice.JoinPath("book0", "1")).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, key2).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	viewSubnetFor(sim, alice, func(batch *database.Batch) {
		data, err := batch.Account(alice.JoinPath("data")).Data()
		require.NoError(t, err)
		de, err := data.Entry(data.Height() - 1)
		require.NoError(t, err)
		require.Equal(t, "foo", string(de.Data[0]))
	})
}

func TestDelegatedSignature_Double(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	// Setup
	alice := url.MustParse("alice")
	key1, key2, key3 := acctesting.GenerateKey(), acctesting.GenerateKey(), acctesting.GenerateKey()
	updateSubnetFor(sim, alice, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key1), types.String(alice.String()), 1e9))
		require.NoError(t, acctesting.CreateKeyBook(batch, types.String(alice.JoinPath("book1").String()), tmed25519.PubKey(key2[32:])))
		require.NoError(t, acctesting.AddCredits(batch, alice.JoinPath("book1", "1"), 1e9))
		require.NoError(t, acctesting.CreateKeyBook(batch, types.String(alice.JoinPath("book2").String()), tmed25519.PubKey(key3[32:])))
		require.NoError(t, acctesting.AddCredits(batch, alice.JoinPath("book2", "1"), 1e9))
		require.NoError(t, acctesting.CreateAccount(batch, &DataAccount{Url: alice.JoinPath("data")}))
		require.NoError(t, acctesting.UpdateKeyPage(batch, alice.JoinPath("book0", "1"), func(page *KeyPage) {
			page.Keys = append(page.Keys, &KeySpec{Owner: alice.JoinPath("book1", "1")})
		}))
		require.NoError(t, acctesting.UpdateKeyPage(batch, alice.JoinPath("book1", "1"), func(page *KeyPage) {
			page.Keys = append(page.Keys, &KeySpec{Owner: alice.JoinPath("book2", "1")})
		}))
	})

	// Execute
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithBody(&WriteData{
				Entry: DataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			WithSigner(alice.JoinPath("book2", "1"), 1).
			WithDelegator(alice.JoinPath("book1", "1")).
			WithDelegator(alice.JoinPath("book0", "1")).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, key2).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	viewSubnetFor(sim, alice, func(batch *database.Batch) {
		data, err := batch.Account(alice.JoinPath("data")).Data()
		require.NoError(t, err)
		de, err := data.Entry(data.Height() - 1)
		require.NoError(t, err)
		require.Equal(t, "foo", string(de.Data[0]))
	})
}

func TestDelegatedSignature_RemoteDelegate(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	alice, bob := url.MustParse("alice"), url.MustParse("bob")
	sim.SetRouteFor(alice, "BVN0")
	sim.SetRouteFor(bob, "BVN1")

	// Setup
	key1, key2 := acctesting.GenerateKey(), acctesting.GenerateKey()
	updateSubnetFor(sim, alice, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key1), types.String(alice.String()), 1e9))
		require.NoError(t, acctesting.CreateAccount(batch, &DataAccount{Url: alice.JoinPath("data")}))
		require.NoError(t, acctesting.UpdateKeyPage(batch, alice.JoinPath("book0", "1"), func(page *KeyPage) {
			page.Keys = append(page.Keys, &KeySpec{Owner: bob.JoinPath("book0", "1")})
		}))
	})
	updateSubnetFor(sim, bob, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key2), types.String(bob.String()), 1e9))
	})

	// Execute
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithBody(&WriteData{
				Entry: DataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			WithSigner(bob.JoinPath("book0", "1"), 1).
			WithDelegator(alice.JoinPath("book0", "1")).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, key2).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	viewSubnetFor(sim, alice, func(batch *database.Batch) {
		data, err := batch.Account(alice.JoinPath("data")).Data()
		require.NoError(t, err)
		de, err := data.Entry(data.Height() - 1)
		require.NoError(t, err)
		require.Equal(t, "foo", string(de.Data[0]))
	})
}

func TestDelegatedSignature_RemoteDelegator(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	alice, bob := url.MustParse("alice"), url.MustParse("bob")
	sim.SetRouteFor(alice, "BVN0")
	sim.SetRouteFor(bob, "BVN1")

	// Setup
	key1, key2, key3 := acctesting.GenerateKey(), acctesting.GenerateKey(), acctesting.GenerateKey()
	updateSubnetFor(sim, alice, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key1), types.String(alice.String()), 1e9))
		require.NoError(t, acctesting.CreateAccount(batch, &DataAccount{Url: alice.JoinPath("data"), AccountAuth: AccountAuth{Authorities: []AuthorityEntry{{Url: bob.JoinPath("book0")}}}}))
	})
	updateSubnetFor(sim, bob, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key2), types.String(bob.String()), 1e9))
		require.NoError(t, acctesting.CreateKeyBook(batch, types.String(bob.JoinPath("book1").String()), tmed25519.PubKey(key3[32:])))
		require.NoError(t, acctesting.AddCredits(batch, bob.JoinPath("book1", "1"), 1e9))
		require.NoError(t, acctesting.UpdateAccount(batch, bob.JoinPath("book0", "1"), func(page *KeyPage) {
			page.Keys = append(page.Keys, &KeySpec{Owner: bob.JoinPath("book1", "1")})
		}))
	})

	// Execute
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithBody(&WriteData{
				Entry: DataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			WithSigner(bob.JoinPath("book1", "1"), 1).
			WithDelegator(bob.JoinPath("book0", "1")).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, key2).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	viewSubnetFor(sim, alice, func(batch *database.Batch) {
		data, err := batch.Account(alice.JoinPath("data")).Data()
		require.NoError(t, err)
		de, err := data.Entry(data.Height() - 1)
		require.NoError(t, err)
		require.Equal(t, "foo", string(de.Data[0]))
	})
}

func TestDelegatedSignature_RemoteDelegateAndAuthority(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	alice, bob, charlie := url.MustParse("alice"), url.MustParse("bob"), url.MustParse("charlie")
	sim.SetRouteFor(alice, "BVN0")
	sim.SetRouteFor(bob, "BVN1")
	sim.SetRouteFor(charlie, "BVN2")

	// Setup
	key1, key2, key3 := acctesting.GenerateKey(), acctesting.GenerateKey(), acctesting.GenerateKey()
	updateSubnetFor(sim, alice, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key1), types.String(alice.String()), 1e9))
		require.NoError(t, acctesting.CreateAccount(batch, &DataAccount{Url: alice.JoinPath("data"), AccountAuth: AccountAuth{Authorities: []AuthorityEntry{{Url: bob.JoinPath("book0")}}}}))
	})
	updateSubnetFor(sim, bob, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key2), types.String(bob.String()), 1e9))
		require.NoError(t, acctesting.UpdateAccount(batch, bob.JoinPath("book0", "1"), func(page *KeyPage) {
			page.Keys = append(page.Keys, &KeySpec{Owner: charlie.JoinPath("book0", "1")})
		}))
	})
	updateSubnetFor(sim, charlie, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key3), types.String(charlie.String()), 1e9))
	})

	// Execute
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithBody(&WriteData{
				Entry: DataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			WithSigner(charlie.JoinPath("book0", "1"), 1).
			WithDelegator(bob.JoinPath("book0", "1")).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, key2).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	viewSubnetFor(sim, alice, func(batch *database.Batch) {
		data, err := batch.Account(alice.JoinPath("data")).Data()
		require.NoError(t, err)
		de, err := data.Entry(data.Height() - 1)
		require.NoError(t, err)
		require.Equal(t, "foo", string(de.Data[0]))
	})
}

func TestDelegatedSignature_DobuleRemote(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	alice, bob, charlie := url.MustParse("alice"), url.MustParse("bob"), url.MustParse("charlie")
	sim.SetRouteFor(alice, "BVN0")
	sim.SetRouteFor(bob, "BVN1")
	sim.SetRouteFor(charlie, "BVN2")

	// Setup
	key1, key2, key3 := acctesting.GenerateKey(), acctesting.GenerateKey(), acctesting.GenerateKey()
	updateSubnetFor(sim, alice, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key1), types.String(alice.String()), 1e9))
		require.NoError(t, acctesting.CreateAccount(batch, &DataAccount{Url: alice.JoinPath("data")}))
		require.NoError(t, acctesting.UpdateAccount(batch, alice.JoinPath("book0", "1"), func(page *KeyPage) {
			page.Keys = append(page.Keys, &KeySpec{Owner: bob.JoinPath("book0", "1")})
		}))
	})
	updateSubnetFor(sim, bob, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key2), types.String(bob.String()), 1e9))
		require.NoError(t, acctesting.UpdateAccount(batch, bob.JoinPath("book0", "1"), func(page *KeyPage) {
			page.Keys = append(page.Keys, &KeySpec{Owner: charlie.JoinPath("book0", "1")})
		}))
	})
	updateSubnetFor(sim, charlie, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key3), types.String(charlie.String()), 1e9))
	})

	// Execute
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithBody(&WriteData{
				Entry: DataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			WithSigner(charlie.JoinPath("book0", "1"), 1).
			WithDelegator(bob.JoinPath("book0", "1")).
			WithDelegator(alice.JoinPath("book0", "1")).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, key2).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	viewSubnetFor(sim, alice, func(batch *database.Batch) {
		data, err := batch.Account(alice.JoinPath("data")).Data()
		require.NoError(t, err)
		de, err := data.Entry(data.Height() - 1)
		require.NoError(t, err)
		require.Equal(t, "foo", string(de.Data[0]))
	})
}

func TestDelegatedSignature_Multisig(t *testing.T) {

}
