// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var cmdHealAll = &cobra.Command{
	Use:   "all [network]",
	Short: "Run anchor and synthetic healing in alternating passes with a pause between cycles",
	Long:  "Run anchor and synthetic healing in alternating passes with a pause between cycles. Limited to 2 transactions per partition pair per pass for both anchor and synthetic transactions.",
	Args:  cobra.MaximumNArgs(1),
	Run:   healAll,
}

var (
	healPauseDuration time.Duration = 10 * time.Second
)

func init() {
	cmdHeal.AddCommand(cmdHealAll)
	cmdHealAll.Flags().DurationVar(&healPauseDuration, "pause", 1*time.Minute, "Duration to pause between healing cycles")
	cmdHealAll.Flags().BoolVar(&healContinuous, "continuous", true, "Run healing continuously until interrupted")
}

func healAll(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Save the original continuous value and clear it for sub-commands
	originalContinuous := healContinuous
	// Set the global to false to prevent infinite loops in sub-commands
	healContinuous = false
	// Ensure we restore the original value when we exit
	defer func() { healContinuous = originalContinuous }()

	// Create a local variable for our own continuous behavior
	localContinuous := originalContinuous

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		slog.Info("Received interrupt signal, shutting down gracefully...")
		cancel()
	}()

	// Run healing in a loop until interrupted
	cycleCount := 0
	for {
		select {
		case <-ctx.Done():
			slog.Info("Healing process terminated")
			return
		default:
		}

		cycleCount++
		slog.Info("Starting healing cycle", "cycle", cycleCount)

		// Run anchor healing
		slog.Info("============================================================")
		slog.Info("||                                                        ||")
		slog.Info("||             STARTING ANCHOR HEALING PASS               ||")
		slog.Info("||                                                        ||")
		slog.Info("============================================================")
		slog.Info("", "cycle", cycleCount)

		anchorArgs := append([]string{}, args...)
		healAnchor(cmd, anchorArgs)

		if ctx.Err() != nil {
			slog.Info("Healing process terminated during anchor healing")
			return
		}

		slog.Info("============================================================")
		slog.Info("||                                                        ||")
		slog.Info("||             COMPLETED ANCHOR HEALING PASS              ||")
		slog.Info("||                                                        ||")
		slog.Info("============================================================")
		slog.Info("", "cycle", cycleCount)

		// Pause briefly to allow for context cancellation check
		select {
		case <-ctx.Done():
			slog.Info("Healing process terminated after anchor healing")
			return
		case <-time.After(100 * time.Millisecond):
		}

		// Run synthetic transaction healing
		slog.Info("============================================================")
		slog.Info("||                                                        ||")
		slog.Info("||          STARTING SYNTHETIC HEALING PASS               ||")
		slog.Info("||                                                        ||")
		slog.Info("============================================================")
		slog.Info("", "cycle", cycleCount)

		synthArgs := append([]string{}, args...)
		healSynth(cmd, synthArgs)

		if ctx.Err() != nil {
			slog.Info("Healing process terminated during synthetic transaction healing")
			return
		}

		slog.Info("============================================================")
		slog.Info("||                                                        ||")
		slog.Info("||          COMPLETED SYNTHETIC HEALING PASS              ||")
		slog.Info("||                                                        ||")
		slog.Info("============================================================")
		slog.Info("", "cycle", cycleCount)

		// If not running continuously, break after one cycle
		if !localContinuous {
			slog.Info("Healing completed (non-continuous mode)")
			break
		}

		// Pause between cycles
		slog.Info("============================================================")
		slog.Info("||                                                        ||")
		slog.Info(fmt.Sprintf("||     PAUSING FOR %-35s     ||", healPauseDuration))
		slog.Info("||                                                        ||")
		slog.Info("============================================================")
		slog.Info("", "cycle", cycleCount)

		select {
		case <-ctx.Done():
			slog.Info("Healing process terminated during pause")
			return
		case <-time.After(healPauseDuration):
		}
	}
}
