package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AccumulateNetwork/accumulated/config"
	"github.com/spf13/cobra"
)

var cmdConfig = &cobra.Command{
	Use: "config",
	Run: printUsageAndExit1,
}

var cmdConfigSentry = &cobra.Command{
	Use:  "set-sentry-dsn [dsn]",
	Run:  setSentryDsn,
	Args: cobra.ExactArgs(1),
}

func init() {
	cmdMain.AddCommand(cmdConfig)
	cmdConfig.AddCommand(cmdConfigSentry)
}

func setSentryDsn(cmd *cobra.Command, args []string) {
	entries, err := os.ReadDir(flagMain.WorkDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: reading %q: %v\n", flagMain.WorkDir, err)
		os.Exit(1)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		cfg, err := config.Load(filepath.Join(flagMain.WorkDir, e.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: loading config for %q: %v\n", e.Name(), err)
			os.Exit(1)
		}

		cfg.Accumulate.SentryDSN = args[0]
		err = config.Store(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: saving config for %q: %v\n", e.Name(), err)
			os.Exit(1)
		}
	}
}
