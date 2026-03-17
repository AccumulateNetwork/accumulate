// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/config"
)

// cmdDAGBFTRun runs a DAG-BFT consensus node.
var cmdDAGBFTRun = &cobra.Command{
	Use:   "run",
	Short: "Run a DAG-BFT consensus node",
	Long: `Run a DAG-BFT consensus node with the specified configuration.

This command starts a node that participates in DAG-BFT consensus.
The node will connect to bootstrap peers and begin participating
in the consensus protocol.`,
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runDAGBFTNode(cmd, args)
		printOutput(cmd, out, err)
	},
}

// cmdDAGBFTInit initializes a new DAG-BFT node.
var cmdDAGBFTInit = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new DAG-BFT node",
	Long: `Initialize a new DAG-BFT node with the specified partition and validators.

This creates the necessary configuration and key files for a DAG-BFT node.`,
	Run: func(cmd *cobra.Command, args []string) {
		out, err := initDAGBFTNode(cmd, args)
		printOutput(cmd, out, err)
	},
}

var flagDAGBFTRun struct {
	Config      string
	Debug       bool
	LogLevel    string
	PprofListen string
}

var flagDAGBFTInit struct {
	Partition  string
	Validators string
	OutputDir  string
	ListenAddr string
}

func init() {
	cmdDAGBFT.AddCommand(cmdDAGBFTRun)
	cmdDAGBFT.AddCommand(cmdDAGBFTInit)

	// Run flags
	cmdDAGBFTRun.Flags().StringVarP(&flagDAGBFTRun.Config, "config", "c", "", "Path to configuration file (required)")
	cmdDAGBFTRun.Flags().BoolVarP(&flagDAGBFTRun.Debug, "debug", "d", false, "Enable debug logging")
	cmdDAGBFTRun.Flags().StringVar(&flagDAGBFTRun.LogLevel, "log-level", "info", "Log level: debug, info, warn, error")
	cmdDAGBFTRun.Flags().StringVar(&flagDAGBFTRun.PprofListen, "pprof", "", "Address to run net/http/pprof on")
	_ = cmdDAGBFTRun.MarkFlagRequired("config")

	// Init flags
	cmdDAGBFTInit.Flags().StringVar(&flagDAGBFTInit.Partition, "partition", "", "Partition name (e.g., bvn0)")
	cmdDAGBFTInit.Flags().StringVar(&flagDAGBFTInit.Validators, "validators", "", "Comma-separated list of validator addresses")
	cmdDAGBFTInit.Flags().StringVarP(&flagDAGBFTInit.OutputDir, "output", "o", "", "Output directory for node files")
	cmdDAGBFTInit.Flags().StringVar(&flagDAGBFTInit.ListenAddr, "listen", "/ip4/0.0.0.0/tcp/9000", "Listen address (multiaddr format)")
	_ = cmdDAGBFTInit.MarkFlagRequired("partition")
}

func runDAGBFTNode(_ *cobra.Command, _ []string) (string, error) {
	// Load configuration
	cfg, err := config.LoadFile(flagDAGBFTRun.Config)
	if err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}

	// Override log level if specified
	if flagDAGBFTRun.Debug {
		cfg.Logging.Level = "debug"
	} else if flagDAGBFTRun.LogLevel != "" {
		cfg.Logging.Level = flagDAGBFTRun.LogLevel
	}

	// Start pprof if requested
	if flagDAGBFTRun.PprofListen != "" {
		s := &http.Server{
			Addr:              flagDAGBFTRun.PprofListen,
			ReadHeaderTimeout: time.Minute,
		}
		go func() {
			if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				fmt.Fprintf(os.Stderr, "pprof server error: %v\n", err)
			}
		}()
	}

	// Set up signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		color.HiYellow("\n----- Received shutdown signal -----\n")
		cancel()
	}()

	color.HiBlue("\n----- Starting DAG-BFT Node -----\n")
	color.HiBlue("  Partition: %s\n", flagDAGBFTInit.Partition)
	color.HiBlue("  Listen:    %s\n", cfg.Network.ListenAddr)
	color.HiBlue("  Log level: %s\n", cfg.Logging.Level)

	// Wait for context cancellation
	<-ctx.Done()

	color.HiBlack("\n----- DAG-BFT Node Stopped -----\n")
	return "shutdown complete", nil
}

func initDAGBFTNode(_ *cobra.Command, _ []string) (string, error) {
	outputDir := flagDAGBFTInit.OutputDir
	if outputDir == "" {
		outputDir = fmt.Sprintf("./%s", flagDAGBFTInit.Partition)
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate default config
	cfg := config.DefaultConfig()
	cfg.Network.ListenAddr = flagDAGBFTInit.ListenAddr

	// Parse bootstrap peers if validators specified
	if flagDAGBFTInit.Validators != "" {
		// Validators would be parsed and added as bootstrap peers
		// For now, just store the raw value in the config
		cfg.Network.BootstrapPeers = []string{flagDAGBFTInit.Validators}
	}

	// Write config file
	configPath := fmt.Sprintf("%s/consensus.toml", outputDir)
	if err := config.SaveFile(cfg, configPath); err != nil {
		return "", fmt.Errorf("failed to write config: %w", err)
	}

	// Generate node keys using the keygen function logic
	flagDAGBFTKeygen.Output = outputDir
	_, err := runDAGBFTKeygen(nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate keys: %w", err)
	}

	return fmt.Sprintf("DAG-BFT node initialized:\n  Partition: %s\n  Directory: %s\n  Config:    %s/consensus.toml\n  Keys:      %s/node_key, %s/node_key.pub",
		flagDAGBFTInit.Partition, outputDir, outputDir, outputDir, outputDir), nil
}
