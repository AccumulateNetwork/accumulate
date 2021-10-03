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

type NetworkType string

const (
	BVC NetworkType = "bvc"
	DC  NetworkType = "dc"
)

type NodeType string

const (
	Validator NodeType = "validator"
	Follower  NodeType = "follower"
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
	tm.Config
	Accumulate Accumulate
}

type Accumulate struct {
	Type     NetworkType `toml:"type" mapstructure:"type"`
	Networks []string    `toml:"networks" mapstructure:"networks"`
	API      API         `toml:"api" mapstructure:"api"`
}

type RPC struct {
	ListenAddress string `toml:"listen-address" mapstructure:"listen-address"`
}

type API struct {
	JSONListenAddress string `toml:"json-listen-address" mapstructure:"json-listen-address"`
	RESTListenAddress string `toml:"rest-listen-address" mapstructure:"rest-listen-address"`
}

func Load(dir string) (*Config, error) {
	return loadFile(dir, filepath.Join(dir, "config", "config.toml"), filepath.Join(dir, "config", "accumulate.toml"))
}

func loadFile(dir, tmFile, accFile string) (*Config, error) {
	tm, err := loadTendermint(dir, tmFile)
	if err != nil {
		return nil, err
	}

	acc, err := loadAccumulate(dir, accFile)
	if err != nil {
		return nil, err
	}

	return &Config{*tm, *acc}, nil
}

func Store(config *Config) error {
	// Exits on fail, hard-coded to write to '${config.RootDir}/config/config.toml'
	tm.WriteConfigFile(config.RootDir, &config.Config)

	f, err := os.Create(filepath.Join(config.RootDir, "config", "accumulate.toml"))
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

func loadTendermint(dir, file string) (*tm.Config, error) {
	config := tm.DefaultConfig()
	err := load(dir, file, config)
	if err != nil {
		return nil, err
	}

	config.SetRoot(dir)
	tm.EnsureRoot(config.RootDir)
	if err := config.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("validate: %v", err)
	}
	return config, nil
}

func loadAccumulate(dir, file string) (*Accumulate, error) {
	config := new(Accumulate)
	err := load(dir, file, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func load(dir, file string, c interface{}) error {
	v := viper.New()
	v.SetConfigFile(file)
	v.AddConfigPath(dir)
	err := v.ReadInConfig()
	if err != nil {
		return fmt.Errorf("read: %v", err)
	}

	err = v.Unmarshal(c)
	if err != nil {
		return fmt.Errorf("unmarshal: %v", err)
	}

	return nil
}
