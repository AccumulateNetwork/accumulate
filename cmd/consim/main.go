// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// consim runs the DAG-BFT consensus stack in one process, in the soak's
// topology, at millisecond round pacing — so behaviour that takes the Docker
// soak half an hour to reach happens in seconds, and a stall names the
// pipeline stage that stopped. See pkg/consensus/consim.
//
//	go run ./cmd/consim -bvns 3 -vals 4 -tps 20 -height 200
//
// Exit codes: 0 = reached the target/duration; 2 = a partition stalled.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/consim"
)

func main() {
	var cfg consim.Config
	logLevel := flag.String("log", "warn", "consensus log level: debug|info|warn|error (consim's own status always prints)")
	flag.IntVar(&cfg.BVNs, "bvns", 3, "BVN partitions")
	flag.IntVar(&cfg.ValidatorsPerBVN, "vals", 4, "validators per BVN (all serve the Directory too)")
	flag.IntVar(&cfg.TPS, "tps", 20, "transactions per second per partition")
	flag.DurationVar(&cfg.MinRoundInterval, "round", 5*time.Millisecond, "round pacing (soak runs 500ms)")
	flag.DurationVar(&cfg.BatchTimeout, "batch-timeout", 10*time.Millisecond, "worker batch cut interval")
	flag.IntVar(&cfg.BatchSize, "batch-size", 20, "transactions per batch")
	targetHeight := flag.Uint64("height", 200, "stop with success once every partition executes this height (0 = run to -duration); a height is one committed leader GROUP (#4164), ~one per two rounds")
	flag.DurationVar(&cfg.Duration, "duration", 10*time.Minute, "hard cap")
	flag.DurationVar(&cfg.StallAfter, "stall-after", 20*time.Second, "no height progress for this long = stall")
	flag.DurationVar(&cfg.BatchCollect, "batch-collect", 30*time.Second, "CollectBatches bound")
	flag.Parse()
	cfg.TargetHeight = *targetHeight
	cfg.Out = os.Stdout

	// The consensus stack logs via slog; at Info it emits per-vote lines that
	// drown consim's one-line-per-second status. Default to Warn.
	var lv slog.Level
	if err := lv.UnmarshalText([]byte(*logLevel)); err != nil {
		fmt.Fprintln(os.Stderr, "bad -log:", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lv})))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Printf("consim: %d BVNs x %d validators (%d hosts, %d nodes), round=%s, tps=%d/partition, target height %d\n",
		cfg.BVNs, cfg.ValidatorsPerBVN, cfg.BVNs*cfg.ValidatorsPerBVN,
		cfg.BVNs*cfg.ValidatorsPerBVN*(1+1), cfg.MinRoundInterval, cfg.TPS, cfg.TargetHeight)

	sim, err := consim.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(1)
	}
	defer sim.Close()

	res, err := sim.Run(ctx)
	fmt.Printf("result: ok=%v reason=%q elapsed=%s heights=%v\n", res.Ok, res.Reason, res.Elapsed.Truncate(time.Second), res.Heights)
	if err != nil {
		os.Exit(2)
	}
}
