// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// cmdBootstrap launches a node via the v2-corrected (DN, BVN)-pair
// bootstrap. After the phase-5 pipeline rewrite, this command is
// awaiting its phase-6 wiring (proper BVNAnchorFromDN
// implementation, two-database setup, flag rework). The previous
// implementation targeted the wrong-shaped first-draft v2 model and
// has been removed to prevent operators from invoking a path the
// pipeline no longer supports.
//
// See docs/plans/bootstrap-v2.md for the corrected model.
var cmdBootstrap = &cobra.Command{
	Use:   "bootstrap",
	Short: "Launch a node from minimum-data state (v2; rewrite-in-progress)",
	Long: `Bootstrap a node via the v2 (DN, BVN)-pair anchor walk.

Status: this command is currently a stub. The bootstrap pipeline
itself is operational (see internal/core/bootstrap/pipeline) but
the operator-facing wiring — opening DN + BVN databases, building
initial validator sets, providing a BVNAnchorFromDN implementation
that reads the trusted DN state, persisting the new-shape
artifact — is being rewritten on the bootstrap-v2 branch.

For the design, see docs/plans/bootstrap-v2.md.`,
	RunE: runBootstrap,
}

var flagBootstrap struct {
	Network              string
	DataDir              string
	BVN                  string
	GenesisStateTreeAnchor string
}

func init() {
	cmdMain.AddCommand(cmdBootstrap)

	f := cmdBootstrap.Flags()
	f.StringVar(&flagBootstrap.Network, "network", "", "network: mainnet | testnet | devnet | <endpoint>")
	f.StringVar(&flagBootstrap.DataDir, "data-dir", "", "data directory (default ~/.accumulated)")
	f.StringVar(&flagBootstrap.BVN, "bvn", "", "BVN partition this node will run (e.g., Apollo)")
	f.StringVar(&flagBootstrap.GenesisStateTreeAnchor, "genesis-state-tree-anchor", "", "override binary pin: DN's StateTreeAnchor at major-block 1 (64 hex chars)")
}

func runBootstrap(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("accumulated bootstrap: phase-6 rewrite not yet wired; see docs/plans/bootstrap-v2.md and the bootstrap-v2 branch")
}
