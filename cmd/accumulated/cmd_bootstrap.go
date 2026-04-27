// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
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
	TargetState string // "ACTIVE" (default) or "BOOTING"
	TrustAnchor string // optional override "height:hexhash"
	Force       bool
}

func init() {
	cmdMain.AddCommand(cmdBootstrap)

	f := cmdBootstrap.Flags()
	f.StringVar(&flagBootstrap.ConfigFile, "config", "", "path to bootstrap.toml (omit for interactive)")
	f.StringVar(&flagBootstrap.DataDir, "data-dir", "", "data directory (default ~/.accumulated)")
	f.StringVar(&flagBootstrap.Network, "network", "", "network: mainnet | testnet | devnet | <endpoint>")
	f.StringVar(&flagBootstrap.TargetState, "target-state", "ACTIVE", "target state to reach before exiting: BOOTING or ACTIVE")
	f.StringVar(&flagBootstrap.TrustAnchor, "trust-anchor", "", "override pinned trust anchor: 'height:hexhash'")
	f.BoolVar(&flagBootstrap.Force, "force", false, "force re-bootstrap even if state is already persisted")
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

	// The actual bootstrap pipeline:
	//   1. Pin block H at confirmed depth.
	//   2. Pull current state at H for the minimum bootstrap set.
	//   3. Run the back-walk validator (#3960). Persist proof (#3965).
	//   4. Fill the BPT structure via #3969.
	//   5. Hand off to accumulated run; hydrator (#3964) drives BOOTING → ACTIVE.
	//   6. If target = ACTIVE, wait for state transition before exit.
	//
	// This scaffolding wires the user-facing flags; the pipeline glue
	// itself depends on persistence + back-walker + hydrator landing
	// (next slice).
	return errors.New("bootstrap pipeline not yet wired — see #3953 child issues for component status")
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
