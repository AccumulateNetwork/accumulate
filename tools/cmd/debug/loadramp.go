// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// loadramp runs a paced load test where the source key is also used
// for signing. Unlike `loadtest`, it does NOT call any faucet service —
// the supplied address must already hold ACME and credits. It paces
// submissions via a ticker (true target TPS, not "as fast as possible")
// and tolerates pushback (mempool full, broadcast errors) so a ramp
// harness can drive it through the breaking point.

var cmdLoadRamp = &cobra.Command{
	Use:   "loadramp [server] [address]",
	Short: "Paced load test that uses the supplied account as both funder and signer",
	Args:  cobra.ExactArgs(2),
	Run:   loadRamp,
}

var loadRampOpts struct {
	tps      int
	duration time.Duration
	interval time.Duration
	batch    int
}

func init() {
	cmdLoadRamp.Flags().IntVar(&loadRampOpts.tps, "tps", 5, "Target transactions per second")
	cmdLoadRamp.Flags().DurationVar(&loadRampOpts.duration, "duration", 30*time.Second, "Test duration at target TPS")
	cmdLoadRamp.Flags().DurationVar(&loadRampOpts.interval, "report-interval", 5*time.Second, "How often to print live metrics")
	cmdLoadRamp.Flags().IntVar(&loadRampOpts.batch, "batch", 1, "Transactions per submit call (1 = no batching)")
	cmd.AddCommand(cmdLoadRamp)
}

func loadRamp(_ *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(args[0], "v3"))
	server.Client.Timeout = 30 * time.Second

	addr, err := address.Parse(args[1])
	checkf(err, "address")
	sk, ok := addr.GetPrivateKey()
	if !ok {
		fatalf("address must be a private key (e.g. AS1...)")
	}
	kh, ok := addr.GetPublicKeyHash()
	if !ok {
		fatalf("address has no public key hash")
	}

	lid := protocol.LiteAuthorityForHash(kh)
	lta := lid.JoinPath("ACME")

	Q := api.Querier2{Querier: server}
	var account *protocol.LiteTokenAccount
	_, err = Q.QueryAccountAs(ctx, lta, nil, &account)
	checkf(err, "query lite token account %v (must be pre-funded)", lta)

	var lidAcct *protocol.LiteIdentity
	_, err = Q.QueryAccountAs(ctx, lid, nil, &lidAcct)
	checkf(err, "query lite identity %v", lid)

	if account.TokenBalance().Sign() == 0 {
		fatalf("source account %v has zero ACME balance", lta)
	}
	if lidAcct.CreditBalance == 0 {
		fatalf("source identity %v has zero credits", lid)
	}

	fmt.Printf("Source:       %v\n", lta)
	fmt.Printf("ACME balance: %v\n", account.TokenBalance())
	fmt.Printf("Credits:      %d\n", lidAcct.CreditBalance)

	entry := new(protocol.DoubleHashDataEntry)
	entry.Data = append(entry.Data, []byte("loadramp"))
	entry.Data = append(entry.Data, []byte(time.Now().Format(time.RFC3339Nano)))
	chainId := protocol.ComputeLiteDataAccountId(entry)
	lda, err := protocol.LiteDataAddress(chainId)
	check(err)

	tps := loadRampOpts.tps
	if tps < 1 {
		fatalf("tps must be >= 1")
	}
	batch := loadRampOpts.batch
	if batch < 1 {
		batch = 1
	}
	if batch > tps {
		batch = tps
	}

	// Network status before
	startStatus, err := server.NetworkStatus(ctx, api.NetworkStatusOptions{})
	checkf(err, "network status")
	startHeight := uint64(0)
	if startStatus.DirectoryHeight != 0 {
		startHeight = startStatus.DirectoryHeight
	}

	// Counters
	var submitted, succeeded, mempoolFull, otherErr atomic.Uint64
	startNonce := uint64(time.Now().UTC().UnixMilli())

	// Dispatcher: builds + submits at the target TPS
	tickInterval := time.Second / time.Duration(tps/batch)
	if tickInterval <= 0 {
		tickInterval = time.Millisecond
	}

	stop := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()
		nonce := startNonce
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
			}

			// Build a batch — each iteration is ONE transaction (one
			// WriteData + its signature, normalized into 2+ messages).
			msgs := make([]messaging.Message, 0, batch*2)
			built := 0
			for i := 0; i < batch; i++ {
				nonce++
				env, err := build.Transaction().
					For(lda).
					WriteData(entry).
					SignWith(lid).Version(1).Timestamp(&nonce).PrivateKey(sk).
					Done()
				if err != nil {
					otherErr.Add(1)
					continue
				}
				m, err := env.Normalize()
				if err != nil {
					otherErr.Add(1)
					continue
				}
				msgs = append(msgs, m...)
				built++
			}
			if built == 0 {
				continue
			}

			submitted.Add(uint64(built))
			subs, err := server.Submit(ctx, &messaging.Envelope{Messages: msgs}, api.SubmitOptions{})
			if err != nil {
				if isMempoolFull(err) {
					mempoolFull.Add(uint64(built))
				} else {
					otherErr.Add(uint64(built))
				}
				continue
			}
			// `subs` has one entry per message; count whole-envelope success
			// (every message must be Success). All-or-nothing per envelope.
			anyFailed := false
			for _, s := range subs {
				if s.Success {
					continue
				}
				anyFailed = true
				if s.Status != nil && s.Status.Error != nil && isMempoolFull(s.Status.Error) {
					mempoolFull.Add(1)
				}
			}
			if !anyFailed {
				succeeded.Add(uint64(built))
			} else {
				otherErr.Add(uint64(built))
			}
		}
	}()

	// Reporter
	deadline := time.Now().Add(loadRampOpts.duration)
	reportTicker := time.NewTicker(loadRampOpts.interval)
	defer reportTicker.Stop()

	var lastSub, lastSucc uint64
	var lastT = time.Now()

	fmt.Printf("Running %d TPS for %v (batch=%d, tick=%v)\n", tps, loadRampOpts.duration, batch, tickInterval)
loop:
	for {
		t := <-reportTicker.C
		s := submitted.Load()
		k := succeeded.Load()
		mf := mempoolFull.Load()
		oe := otherErr.Load()
		dt := t.Sub(lastT).Seconds()
		subRate := float64(s-lastSub) / dt
		okRate := float64(k-lastSucc) / dt
		fmt.Printf("  submitted=%6d (%.1f/s)  ok=%6d (%.1f/s)  mempoolFull=%d  otherErr=%d\n",
			s, subRate, k, okRate, mf, oe)
		lastSub, lastSucc, lastT = s, k, t
		if t.After(deadline) {
			break loop
		}
	}

	close(stop)
	<-done

	// Final status
	endStatus, err := server.NetworkStatus(ctx, api.NetworkStatusOptions{})
	endHeight := startHeight
	if err == nil && endStatus.DirectoryHeight != 0 {
		endHeight = endStatus.DirectoryHeight
	}

	s := submitted.Load()
	k := succeeded.Load()
	mf := mempoolFull.Load()
	oe := otherErr.Load()
	elapsed := loadRampOpts.duration.Seconds()

	fmt.Println()
	fmt.Println("=== summary ===")
	fmt.Printf("Target TPS:          %d\n", tps)
	fmt.Printf("Duration:            %v\n", loadRampOpts.duration)
	fmt.Printf("Submitted total:     %d  (%.1f/s)\n", s, float64(s)/elapsed)
	fmt.Printf("Submit-success:      %d  (%.1f/s)\n", k, float64(k)/elapsed)
	fmt.Printf("Mempool full:        %d\n", mf)
	fmt.Printf("Other errors:        %d\n", oe)
	fmt.Printf("DN height start->end: %d -> %d  (Δ%d)\n", startHeight, endHeight, endHeight-startHeight)

	// Exit code: non-zero if breakage
	broken := false
	switch {
	case s == 0:
		broken = true
		fmt.Println("BROKEN: nothing submitted")
	case float64(k)/float64(s) < 0.95:
		broken = true
		fmt.Printf("BROKEN: success rate %.1f%% < 95%%\n", 100*float64(k)/float64(s))
	case mf > s/20:
		broken = true
		fmt.Printf("BROKEN: mempool-full rate %.1f%% > 5%%\n", 100*float64(mf)/float64(s))
	case endHeight == startHeight:
		broken = true
		fmt.Println("BROKEN: network height did not advance")
	}
	if broken {
		os.Exit(2)
	}
}

func isMempoolFull(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "mempool is full") || strings.Contains(s, "mempool: full")
}
