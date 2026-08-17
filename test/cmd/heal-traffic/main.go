// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Traffic generator for the #4056 anchor-healing network test: sends ACME
// from the deterministic genesis faucet to fresh lite accounts to keep the
// anchor cascade running.
package main

import (
	"context"
	"crypto/ed25519"
	"flag"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func main() {
	server := flag.String("s", "http://127.0.0.1:26660/v3", "API v3 endpoint")
	interval := flag.Duration("i", 2*time.Second, "send interval")
	duration := flag.Duration("d", 10*time.Minute, "how long to run")
	flag.Parse()

	var seed storage.Key
	seed = seed.Append("FAUCET")
	sk := ed25519.NewKeyFromSeed(seed[:])
	faucetUrl, err := protocol.LiteTokenAddress(sk[32:], "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		panic(err)
	}
	fmt.Println("faucet:", faucetUrl)

	client := jsonrpc.NewClient(*server)
	timestamp := uint64(time.Now().UnixMilli())
	deadline := time.Now().Add(*duration)

	for n := 0; time.Now().Before(deadline); n++ {
		dest := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("heal-traffic", n))
		env, err := build.Transaction().For(faucetUrl).
			SendTokens(1, protocol.AcmePrecisionPower).To(dest).
			SignWith(faucetUrl).Version(1).Timestamp(&timestamp).PrivateKey(sk).
			Done()
		if err != nil {
			fmt.Println("build:", err)
			continue
		}
		subs, err := client.Submit(context.Background(), env, v3.SubmitOptions{})
		if err != nil {
			fmt.Println("submit error:", err)
		} else {
			for _, sub := range subs {
				if sub.Status != nil && sub.Status.Error != nil {
					fmt.Println("status error:", sub.Status.Error)
				}
			}
			if n%10 == 0 {
				fmt.Println("sent", n+1, "to", dest)
			}
		}
		time.Sleep(*interval)
	}
}
