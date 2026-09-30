// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
)

// legacyConfig is a CometBFT-era partition directory (dnn/ or bvnn/): the
// fields of config/tendermint.toml the migration reads, and
// config/accumulate.toml. It is this tool's own copy; the node no longer has
// one.
type legacyConfig struct {
	legacyTendermint `mapstructure:",squash"`
	Accumulate       config.Accumulate
}

// legacyTendermint is the part of CometBFT's config.toml the migration reads,
// with CometBFT's keys and defaults.
type legacyTendermint struct {
	Genesis          string `mapstructure:"genesis_file"`
	PrivValidatorKey string `mapstructure:"priv_validator_key_file"`
	NodeKey          string `mapstructure:"node_key_file"`
	P2P              struct {
		ListenAddress   string `mapstructure:"laddr"`
		PersistentPeers string `mapstructure:"persistent_peers"`
	} `mapstructure:"p2p"`
}

func defaultLegacyTendermint() legacyTendermint {
	var t legacyTendermint
	t.Genesis = "config/genesis.json"
	t.PrivValidatorKey = "config/priv_validator_key.json"
	t.NodeKey = "config/node_key.json"
	t.P2P.ListenAddress = "tcp://0.0.0.0:26656"
	return t
}

// loadLegacy reads dir/config/tendermint.toml (CometBFT's defaults if it is
// missing) and dir/config/accumulate.toml.
func loadLegacy(dir string) (*legacyConfig, error) {
	c := &legacyConfig{legacyTendermint: defaultLegacyTendermint()}

	tm := filepath.Join(dir, "config", "tendermint.toml")
	if _, err := os.Stat(tm); err == nil {
		err = loadToml(tm, &c.legacyTendermint)
		if err != nil {
			return nil, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	err := loadToml(filepath.Join(dir, "config", "accumulate.toml"), &c.Accumulate)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func loadToml(file string, v any) error {
	vp := viper.New()
	vp.SetConfigFile(file)
	err := vp.ReadInConfig()
	if err != nil {
		return fmt.Errorf("read config file %q: %v", file, err)
	}
	err = vp.Unmarshal(v, viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		config.StringToEnumHookFunc(),
		config.StringToMultiaddrHookFunc(),
	)))
	if err != nil {
		return fmt.Errorf("unmarshal %q: %v", file, err)
	}
	return nil
}
