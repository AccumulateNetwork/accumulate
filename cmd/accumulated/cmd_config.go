package main

import (
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
	checkf(err, "reading %q", flagMain.WorkDir)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		cfg, err := config.Load(filepath.Join(flagMain.WorkDir, e.Name()))
		checkf(err, "loading config for %q", e.Name())

		cfg.Accumulate.SentryDSN = args[0]
		err = config.Store(cfg)
		checkf(err, "saving config for %q", e.Name())
	}
}
