// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestNetSimMultiValidatorAdvancesBlocks starts a netsim whose partitions have
// more than one validator and requires every partition to keep producing
// blocks (#4392). Each sub-node's DAG-BFT service needs its own libp2p host
// and GossipSub router; without them a committee of two never reaches quorum
// ("Not enough stake for certificate") and the ledgers stay at genesis.
func TestNetSimMultiValidatorAdvancesBlocks(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a multi-validator netsim")
	}

	cases := []struct {
		Name       string
		Bvns       uint64
		Validators uint64
	}{
		{"Two validators per partition", 1, 2},
		{"Two BVNs", 2, 1},
		{"Two validators and two BVNs", 2, 2},
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			rootDir := t.TempDir()
			basePort := freeDevnetBase(t, 1)

			cfg := &Config{
				Network: "MultiValidator",
				Logging: &Logging{
					Format: "plain",
					Rules:  []*LoggingRule{{Level: slog.LevelError}},
				},
				P2P: &P2P{
					Key: &PrivateKeySeed{Seed: record.NewKey("test-netsim-multi-validator", c.Name)},
				},
				Configurations: []Configuration{
					&NetSimConfiguration{
						Listen:     multiaddr.StringCast(fmt.Sprintf("/tcp/%d", basePort)),
						Bvns:       c.Bvns,
						Validators: c.Validators,
						Globals: &network.GlobalValues{
							Globals: &protocol.NetworkGlobals{MajorBlockSchedule: "0 0 1 1 *"},
						},
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
			const wantHeight = 5
			parts := []string{"dn"}
			for i := uint64(1); i <= c.Bvns; i++ {
				parts = append(parts, fmt.Sprintf("bvn-BVN%d", i))
			}
			require.Eventually(t, func() bool {
				for _, p := range parts {
					h, err := ledgerIndex(api, p)
					if err != nil || h < wantHeight {
						return false
					}
				}
				return true
			}, 90*time.Second, time.Second, "every partition should reach block %d", wantHeight)
		})
	}
}
