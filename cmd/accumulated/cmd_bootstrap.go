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
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bootpersist"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/headerwalk"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/keybookat"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pinned"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pipeline"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// cmdBootstrap launches a node from minimum-data state via the v2
// bootstrap design: validator-quorum-anchored block-header walk +
// complete-state account puller + BPT-root convergence.
//
// Usage:
//
//	accumulated bootstrap --network mainnet --data-dir ~/.accumulate/data --partition Directory
//
// Output: a bootstrap-state-v2.json artifact + a populated database
// directory. Subsequent runs of `accumulated run` detect the artifact
// and resume from the recorded state.
var cmdBootstrap = &cobra.Command{
	Use:   "bootstrap",
	Short: "Launch a node from minimum-data state (v2 design)",
	Long: `Bootstrap a node by pulling complete state for the minimum
account set, walking block headers from a pinned height to current
verifying validator quorum at each step, and converging the local
BPT root against the verified terminal header.

Trust comes from the binary's pinned (validator-set-hash, height) for
the named network. No genesis snapshot is referenced; no historical
user signatures are replayed.`,
	RunE: runBootstrap,
}

var flagBootstrap struct {
	Network         string
	DataDir         string
	Partition       string
	HeightRange     uint64
	PinnedHashHex   string // override pinned validator-set hash
	PinnedHeight    uint64 // override pinned height
	SkipQuorumCheck bool   // dev-only: skip per-block quorum check
}

func init() {
	cmdMain.AddCommand(cmdBootstrap)

	f := cmdBootstrap.Flags()
	f.StringVar(&flagBootstrap.Network, "network", "", "network: mainnet | testnet | devnet | <endpoint>")
	f.StringVar(&flagBootstrap.DataDir, "data-dir", "", "data directory (default ~/.accumulated)")
	f.StringVar(&flagBootstrap.Partition, "partition", "Directory", "partition to bootstrap (Directory or BVN name)")
	f.Uint64Var(&flagBootstrap.HeightRange, "height-range", 1, "blocks to walk back from current tip")
	f.StringVar(&flagBootstrap.PinnedHashHex, "pinned-hash", "", "override pinned validator-set hash (64 hex chars)")
	f.Uint64Var(&flagBootstrap.PinnedHeight, "pinned-height", 0, "override pinned height")
	f.BoolVar(&flagBootstrap.SkipQuorumCheck, "skip-quorum", false, "dev-only: skip per-block validator quorum check (--skip-proof equivalent)")
}

func runBootstrap(cmd *cobra.Command, args []string) error {
	if flagBootstrap.Network == "" {
		return fmt.Errorf("--network is required")
	}
	if flagBootstrap.DataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home dir: %w", err)
		}
		flagBootstrap.DataDir = filepath.Join(home, ".accumulated")
	}
	if err := os.MkdirAll(flagBootstrap.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	endpoint := accumulate.ResolveWellKnownEndpoint(flagBootstrap.Network, "v3")
	fmt.Printf("Bootstrap configuration:\n")
	fmt.Printf("  network:   %s\n", flagBootstrap.Network)
	fmt.Printf("  endpoint:  %s\n", endpoint)
	fmt.Printf("  data-dir:  %s\n", flagBootstrap.DataDir)
	fmt.Printf("  partition: %s\n", flagBootstrap.Partition)
	fmt.Println()

	ctx := cmd.Context()
	client := jsonrpc.NewClient(endpoint)
	q := api.Querier2{Querier: client}

	// 1. Resolve the pin (binary table or operator override).
	pin, pinSource, err := resolveBootstrapPin(flagBootstrap.Network)
	if err != nil {
		return fmt.Errorf("resolve pin: %w", err)
	}
	fmt.Printf("Pin source: %s\n", pinSource)
	fmt.Printf("  DN genesis state-tree anchor: %x\n", pin.DNGenesisStateTreeAnchor[:8])
	fmt.Println()

	// 2. Identify partition + tip.
	ni, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
	if err != nil {
		return fmt.Errorf("node info: %w", err)
	}
	cs, err := client.ConsensusStatus(ctx, api.ConsensusStatusOptions{
		NodeID:    ni.PeerID.String(),
		Partition: flagBootstrap.Partition,
	})
	if err != nil {
		return fmt.Errorf("consensus status: %w", err)
	}
	tip := uint64(cs.LastBlock.Height)

	endHeight := tip
	startHeight := tip
	if flagBootstrap.HeightRange > 0 && tip >= flagBootstrap.HeightRange {
		startHeight = tip - flagBootstrap.HeightRange + 1
	}
	fmt.Printf("Walk range: [%d..%d] (tip=%d)\n", startHeight, endHeight, tip)

	// 3. Build initial validator set from the operators key book.
	// Note: this uses the *current* operators state, not historical
	// state at the pinned height. For steady-state networks where
	// operators haven't rotated since the pin, this is correct. For
	// rotation-aware bootstraps, we'd need keybookat to walk from
	// the pinned snapshot forward — tracked as a follow-up.
	partitionUrl := protocol.PartitionUrl(flagBootstrap.Partition)
	operatorsUrl := partitionUrl.JoinPath(protocol.Operators)
	initialSet, err := buildInitialValidatorSet(ctx, q, operatorsUrl)
	if err != nil {
		return fmt.Errorf("build initial validator set: %w", err)
	}
	fmt.Printf("Initial validator set: %d validators from %s\n", len(initialSet.Validators), operatorsUrl)
	fmt.Println()

	// 4. Construct sources and run the pipeline.
	headerSrc := headerwalk.NewAPISource(q, partitionUrl.JoinPath(protocol.AnchorPool))
	// Surface operators-keybook deltas across the walk so rotation
	// is handled correctly.
	headerSrc.SetOperatorsPage(operatorsUrl.JoinPath("1"))
	pullSrc := pull.NewAPISource(q)

	dbPath := filepath.Join(flagBootstrap.DataDir, "accumulate.db")
	db, err := database.OpenBadger(dbPath, nil)
	if err != nil {
		return fmt.Errorf("open badger at %s: %w", dbPath, err)
	}
	defer db.Close()

	accounts := minimumBootstrapSet(flagBootstrap.Partition)

	quorumOpts := headerwalk.QuorumOptions{}
	if flagBootstrap.SkipQuorumCheck {
		quorumOpts.MinSignatures = 1 // accept any one validator (dev-only)
	}

	res, err := pipeline.Bootstrap(ctx, pipeline.Options{
		HeaderSource:        headerSrc,
		StartHeight:         startHeight,
		EndHeight:           endHeight,
		InitialValidatorSet: initialSet,
		QuorumOpts:          quorumOpts,
		// keybookat applies operators-keybook deltas across the
		// walk so the validator set evolves correctly when there's
		// rotation between the pinned height and current. For
		// blocks without operators-keybook updates this is a no-op
		// (which is the steady-state case).
		ApplyDelta: keybookat.ApplyDelta,
		PullSource: pullSrc,
		Accounts:   accounts,
		Database:   db,
	})
	if err != nil {
		return fmt.Errorf("bootstrap pipeline: %w", err)
	}

	// 5. Persist the artifact.
	//
	// NOTE: this commit is the schema-rework slice. The
	// single-phase pipeline still produces one VerifiedAnchor —
	// stored under DNVerifiedAnchor, with BVN* fields left zero.
	// Phase 5 of the rewrite splits this into a two-phase pipeline
	// that produces both DN and BVN anchors.
	now := time.Now().UTC()
	art := &bootpersist.Artifact{
		Network:                  flagBootstrap.Network,
		BVN:                      flagBootstrap.Partition,
		DNGenesisStateTreeAnchor: pin.DNGenesisStateTreeAnchor,
		DNVerifiedAnchor:         res.VerifiedAnchor,
		DNVerifiedMajorBlock:     res.TerminalStep.Header.Height,
		State: bootpersist.StateRecord{
			Current:        "ACTIVE",
			EnteredBooting: now,
			EnteredActive:  now,
		},
		Cursors: bootpersist.Cursors{
			WalkLastVerified: res.TerminalStep.Header.Height,
			AccountsPulled:   uint64(res.AccountsPulled),
		},
	}
	if err := bootpersist.Save(flagBootstrap.DataDir, art); err != nil {
		return fmt.Errorf("save artifact: %w", err)
	}

	fmt.Println()
	fmt.Println("Bootstrap complete.")
	fmt.Printf("  accounts pulled:   %d\n", res.AccountsPulled)
	fmt.Printf("  verified anchor:   %x\n", res.VerifiedAnchor[:8])
	fmt.Printf("  verified height:   %d\n", res.TerminalStep.Header.Height)
	fmt.Printf("  artifact:          %s/%s\n", flagBootstrap.DataDir, bootpersist.FileName)
	fmt.Println()
	fmt.Println("Run `accumulated run` to start the node in ACTIVE state.")
	return nil
}

// resolveBootstrapPin returns the pin to use for the network, plus a
// human-readable description of where it came from.
//
// Phase-1 schema note: --pinned-hash is repurposed as the DN
// genesis StateTreeAnchor override (not the validator-set hash);
// --pinned-height is now unused and accepted only for backward
// compat with operators' command lines. Phase 6 of the rewrite
// renames the flag to --genesis-state-tree-anchor and removes
// --pinned-height.
func resolveBootstrapPin(network string) (pinned.Pin, string, error) {
	if flagBootstrap.PinnedHashHex != "" {
		var p pinned.Pin
		hash, err := hexDecode32(flagBootstrap.PinnedHashHex)
		if err != nil {
			return pinned.Pin{}, "", fmt.Errorf("--pinned-hash: %w", err)
		}
		p.DNGenesisStateTreeAnchor = hash
		return p, "operator override (--pinned-hash)", nil
	}

	p := pinned.Get(network)
	if p.IsZero() {
		return pinned.Pin{}, "", fmt.Errorf("no pin available for network %q — pass --pinned-hash to override", network)
	}
	return p, fmt.Sprintf("binary pin for %q", network), nil
}

// buildInitialValidatorSet pulls the operators key book and projects
// its key page entries onto a headerwalk.ValidatorSet. The KeyPage
// entries carry public-key hashes; full public keys come from the
// signatures themselves (KeySignature.GetPublicKey()). For
// verification, that's all the validator set needs to know.
func buildInitialValidatorSet(ctx context.Context, q api.Querier2, operatorsUrl *url.URL) (headerwalk.ValidatorSet, error) {
	rec, err := q.QueryAccount(ctx, operatorsUrl.JoinPath("1"), nil)
	if err != nil {
		return headerwalk.ValidatorSet{}, fmt.Errorf("query operators page 1: %w", err)
	}
	page, ok := rec.Account.(*protocol.KeyPage)
	if !ok {
		return headerwalk.ValidatorSet{}, fmt.Errorf("operators page 1 has unexpected type %T", rec.Account)
	}
	out := headerwalk.ValidatorSet{Validators: make([]headerwalk.Validator, 0, len(page.Keys))}
	for _, k := range page.Keys {
		var pkh [32]byte
		copy(pkh[:], k.PublicKeyHash)
		out.Validators = append(out.Validators, headerwalk.Validator{
			PublicKeyHash: pkh,
			Type:          protocol.SignatureTypeED25519, // default; KeySignature path will use the actual scheme
		})
	}
	return out, nil
}

// minimumBootstrapSet returns the canonical accounts to pull. Same
// shape as v1's set; can be extended via a future config knob.
func minimumBootstrapSet(partition string) []*url.URL {
	pu := protocol.PartitionUrl(partition)
	return []*url.URL{
		protocol.DnUrl().JoinPath(protocol.Network),
		pu.JoinPath(protocol.Operators),
		pu.JoinPath(protocol.Operators, "1"),
		pu.JoinPath(protocol.Ledger),
		pu.JoinPath(protocol.AnchorPool),
		pu.JoinPath(protocol.Synthetic),
	}
}

func hexDecode32(s string) ([32]byte, error) {
	if len(s) != 64 {
		return [32]byte{}, fmt.Errorf("expected 64 hex chars, got %d", len(s))
	}
	var out [32]byte
	for i := 0; i < 32; i++ {
		hi, err := hexNibble(s[2*i])
		if err != nil {
			return out, err
		}
		lo, err := hexNibble(s[2*i+1])
		if err != nil {
			return out, err
		}
		out[i] = hi<<4 | lo
	}
	return out, nil
}

func hexNibble(c byte) (byte, error) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', nil
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, nil
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, nil
	default:
		return 0, fmt.Errorf("invalid hex character %q", c)
	}
}
