// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"golang.org/x/exp/slices"
	"golang.org/x/term"
)

func initDevNet(cmd *cobra.Command) *run.Config {
	f, ok := findConfigFile(flagMain.WorkDir)
	if !ok {
		check(os.MkdirAll(flagMain.WorkDir, 0700))

		// Default config
		dev := &run.DevnetConfiguration{
			Listen: multiaddr.StringCast(fmt.Sprintf("/tcp/%d", flagRunDevnet.BasePort)),
		}
		cfg := &run.Config{
			Network:        flagRunDevnet.Name,
			Configurations: []run.Configuration{dev},
		}
		applyDevNetFlags(cmd, cfg, dev, false)
		cfg.SetFilePath(filepath.Join(flagMain.WorkDir, "accumulate.toml"))
		check(cfg.Save())
		return cfg
	}

	cfg := new(run.Config)
	check(cfg.LoadFrom(f))
	i := slices.IndexFunc(cfg.Configurations, func(c run.Configuration) bool { return c.Type() == run.ConfigurationTypeDevnet })
	if i < 0 {
		fatalf("not a devnet: %q", f)
	}
	dev := cfg.Configurations[i].(*run.DevnetConfiguration)

	// Are any of the values different?
	wantReset := false ||
		cmd.Flag("name").Changed && cfg.Network != flagRunDevnet.Name ||
		cmd.Flag("logging").Changed && !flagRunDevnet.Logging.Equal(cfg.Logging) ||
		cmd.Flag("bvns").Changed && dev.Bvns != uint64(flagRunDevnet.NumBvns) ||
		cmd.Flag("validators").Changed && dev.Validators != uint64(flagRunDevnet.NumValidators) ||
		cmd.Flag("followers").Changed && dev.Followers != uint64(flagRunDevnet.NumFollowers) ||
		cmd.Flag("globals").Changed && !flagRunDevnet.Globals.Equal(dev.Globals)
	if wantReset {
		switch {
		case flagMain.Reset:
			// Ok

		case flagRunDevnet.SoftReset:
			flagMain.Reset = true

		case term.IsTerminal(int(os.Stdin.Fd())):
			fmt.Fprint(os.Stderr, "Configuration and flags do not match. Reset? [yN] ")
			s, err := bufio.NewReader(os.Stdin).ReadString('\n')
			check(err)
			s = strings.TrimSpace(s)
			s = strings.ToLower(s)
			switch s {
			case "y", "yes":
				flagMain.Reset = true
			default:
				os.Exit(0)
			}

		default:
			fatalf("the configuration and flags do not match; use --reset if you wish to override (and reset) the existing configuration")
		}
	}

	applyDevNetFlags(cmd, cfg, dev, true)
	check(cfg.Save())
	return cfg
}

func applyDevNetFlags(cmd *cobra.Command, cfg *run.Config, dev *run.DevnetConfiguration, onlyChanged bool) {
	if cfg.P2P == nil {
		cfg.P2P = new(run.P2P)
	}
	if cfg.P2P.Key == nil {
		_, sk, err := ed25519.GenerateKey(rand.Reader)
		check(err)
		cfg.P2P.Key = &run.RawPrivateKey{
			Address: address.FromED25519PrivateKey(sk).String(),
		}
	}

	if cmd.Flag("database").Changed {
		var typ run.StorageType
		if !typ.SetByName(flagRunDevnet.Database) {
			fatalf("--database: %q is not a valid storage type", flagRunDevnet.Database)
		}
		dev.StorageType = &typ
	}

	applyDevNetFlag(cmd, "name", &cfg.Network, flagRunDevnet.Name, onlyChanged)
	applyDevNetFlag(cmd, "logging", &cfg.Logging, &flagRunDevnet.Logging, onlyChanged)
	applyDevNetFlag(cmd, "bvns", &dev.Bvns, uint64(flagRunDevnet.NumBvns), onlyChanged)
	applyDevNetFlag(cmd, "validators", &dev.Validators, uint64(flagRunDevnet.NumValidators), onlyChanged)
	applyDevNetFlag(cmd, "followers", &dev.Followers, uint64(flagRunDevnet.NumFollowers), onlyChanged)
	applyDevNetFlag(cmd, "globals", &dev.Globals, &flagRunDevnet.Globals, onlyChanged)
}

func applyDevNetFlag[V any](cmd *cobra.Command, name string, ptr *V, value V, onlyChanged bool) {
	if !onlyChanged || cmd.Flag(name).Changed {
		*ptr = value
	}
}
