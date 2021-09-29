package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml"
	"github.com/spf13/viper"
	tm "github.com/tendermint/tendermint/config"
)

func Default() *Config {
	c := new(Config)
	c.Config = *tm.DefaultConfig()
	return c
}

func DefaultValidator() *Config {
	c := new(Config)
	c.Config = *tm.DefaultValidatorConfig()
	return c
}

type Config struct {
	tm.Config  `mapstructure:",squash"`
	Accumulate Accumulate `mapstructure:",squash"`
}

type Accumulate struct {
	AccRPC    RPC    `toml:"acc-rpc" mapstructure:"acc-rpc"`
	AccRouter Router `toml:"acc-router" mapstructure:"acc-router"`
}

type RPC struct {
	ListenAddress string `toml:"listen-address" mapstructure:"listen-address"`
}

type Router struct {
	JSONListenAddress string `toml:"json-listen-address" mapstructure:"json-listen-address"`
	RESTListenAddress string `toml:"rest-listen-address" mapstructure:"rest-listen-address"`
}

func Load(dir string) (*Config, error) {
	return LoadFile(dir, filepath.Join(dir, "config", "config.toml"))
}

func LoadFile(dir, file string) (*Config, error) {

	viper.SetConfigFile(file)
	viper.AddConfigPath(dir)
	err := viper.ReadInConfig()
	if err != nil {
		return nil, fmt.Errorf("read: %v", err)
	}

	config := &Config{Config: *tm.DefaultConfig()}
	err = viper.Unmarshal(config)
	if err != nil {
		return nil, fmt.Errorf("unmarshal: %v", err)
	}

	config.SetRoot(dir)
	tm.EnsureRoot(config.RootDir)
	if err := config.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("validate: %v", err)
	}
	return config, nil
}

func Store(config *Config) error {
	file := filepath.Join(config.RootDir, "config", "config.toml")

	// exits on fail
	tm.WriteConfigFile(config.RootDir, &config.Config)

	f, err := os.OpenFile(file, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	return toml.NewEncoder(f).Encode(config.Accumulate)
}
