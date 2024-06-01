// Copyright 2024 The Accumulate Authors
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
	"strconv"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdLoadTest = &cobra.Command{
	Use:   "loadtest [server] [tps] [address]",
	Short: "Load test Accumulate",
	Args:  cobra.RangeArgs(2, 3),
	Run:   loadTest,
}

func init() {
	cmd.AddCommand(cmdLoadTest)
}

func loadTest(_ *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(args[0], "v3"))
	server.Client.Timeout = time.Minute

	tps, err := strconv.ParseUint(args[1], 10, 64)
	checkf(err, "tps")

	var addr address.Address
	if len(args) > 2 {
		addr, err = address.Parse(args[2])
		checkf(err, "address")
	} else {
		pk, sk, err := ed25519.GenerateKey(rand.Reader)
		check(err)
		addr = &address.PrivateKey{
			PublicKey: address.PublicKey{
				Type: protocol.SignatureTypeED25519,
				Key:  pk,
			},
			Key: sk,
		}
		fmt.Println("Generated", addr)
	}

	sk, ok := addr.GetPrivateKey()
	if !ok {
		fatalf("address is not a private key")
	}
	kh, ok := addr.GetPublicKeyHash()
	if !ok {
		panic("have private key but no hash?!?")
	}

	// Check the account
	Q := api.Querier2{Querier: server}
	var account *protocol.LiteTokenAccount
	_, err = Q.QueryAccountAs(ctx, protocol.LiteAuthorityForHash(kh).JoinPath("ACME"), nil, &account)
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		account = &protocol.LiteTokenAccount{
			Url: protocol.LiteAuthorityForHash(kh).JoinPath("ACME"),
		}
	default:
		check(err)
	}

	// Ensure a balance
	if account.TokenBalance().Sign() == 0 {
		sub, err := server.Faucet(ctx, account.Url, api.FaucetOptions{Token: protocol.AcmeUrl()})
		check(err)

		fmt.Printf("Waiting for faucet transaction %v to complete\n", sub.Status.TxID)
		waitForMessages(ctx, Q, map[[32]byte]struct{}{
			sub.Status.TxID.Hash(): {},
		})
	}

	var lid *protocol.LiteIdentity
	_, err = Q.QueryAccountAs(ctx, account.Url.RootIdentity(), nil, &lid)
	check(err)

	// Ensure credits
	nonce := uint64(time.Now().UTC().UnixMilli())
	if lid.CreditBalance == 0 {
		ns, err := server.NetworkStatus(ctx, api.NetworkStatusOptions{})
		check(err)
		env, err := build.Transaction().
			For(account.Url).
			AddCredits().To(lid.Url).Spend(1).
			WithOracle(float64(ns.Oracle.Price) / protocol.AcmeOraclePrecision).
			SignWith(lid.Url).Version(1).Timestamp(&nonce).PrivateKey(sk).
			Done()
		check(err)
		subs, err := server.Submit(ctx, env, api.SubmitOptions{})
		check(err)
		ids := map[[32]byte]struct{}{}
		for _, sub := range subs {
			if sub.Success {
				ids[sub.Status.TxID.Hash()] = struct{}{}
			} else if sub.Status != nil && sub.Status.Error != nil {
				check(sub.Status.Error)
			} else {
				fatalf(sub.Message)
			}
		}
		waitForMessages(ctx, Q, ids)
	}

	entry := new(protocol.DoubleHashDataEntry)
	entry.Data = append(entry.Data, []byte{})
	entry.Data = append(entry.Data, []byte("Factom PRO"))
	entry.Data = append(entry.Data, []byte("Tutorial"))
	chainId := protocol.ComputeLiteDataAccountId(entry)
	lda, err := protocol.LiteDataAddress(chainId)
	check(err)

	// Build envelopes
	toSend := make(chan *messaging.Envelope, tps)
	for i := uint64(0); i < tps; i++ {
		go func() {
			for {
				env, err := build.Transaction().
					For(lda).
					WriteData(entry).
					SignWith(lid.Url).Version(1).Timestamp(&nonce).PrivateKey(sk).
					Done()
				check(err)
				toSend <- env
			}
		}()
	}

	// Submit envelopes
	var count atomic.Uint64
	go func() {
		var messages []messaging.Message
		for {
			for i := uint64(0); i < tps; i++ {
				env := <-toSend
				msg, err := env.Normalize()
				check(err)
				messages = append(messages, msg...)
			}

			subs, err := server.Submit(ctx, &messaging.Envelope{Messages: messages}, api.SubmitOptions{})
			check(err)
			for _, sub := range subs {
				if sub.Success {
					// Ok
				} else if sub.Status != nil && sub.Status.Error != nil {
					check(sub.Status.Error)
				} else {
					fatalf(sub.Message)
				}
			}

			count.Add(uint64(len(messages)))
			messages = messages[:0]
		}
	}()

	lastTime := time.Now()
	lastCount := count.Load()
	for range time.Tick(time.Minute) {
		c := count.Load()
		if c == lastCount {
			continue
		}

		t := time.Now()
		fmt.Printf("Messages: %.2f/s\n", float64(c-lastCount)/t.Sub(lastTime).Seconds())
		lastTime, lastCount = t, c
	}
}

func waitForMessages(ctx context.Context, Q api.Querier2, waitingFor map[[32]byte]struct{}) {
	for len(waitingFor) > 0 {
		// Avoid modifying the map while iterating over it
		C := make([][32]byte, 0, len(waitingFor))
		for hash := range waitingFor {
			C = append(C, hash)
		}
		for _, hash := range C {
			r, err := Q.QueryMessage(ctx, protocol.UnknownUrl().WithTxID(hash), nil)
			switch {
			case err == nil:
				// Ok
			case errors.Is(err, errors.NotFound):
				time.Sleep(time.Second)
				continue
			default:
				check(err)
			}

			if !r.Status.Delivered() {
				time.Sleep(time.Second)
				continue
			}
			if r.Error != nil {
				check(r.AsError())
			}

			delete(waitingFor, hash)
			if r.Produced != nil {
				for _, prod := range r.Produced.Records {
					fmt.Printf("Waiting for synthetic message %v\n", prod.Value)
					waitingFor[prod.Value.Hash()] = struct{}{}
				}
			}
		}
	}
}
