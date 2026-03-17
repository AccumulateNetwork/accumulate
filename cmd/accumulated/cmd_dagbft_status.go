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

// cmdDAGBFTStatus shows the status of a DAG-BFT node.
var cmdDAGBFTStatus = &cobra.Command{
	Use:   "status",
	Short: "Check node status",
	Long:  `Display the current status of a DAG-BFT consensus node.`,
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runDAGBFTStatus(cmd, args)
		printOutput(cmd, out, err)
	},
}

// cmdDAGBFTConsensus shows consensus information.
var cmdDAGBFTConsensus = &cobra.Command{
	Use:   "consensus",
	Short: "Get consensus info",
	Long:  `Display current consensus state and round information.`,
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runDAGBFTConsensus(cmd, args)
		printOutput(cmd, out, err)
	},
}

var flagDAGBFTStatus struct {
	Endpoint string
	JSON     bool
}

func init() {
	cmdDAGBFT.AddCommand(cmdDAGBFTStatus)
	cmdDAGBFT.AddCommand(cmdDAGBFTConsensus)

	// Status flags
	cmdDAGBFTStatus.Flags().StringVarP(&flagDAGBFTStatus.Endpoint, "endpoint", "e", "http://localhost:26660", "Node API endpoint")
	cmdDAGBFTStatus.Flags().BoolVar(&flagDAGBFTStatus.JSON, "json", false, "Output in JSON format")

	// Consensus flags (reuse status flags)
	cmdDAGBFTConsensus.Flags().StringVarP(&flagDAGBFTStatus.Endpoint, "endpoint", "e", "http://localhost:26660", "Node API endpoint")
	cmdDAGBFTConsensus.Flags().BoolVar(&flagDAGBFTStatus.JSON, "json", false, "Output in JSON format")
}

// NodeStatus represents the status of a DAG-BFT node.
type NodeStatus struct {
	NodeID      string    `json:"node_id"`
	Partition   string    `json:"partition"`
	Epoch       uint64    `json:"epoch"`
	Round       uint64    `json:"round"`
	BlockHeight uint64    `json:"block_height"`
	StateHash   string    `json:"state_hash"`
	PeerCount   int       `json:"peer_count"`
	IsValidator bool      `json:"is_validator"`
	Syncing     bool      `json:"syncing"`
	Uptime      string    `json:"uptime"`
	StartTime   time.Time `json:"start_time"`
}

// ConsensusInfo represents consensus state information.
type ConsensusInfo struct {
	Epoch             uint64   `json:"epoch"`
	CurrentRound      uint64   `json:"current_round"`
	CommittedRound    uint64   `json:"committed_round"`
	LeaderRound       uint64   `json:"leader_round"`
	PendingBatches    int      `json:"pending_batches"`
	DAGSize           int      `json:"dag_size"`
	ValidatorCount    int      `json:"validator_count"`
	TotalStake        uint64   `json:"total_stake"`
	MyStake           uint64   `json:"my_stake"`
	AmLeader          bool     `json:"am_leader"`
	RecentCommits     int      `json:"recent_commits"`
	AvgBlockTime      string   `json:"avg_block_time"`
	TransactionsInDAG int      `json:"transactions_in_dag"`
	Validators        []string `json:"validators,omitempty"`
}

func runDAGBFTStatus(_ *cobra.Command, _ []string) (string, error) {
	// In a full implementation, this would query the node's API endpoint
	// For now, return a placeholder status

	status := NodeStatus{
		NodeID:      "12D3KooW...",
		Partition:   "bvn0",
		Epoch:       1,
		Round:       100,
		BlockHeight: 50,
		StateHash:   "0x...",
		PeerCount:   3,
		IsValidator: true,
		Syncing:     false,
		Uptime:      "2h30m",
		StartTime:   time.Now().Add(-2*time.Hour - 30*time.Minute),
	}

	if flagDAGBFTStatus.JSON {
		data, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal status: %w", err)
		}
		return string(data), nil
	}

	return fmt.Sprintf(`DAG-BFT Node Status
==================
Node ID:      %s
Partition:    %s
Epoch:        %d
Round:        %d
Block Height: %d
State Hash:   %s
Peer Count:   %d
Is Validator: %t
Syncing:      %t
Uptime:       %s`,
		status.NodeID,
		status.Partition,
		status.Epoch,
		status.Round,
		status.BlockHeight,
		status.StateHash,
		status.PeerCount,
		status.IsValidator,
		status.Syncing,
		status.Uptime), nil
}

func runDAGBFTConsensus(_ *cobra.Command, _ []string) (string, error) {
	// In a full implementation, this would query the node's API endpoint
	// For now, return placeholder consensus info

	info := ConsensusInfo{
		Epoch:             1,
		CurrentRound:      100,
		CommittedRound:    98,
		LeaderRound:       99,
		PendingBatches:    5,
		DAGSize:           150,
		ValidatorCount:    4,
		TotalStake:        4000,
		MyStake:           1000,
		AmLeader:          false,
		RecentCommits:     50,
		AvgBlockTime:      "3.2s",
		TransactionsInDAG: 2500,
	}

	if flagDAGBFTStatus.JSON {
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal consensus info: %w", err)
		}
		return string(data), nil
	}

	return fmt.Sprintf(`DAG-BFT Consensus Info
======================
Epoch:              %d
Current Round:      %d
Committed Round:    %d
Leader Round:       %d
Pending Batches:    %d
DAG Size:           %d
Validator Count:    %d
Total Stake:        %d
My Stake:           %d
Am Leader:          %t
Recent Commits:     %d
Avg Block Time:     %s
Transactions in DAG: %d`,
		info.Epoch,
		info.CurrentRound,
		info.CommittedRound,
		info.LeaderRound,
		info.PendingBatches,
		info.DAGSize,
		info.ValidatorCount,
		info.TotalStake,
		info.MyStake,
		info.AmLeader,
		info.RecentCommits,
		info.AvgBlockTime,
		info.TransactionsInDAG), nil
}
