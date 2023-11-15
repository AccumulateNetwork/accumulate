// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator_test

import (
	"context"
	"flag"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
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
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(123, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// Verify
	account := GetAccount[*TokenAccount](t, sim.DatabaseFor(bob), bob.JoinPath("tokens"))
	require.Equal(t, 123, int(account.Balance.Int64()))
}

// TestSimulator2 tests the simulator asynchronously
func TestSimulator2(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	liteKey := acctesting.GenerateKey(t.Name())
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	other := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey(t.Name(), "other"))

	// Initialize
	sim := NewSim(t,
		simulator.LocalNetwork(t.Name(), 3, 3, net.ParseIP("127.0.1.1"), 12345),
		simulator.Genesis(GenesisTime),
	)

	// Fund the LTA - no need to wait since the simulator uses a fake faucet
	_, err := sim.S.Services().Faucet(ctx, lite, api.FaucetOptions{})
	require.NoError(t, err)

	// Launch HTTP servers
	err = sim.S.ListenAndServe(ctx, simulator.ListenOptions{
		ListenHTTPv3: true,
		ServeError:   func(err error) { t.Log(err) },
	})
	require.NoError(t, err)

	// Tick the simulator - capture errors and feed them to this goroutine
	didFail := make(chan any, 2)
	defer func() {
		for err := range didFail {
			t.Log(err)
			t.Fail()
		}
	}()

	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	go func() {
		defer close(didFail)
		defer func() {
			if r := recover(); r != nil {
				didFail <- r
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				err := sim.S.Step()
				if err != nil {
					didFail <- err
				}
			}
		}
	}()

	// Stop as soon as the tests below are done
	defer cancel()

	// Create a new HTTP client
	C := jsonrpc.NewClient("http://127.0.1.1:12349/v3")

	ns, err := C.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: Directory})
	require.NoError(t, err)

	// Buy credits
	st := buildAndSubmit(t, ctx, C,
		build.Transaction().For(lite).
			AddCredits().To(lite).WithOracle(float64(ns.Oracle.Price)/AcmeOraclePrecision).Purchase(3).
			SignWith(lite).Version(1).Timestamp(1).PrivateKey(liteKey))

	for !Txn(st.TxID).Completes().Satisfied(&sim.Harness) {
		// Wait for the transaction to complete
		time.Sleep(10 * time.Millisecond)
	}

	// Send tokens
	st = buildAndSubmit(t, ctx, C,
		build.Transaction().For(lite).
			SendTokens(1, 0).To(other).
			SignWith(lite).Version(1).Timestamp(2).PrivateKey(liteKey))

	for !Txn(st.TxID).Completes().Satisfied(&sim.Harness) {
		// Wait for the transaction to complete
		time.Sleep(10 * time.Millisecond)
	}
}

func buildAndSubmit(t testing.TB, ctx context.Context, svc api.Submitter, bld EnvelopeBuilder) *TransactionStatus {
	env, err := bld.Done()
	require.NoError(t, err)

	subs, err := svc.Submit(ctx, env, api.SubmitOptions{})
	require.NoError(t, err)

	for _, sub := range subs {
		require.NoError(t, sub.Status.AsError())
	}
	return subs[0].Status
}

func TestSimulatorFaucet(t *testing.T) {
	lite := acctesting.GenerateKey(t.Name(), "Lite")
	liteUrl := acctesting.AcmeLiteAddressStdPriv(lite)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	// Execute
	sub, err := sim.S.Services().Faucet(context.Background(), liteUrl, api.FaucetOptions{})
	require.NoError(t, err)
	require.True(t, sub.Success)

	// Verify
	account := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(liteUrl), liteUrl)
	require.NotZero(t, account.Balance.Int64())
}

// func TestSimulatorWithABCI(t *testing.T) {
// 	alice := url.MustParse("alice")
// 	bob := url.MustParse("bob")
// 	aliceKey := acctesting.GenerateKey(alice)
// 	bobKey := acctesting.GenerateKey(bob)

// 	// Initialize
// 	sim := NewSim(t,
// 		simulator.SimpleNetwork(t.Name(), 3, 3),
// 		simulator.Genesis(GenesisTime),
// 		simulator.UseABCI,
// 	)

// 	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
// 	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
// 	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
// 	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
// 	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
// 	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

// 	// Execute
// 	st := sim.BuildAndSubmitTxnSuccessfully(
// 		build.Transaction().For(alice, "tokens").
// 			SendTokens(123, 0).To(bob, "tokens").
// 			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

// 	sim.StepUntil(
// 		Txn(st.TxID).Succeeds(),
// 		Txn(st.TxID).Produced().Succeeds())

// 	// Verify
// 	account := GetAccount[*TokenAccount](t, sim.DatabaseFor(bob), bob.JoinPath("tokens"))
// 	require.Equal(t, 123, int(account.Balance.Int64()))
// }

var flagRecording = flag.String("test.dump-recording", "", "Recording to dump")

func TestDumpRecording(t *testing.T) {
	if *flagRecording == "" {
		t.Skip()
	}

	f, err := os.Open(*flagRecording)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, simulator.DumpRecording(f))
}
