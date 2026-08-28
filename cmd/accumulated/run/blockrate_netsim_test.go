// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestMeasureBlockRate reports how fast a netsim actually produces blocks
// (#4099). It is a measurement, not an assertion, so it is opt-in:
//
//	ACC_MEASURE_BLOCKRATE=90 go test ./cmd/accumulated/run -tags testnet \
//	    -run TestMeasureBlockRate -v -timeout 10m
//
// The value is the sampling window in seconds.
//
// Height is read from the partition's system ledger, whose index is the block
// height. That is deliberate: it reads Accumulate state rather than consensus
// internals, so a CometBFT number and a DAG-BFT number mean the same thing and
// can be compared directly — and it does not depend on routing debug logs out
// of a subnode, which is what defeated the first attempt to measure this.
//
// Why this exists: #4098 reports that Timing.BlockInterval (3s default) is
// validated and never read, while the primary falls back to a hardcoded 100ms
// MinRoundInterval. That claim is from source. This is how it gets a number,
// before and after the fix.
func TestMeasureBlockRate(t *testing.T) {
	window := os.Getenv("ACC_MEASURE_BLOCKRATE")
	if window == "" {
		t.Skip("measurement only; set ACC_MEASURE_BLOCKRATE=<seconds> to run")
	}
	secs, err := strconv.Atoi(window)
	require.NoError(t, err, "ACC_MEASURE_BLOCKRATE must be a number of seconds")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootDir := t.TempDir()
	basePort := freeDevnetBase(t, 1)

	globals := &network.GlobalValues{
		Globals: &protocol.NetworkGlobals{
			// Never during the window: a major block is not what is being
			// measured and only adds variance.
			MajorBlockSchedule: "0 0 1 1 *",
		},
	}

	cfg := &Config{
		Network: "BlockRate",
		Logging: &Logging{
			Format: "plain",
			Rules:  []*LoggingRule{{Level: slog.LevelError}},
		},
		P2P: &P2P{
			Key: &PrivateKeySeed{Seed: record.NewKey("measure-block-rate")},
		},
		Configurations: []Configuration{
			&NetSimConfiguration{
				Listen:     multiaddr.StringCast(fmt.Sprintf("/tcp/%d", basePort)),
				Bvns:       1,
				Validators: 1,
				Globals:    globals,
			},
		},
	}
	cfg.file = rootDir + "/accumulate.toml"

	inst, err := New(ctx, cfg)
	require.NoError(t, err)
	inst.rootDir = rootDir
	require.NoError(t, inst.Start())
	defer inst.Stop()

	api := fmt.Sprintf("http://127.0.0.1:%d/v3", basePort+int(portAccAPI))

	// Wait for the API before starting the clock, so start-up is not counted
	// as a slow block.
	var ready bool
	for i := 0; i < 60; i++ {
		if _, err := ledgerIndex(api, "dn"); err == nil {
			ready = true
			break
		}
		time.Sleep(time.Second)
	}
	require.True(t, ready, "API never became ready")

	type sample struct {
		at time.Time
		h  uint64
	}
	firstS := map[string]sample{}
	lastS := map[string]sample{}
	parts := []string{"dn", "bvn-BVN1"}

	deadline := time.Now().Add(time.Duration(secs) * time.Second)
	for time.Now().Before(deadline) {
		for _, p := range parts {
			h, err := ledgerIndex(api, p)
			if err != nil {
				continue
			}
			s := sample{at: time.Now(), h: h}
			if _, ok := firstS[p]; !ok {
				firstS[p] = s
			}
			lastS[p] = s
		}
		time.Sleep(2 * time.Second)
	}

	t.Logf("=== block rate over %ds ===", secs)
	for _, p := range parts {
		f, ok1 := firstS[p]
		l, ok2 := lastS[p]
		if !ok1 || !ok2 {
			t.Logf("  %-10s unreachable", p)
			continue
		}
		span := l.at.Sub(f.at).Seconds()
		blocks := int64(l.h) - int64(f.h)
		if span <= 0 {
			t.Logf("  %-10s window too short", p)
			continue
		}
		rate := float64(blocks) / span
		per := 0.0
		if blocks > 0 {
			per = span / float64(blocks)
		}
		t.Logf("  %-10s %d blocks in %.1fs  =  %.3f blocks/sec  (%.2fs per block)",
			p, blocks, span, rate, per)
	}
	t.Logf("configured intent: BlockInterval default %v (see #4098)", 3*time.Second)
}

// ledgerIndex returns a partition's block height, read from its system ledger.
func ledgerIndex(api, part string) (uint64, error) {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "query",
		"params": map[string]any{"scope": fmt.Sprintf("acc://%s.acme/ledger", part)},
	})
	resp, err := http.Post(api, "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var out struct {
		Result struct {
			Account struct {
				Index uint64 `json:"index"`
			} `json:"account"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, err
	}
	if out.Result.Account.Index == 0 {
		return 0, fmt.Errorf("no ledger index yet")
	}
	return out.Result.Account.Index, nil
}
