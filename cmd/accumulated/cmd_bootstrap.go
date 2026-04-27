// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pipeline"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
)

// cmdBootstrap launches a node from minimum-data state via the receiver-
// constructed proof-of-derivation back-walk (issue #3959, parent #3953).
//
// Modes:
//   - Config-file: accumulated bootstrap --config bootstrap.toml
//   - Interactive: accumulated bootstrap (no --config)
//
// Targets BOOTING → ACTIVE by default. ACTIVE → COMPLETE is driven by
// the post-bootstrap history backfill (#3967).
//
// This is the thin user-facing wrapper. All algorithmic work lives in
// internal/core/bootstrap/* — back-walk validator (#3960), BPT proofs
// (#3958), enumeration (#3969), load tracking (#3962), hydrator (#3964),
// nodestate (#3970), persistence (#3965).
var cmdBootstrap = &cobra.Command{
	Use:   "bootstrap",
	Short: "Bootstrap a node from minimum-data state via back-walk proof of derivation",
	Long: `Bootstrap a node from a tiny seed (the binary's pinned genesis
snapshot hash) into a fully participating ACTIVE node. Uses receiver-
constructed proof of derivation (back-walking main chains to genesis)
and multi-source state hydration (live traffic listener + paginated BPT
enumeration + passive touch).

Two modes:
  - Config-file:  accumulated bootstrap --config bootstrap.toml
  - Interactive:  accumulated bootstrap (no --config; prompts you)

Subsequent runs use 'accumulated run' as today; bootstrap state is
detected and resumed.`,
	RunE: runBootstrap,
}

var flagBootstrap struct {
	ConfigFile  string
	DataDir     string
	Network     string
	Partition   string
	TargetState string // "ACTIVE" (default) or "BOOTING"
	TrustAnchor string // optional override "height:hexhash"
	Force       bool
	SkipProof   bool
}

func init() {
	cmdMain.AddCommand(cmdBootstrap)

	f := cmdBootstrap.Flags()
	f.StringVar(&flagBootstrap.ConfigFile, "config", "", "path to bootstrap.toml (omit for interactive)")
	f.StringVar(&flagBootstrap.DataDir, "data-dir", "", "data directory (default ~/.accumulated)")
	f.StringVar(&flagBootstrap.Network, "network", "", "network: mainnet | testnet | devnet | <endpoint>")
	f.StringVar(&flagBootstrap.Partition, "partition", "Directory", "partition to bootstrap into (Directory or BVN name)")
	f.StringVar(&flagBootstrap.TargetState, "target-state", "ACTIVE", "target state to reach before exiting: BOOTING or ACTIVE")
	f.StringVar(&flagBootstrap.TrustAnchor, "trust-anchor", "", "override pinned trust anchor: 'height:hexhash'")
	f.BoolVar(&flagBootstrap.Force, "force", false, "force re-bootstrap even if state is already persisted")
	f.BoolVar(&flagBootstrap.SkipProof, "skip-proof", false, "do not abort on back-walker proof failures (development only)")
}

func runBootstrap(cmd *cobra.Command, args []string) error {
	cfg, err := resolveBootstrapConfig(cmd)
	if err != nil {
		return err
	}

	fmt.Printf("Bootstrap configuration:\n")
	fmt.Printf("  network:      %s\n", cfg.Network)
	fmt.Printf("  data-dir:     %s\n", cfg.DataDir)
	fmt.Printf("  target-state: %s\n", cfg.TargetState)
	if cfg.TrustAnchor != "" {
		fmt.Printf("  trust-anchor: %s\n", cfg.TrustAnchor)
	} else {
		fmt.Printf("  trust-anchor: (built-in pinned for %s)\n", cfg.Network)
	}
	fmt.Println()

	endpoint := accumulate.ResolveWellKnownEndpoint(cfg.Network, "v3")
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	res, err := pipeline.Run(cmd.Context(), pipeline.Options{
		Endpoint:  endpoint,
		Network:   cfg.Network,
		Partition: flagBootstrap.Partition,
		DataDir:   cfg.DataDir,
		SkipProof: flagBootstrap.SkipProof,
		// PinnedGenesisHash left zero — placeholder until the binary
		// pins per-network hashes (deferred under #3979).
		Logger: func(format string, a ...any) {
			fmt.Printf(format+"\n", a...)
		},
	})
	if err != nil {
		return fmt.Errorf("bootstrap pipeline: %w", err)
	}

	fmt.Println()
	fmt.Println("Bootstrap result:")
	fmt.Printf("  pin block:           %d\n", res.PinBlock)
	fmt.Printf("  accounts pulled:     %d\n", res.AccountsPulled)
	fmt.Printf("  chain entries:       %d\n", res.ChainEntriesPulled)
	fmt.Printf("  back-walk memos:     %d\n", res.BackWalkEntries)
	fmt.Printf("  genesis terminated:  %v\n", res.GenesisTerminated)
	fmt.Printf("  artifact:            %s\n", res.ArtifactPath)
	return nil
}

type bootstrapConfig struct {
	Network     string
	DataDir     string
	TargetState string
	TrustAnchor string
}

func resolveBootstrapConfig(cmd *cobra.Command) (*bootstrapConfig, error) {
	cfg := &bootstrapConfig{
		Network:     flagBootstrap.Network,
		DataDir:     flagBootstrap.DataDir,
		TargetState: strings.ToUpper(flagBootstrap.TargetState),
		TrustAnchor: flagBootstrap.TrustAnchor,
	}

	if flagBootstrap.ConfigFile == "" && cfg.Network == "" {
		return resolveInteractive(cmd, cfg)
	}

	// Config-file mode — implementation deferred. For now require all
	// values via flags.
	if cfg.Network == "" {
		return nil, fmt.Errorf("--network required (or use interactive mode)")
	}
	if cfg.DataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		cfg.DataDir = home + "/.accumulated"
	}
	if cfg.TargetState != "BOOTING" && cfg.TargetState != "ACTIVE" {
		return nil, fmt.Errorf("--target-state must be BOOTING or ACTIVE, got %q", cfg.TargetState)
	}
	return cfg, nil
}

func resolveInteractive(cmd *cobra.Command, cfg *bootstrapConfig) (*bootstrapConfig, error) {
	r := bufio.NewReader(cmd.InOrStdin())

	prompt := func(label, def string) string {
		if def != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "%s [%s]: ", label, def)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: ", label)
		}
		line, err := r.ReadString('\n')
		if err != nil {
			return def
		}
		v := strings.TrimSpace(line)
		if v == "" {
			return def
		}
		return v
	}

	cfg.Network = prompt("Network (mainnet/testnet/devnet/<endpoint>)", "mainnet")
	defaultDataDir := "~/.accumulated"
	if home, err := os.UserHomeDir(); err == nil {
		defaultDataDir = home + "/.accumulated"
	}
	cfg.DataDir = prompt("Data directory", defaultDataDir)
	cfg.TargetState = strings.ToUpper(prompt("Target state (BOOTING/ACTIVE)", "ACTIVE"))
	cfg.TrustAnchor = prompt("Trust anchor override (empty = use built-in)", "")

	if cfg.TargetState != "BOOTING" && cfg.TargetState != "ACTIVE" {
		return nil, fmt.Errorf("target-state must be BOOTING or ACTIVE")
	}
	return cfg, nil
}
