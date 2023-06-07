// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"strings"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/services"
	. "gitlab.com/accumulatenetwork/accumulate/cmd/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

var cmdRunServices = &cobra.Command{
	Use:   "services [file or config]",
	Short: "Run multiple services",
	Args:  cobra.ExactArgs(1),
	Run:   runServices,
}

type Config struct {
	LogLevel string
	Network  string
	P2P      *services.P2P
	Services []services.Service
}

func runServices(_ *cobra.Command, args []string) {
	// Load config
	var cfg Config
	if s := strings.TrimSpace(args[0]); strings.HasPrefix(s, "{") {
		Check(json.Unmarshal([]byte(s), &cfg))
	} else {
		b, err := os.ReadFile(args[0])
		Check(err)
		Check(json.Unmarshal(b, &cfg))
	}

	// Validate P2P config
	if cfg.P2P == nil {
		fatalf("missing p2p config")
	}
	Check(cfg.P2P.IsValid())

	// Validate services config
	// TODO Bootstrap mode?
	if len(cfg.Services) == 0 {
		fatalf("no services")
	}
	for _, s := range cfg.Services {
		Check(s.IsValid())

		// Check for unsupported service types
		switch s.Type() {
		case services.ServiceTypeFaucet:
			// Ok
		default:
			fatalf("unsupported service type %v", s.Type())
		}
	}

	// Shutdown on SIGINT with a context
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	go func() {
		<-sigs
		signal.Stop(sigs)
		cancel()
	}()

	// Logging
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	lw, err := logging.NewConsoleWriter("plain")
	Check(err)
	ll, lw, err := logging.ParseLogLevel(cfg.LogLevel, lw)
	Check(err)
	logger, err := logging.NewTendermintLogger(zerolog.New(lw), ll, false)
	Check(err)
}
