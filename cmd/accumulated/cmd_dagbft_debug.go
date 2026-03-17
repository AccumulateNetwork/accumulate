// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// cmdDAGBFTDAG views DAG state at a specific round.
var cmdDAGBFTDAG = &cobra.Command{
	Use:   "dag",
	Short: "View DAG state",
	Long:  `Display the DAG state at a specific round or the current round.`,
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runDAGBFTDAG(cmd, args)
		printOutput(cmd, out, err)
	},
}

// cmdDAGBFTCert views certificate details.
var cmdDAGBFTCert = &cobra.Command{
	Use:   "cert <digest>",
	Short: "View certificate",
	Long:  `Display details of a specific certificate by its digest.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runDAGBFTCert(cmd, args)
		printOutput(cmd, out, err)
	},
}

// cmdDAGBFTPending views pending certificates.
var cmdDAGBFTPending = &cobra.Command{
	Use:   "pending",
	Short: "View pending certificates",
	Long:  `Display certificates that are pending commitment.`,
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runDAGBFTPending(cmd, args)
		printOutput(cmd, out, err)
	},
}

var flagDAGBFTDebug struct {
	Endpoint string
	Round    uint64
	JSON     bool
	Verbose  bool
}

func init() {
	cmdDAGBFT.AddCommand(cmdDAGBFTDAG)
	cmdDAGBFT.AddCommand(cmdDAGBFTCert)
	cmdDAGBFT.AddCommand(cmdDAGBFTPending)

	// DAG flags
	cmdDAGBFTDAG.Flags().StringVarP(&flagDAGBFTDebug.Endpoint, "endpoint", "e", "http://localhost:26660", "Node API endpoint")
	cmdDAGBFTDAG.Flags().Uint64Var(&flagDAGBFTDebug.Round, "round", 0, "Round to inspect (0 for current)")
	cmdDAGBFTDAG.Flags().BoolVar(&flagDAGBFTDebug.JSON, "json", false, "Output in JSON format")
	cmdDAGBFTDAG.Flags().BoolVarP(&flagDAGBFTDebug.Verbose, "verbose", "v", false, "Show detailed information")

	// Cert flags
	cmdDAGBFTCert.Flags().StringVarP(&flagDAGBFTDebug.Endpoint, "endpoint", "e", "http://localhost:26660", "Node API endpoint")
	cmdDAGBFTCert.Flags().BoolVar(&flagDAGBFTDebug.JSON, "json", false, "Output in JSON format")
	cmdDAGBFTCert.Flags().BoolVarP(&flagDAGBFTDebug.Verbose, "verbose", "v", false, "Show detailed information")

	// Pending flags
	cmdDAGBFTPending.Flags().StringVarP(&flagDAGBFTDebug.Endpoint, "endpoint", "e", "http://localhost:26660", "Node API endpoint")
	cmdDAGBFTPending.Flags().BoolVar(&flagDAGBFTDebug.JSON, "json", false, "Output in JSON format")
	cmdDAGBFTPending.Flags().BoolVarP(&flagDAGBFTDebug.Verbose, "verbose", "v", false, "Show detailed information")
}

// DAGRoundInfo represents information about a DAG round.
type DAGRoundInfo struct {
	Round        uint64        `json:"round"`
	Epoch        uint64        `json:"epoch"`
	Certificates []CertSummary `json:"certificates"`
	IsLeader     bool          `json:"is_leader"`
	Leader       string        `json:"leader"`
	Committed    bool          `json:"committed"`
}

// CertSummary represents a summary of a certificate.
type CertSummary struct {
	Digest       string   `json:"digest"`
	Author       string   `json:"author"`
	Round        uint64   `json:"round"`
	ParentCount  int      `json:"parent_count"`
	BatchCount   int      `json:"batch_count"`
	SignerCount  int      `json:"signer_count"`
	TotalStake   uint64   `json:"total_stake"`
	CreatedAt    string   `json:"created_at"`
	IsLeaderCert bool     `json:"is_leader_cert"`
	Parents      []string `json:"parents,omitempty"`
}

// CertDetails represents detailed certificate information.
type CertDetails struct {
	Digest          string   `json:"digest"`
	Author          string   `json:"author"`
	Round           uint64   `json:"round"`
	Epoch           uint64   `json:"epoch"`
	Parents         []string `json:"parents"`
	Batches         []string `json:"batches"`
	Signers         []string `json:"signers"`
	SignerCount     int      `json:"signer_count"`
	TotalStake      uint64   `json:"total_stake"`
	StakePercentage float64  `json:"stake_percentage"`
	IsLeaderCert    bool     `json:"is_leader_cert"`
	Committed       bool     `json:"committed"`
	CommittedAt     string   `json:"committed_at,omitempty"`
	CreatedAt       string   `json:"created_at"`
}

// PendingCertificates represents pending certificates info.
type PendingCertificates struct {
	Count        int           `json:"count"`
	OldestRound  uint64        `json:"oldest_round"`
	NewestRound  uint64        `json:"newest_round"`
	Certificates []CertSummary `json:"certificates"`
}

func runDAGBFTDAG(_ *cobra.Command, _ []string) (string, error) {
	// In a full implementation, this would query the node's API endpoint
	// For now, return placeholder DAG info

	round := flagDAGBFTDebug.Round
	if round == 0 {
		round = 100 // Current round placeholder
	}

	info := DAGRoundInfo{
		Round: round,
		Epoch: 1,
		Certificates: []CertSummary{
			{
				Digest:       "0xabc123...",
				Author:       "12D3KooWValidator1...",
				Round:        round,
				ParentCount:  4,
				BatchCount:   3,
				SignerCount:  3,
				TotalStake:   3000,
				CreatedAt:    time.Now().Add(-5 * time.Second).Format(time.RFC3339),
				IsLeaderCert: true,
			},
			{
				Digest:       "0xdef456...",
				Author:       "12D3KooWValidator2...",
				Round:        round,
				ParentCount:  4,
				BatchCount:   2,
				SignerCount:  3,
				TotalStake:   3000,
				CreatedAt:    time.Now().Add(-4 * time.Second).Format(time.RFC3339),
				IsLeaderCert: false,
			},
		},
		IsLeader:  false,
		Leader:    "12D3KooWValidator1...",
		Committed: true,
	}

	if flagDAGBFTDebug.JSON {
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal DAG info: %w", err)
		}
		return string(data), nil
	}

	output := fmt.Sprintf(`DAG Round %d (Epoch %d)
=======================
Leader:    %s
Committed: %t

Certificates (%d):
`, info.Round, info.Epoch, info.Leader, info.Committed, len(info.Certificates))

	for i, cert := range info.Certificates {
		leaderMark := ""
		if cert.IsLeaderCert {
			leaderMark = " [LEADER]"
		}
		output += fmt.Sprintf(`
  [%d] %s%s
      Author:     %s
      Parents:    %d
      Batches:    %d
      Signers:    %d (stake: %d)
`, i+1, cert.Digest, leaderMark, cert.Author, cert.ParentCount, cert.BatchCount, cert.SignerCount, cert.TotalStake)
	}

	return output, nil
}

func runDAGBFTCert(_ *cobra.Command, args []string) (string, error) {
	digest := args[0]

	// In a full implementation, this would query the node's API endpoint
	// For now, return placeholder certificate details

	details := CertDetails{
		Digest:          digest,
		Author:          "12D3KooWValidator1...",
		Round:           100,
		Epoch:           1,
		Parents:         []string{"0xparent1...", "0xparent2...", "0xparent3...", "0xparent4..."},
		Batches:         []string{"0xbatch1...", "0xbatch2...", "0xbatch3..."},
		Signers:         []string{"Validator1", "Validator2", "Validator3"},
		SignerCount:     3,
		TotalStake:      3000,
		StakePercentage: 75.0,
		IsLeaderCert:    true,
		Committed:       true,
		CommittedAt:     time.Now().Add(-2 * time.Second).Format(time.RFC3339),
		CreatedAt:       time.Now().Add(-5 * time.Second).Format(time.RFC3339),
	}

	if flagDAGBFTDebug.JSON {
		data, err := json.MarshalIndent(details, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal certificate: %w", err)
		}
		return string(data), nil
	}

	output := fmt.Sprintf(`Certificate Details
===================
Digest:           %s
Author:           %s
Round:            %d
Epoch:            %d
Is Leader Cert:   %t
Committed:        %t
Committed At:     %s
Created At:       %s

Parents (%d):
`, details.Digest, details.Author, details.Round, details.Epoch,
		details.IsLeaderCert, details.Committed, details.CommittedAt, details.CreatedAt,
		len(details.Parents))

	for _, p := range details.Parents {
		output += fmt.Sprintf("  - %s\n", p)
	}

	output += fmt.Sprintf("\nBatches (%d):\n", len(details.Batches))
	for _, b := range details.Batches {
		output += fmt.Sprintf("  - %s\n", b)
	}

	output += fmt.Sprintf("\nSigners (%d, stake: %d, %.1f%%):\n", details.SignerCount, details.TotalStake, details.StakePercentage)
	for _, s := range details.Signers {
		output += fmt.Sprintf("  - %s\n", s)
	}

	return output, nil
}

func runDAGBFTPending(_ *cobra.Command, _ []string) (string, error) {
	// In a full implementation, this would query the node's API endpoint
	// For now, return placeholder pending certificates

	pending := PendingCertificates{
		Count:       2,
		OldestRound: 99,
		NewestRound: 100,
		Certificates: []CertSummary{
			{
				Digest:      "0xpending1...",
				Author:      "12D3KooWValidator3...",
				Round:       100,
				ParentCount: 4,
				BatchCount:  1,
				SignerCount: 2,
				TotalStake:  2000,
				CreatedAt:   time.Now().Add(-1 * time.Second).Format(time.RFC3339),
			},
			{
				Digest:      "0xpending2...",
				Author:      "12D3KooWValidator4...",
				Round:       100,
				ParentCount: 4,
				BatchCount:  2,
				SignerCount: 2,
				TotalStake:  2000,
				CreatedAt:   time.Now().Add(-500 * time.Millisecond).Format(time.RFC3339),
			},
		},
	}

	if flagDAGBFTDebug.JSON {
		data, err := json.MarshalIndent(pending, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal pending certificates: %w", err)
		}
		return string(data), nil
	}

	if pending.Count == 0 {
		return "No pending certificates", nil
	}

	output := fmt.Sprintf(`Pending Certificates
====================
Count:        %d
Round Range:  %d - %d

Certificates:
`, pending.Count, pending.OldestRound, pending.NewestRound)

	for i, cert := range pending.Certificates {
		output += fmt.Sprintf(`
  [%d] %s
      Author:     %s
      Round:      %d
      Parents:    %d
      Batches:    %d
      Signers:    %d (stake: %d)
      Created:    %s
`, i+1, cert.Digest, cert.Author, cert.Round, cert.ParentCount, cert.BatchCount, cert.SignerCount, cert.TotalStake, cert.CreatedAt)
	}

	return output, nil
}
