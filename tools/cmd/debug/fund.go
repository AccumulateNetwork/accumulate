// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// fund mints a fresh lite account funded from an existing faucet account and
// adds credits, then prints its AS1 key. Used to create an independent funded
// account (e.g. so loadramp can flood one account's partition while loadmix
// drives the faucet — they cannot share a signer without nonce collisions).
var cmdFund = &cobra.Command{
	Use:   "fund [server] [faucet-address]",
	Short: "Fund a fresh lite account from a faucet account, add credits, print its AS1 key",
	Args:  cobra.ExactArgs(2),
	Run:   runFund,
}

var fundOpts struct {
	acme    uint64
	credits float64
}

func init() {
	cmdFund.Flags().Uint64Var(&fundOpts.acme, "acme", 1000, "ACME to send to the new account")
	cmdFund.Flags().Float64Var(&fundOpts.credits, "credits", 100, "ACME to spend on credits for the new account")
	cmd.AddCommand(cmdFund)
}

func runFund(_ *cobra.Command, args []string) {
	ctx := context.Background()
	server := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(args[0], "v3"))
	server.Client.Timeout = 30 * time.Second

	faddr, err := address.Parse(args[1])
	checkf(err, "faucet address")
	fsk, ok := faddr.GetPrivateKey()
	if !ok {
		fatalf("faucet address must be a private key (AS1...)")
	}
	fkh, _ := faddr.GetPublicKeyHash()
	faucetLid := protocol.LiteAuthorityForHash(fkh)
	faucetLta := faucetLid.JoinPath("ACME")

	ns, err := server.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	checkf(err, "network status")
	router := routing.NewRouter(routing.RouterOptions{Initial: ns.Routing})
	oracle := float64(ns.Oracle.Price) / protocol.AcmeOraclePrecision
	pc := buildPartClient(ctx, args[0], router, server)

	pub, sk, err := ed25519.GenerateKey(rand.Reader)
	check(err)
	lid := protocol.LiteAuthorityForKey(pub, protocol.SignatureTypeED25519)
	lta := lid.JoinPath("ACME")

	// Send ACME from the faucet to the new account (synthetic deposit).
	nonce := uint64(time.Now().UTC().UnixNano())
	env, err := build.Transaction().For(faucetLta).
		SendTokens(fundOpts.acme, protocol.AcmePrecisionPower).To(lta).
		SignWith(faucetLid).Version(1).Timestamp(&nonce).PrivateKey(fsk).Done()
	check(err)
	fc, _ := pc.clientFor(faucetLta)
	_, err = fc.Submit(ctx, env, api.SubmitOptions{})
	check(err)
	fmt.Printf("Funding %v with %d ACME...\n", lta, fundOpts.acme)
	waitFund(ctx, pc, lta)

	// Buy credits for the new lite identity from its own ACME.
	nonce2 := uint64(time.Now().UTC().UnixNano())
	env2, err := build.Transaction().For(lta).
		AddCredits().To(lid).Spend(fundOpts.credits).WithOracle(oracle).
		SignWith(lid).Version(1).Timestamp(&nonce2).PrivateKey(sk).Done()
	check(err)
	lc, _ := pc.clientFor(lta)
	subs, err := lc.Submit(ctx, env2, api.SubmitOptions{})
	check(err)
	for _, s := range subs {
		if !s.Success && s.Status != nil && s.Status.Error != nil {
			check(s.Status.Error)
		}
	}
	waitCredits(ctx, pc, lid)

	priv := &address.PrivateKey{PublicKey: address.PublicKey{Type: protocol.SignatureTypeED25519, Key: pub}, Key: sk}
	fmt.Println("Funded account ready:")
	fmt.Println(priv.String())
	fmt.Println(lta)
}

func waitFund(ctx context.Context, pc *partClient, lta *url.URL) {
	q := pc.querier(lta)
	for i := 0; i < 60; i++ {
		var a *protocol.LiteTokenAccount
		_, err := q.QueryAccountAs(ctx, lta, nil, &a)
		if err == nil && a.TokenBalance().Sign() > 0 {
			return
		}
		time.Sleep(time.Second)
	}
	fatalf("timed out waiting for %v to be funded", lta)
}

func waitCredits(ctx context.Context, pc *partClient, lid *url.URL) {
	q := pc.querier(lid)
	for i := 0; i < 60; i++ {
		var a *protocol.LiteIdentity
		_, err := q.QueryAccountAs(ctx, lid, nil, &a)
		if err == nil && a.CreditBalance > 0 {
			return
		}
		time.Sleep(time.Second)
	}
	fatalf("timed out waiting for %v credits", lid)
}
