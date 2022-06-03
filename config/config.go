package config

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml"
	"github.com/spf13/viper"
	tm "github.com/tendermint/tendermint/config"
	etcd "go.etcd.io/etcd/client/v3"
)

////go:generate go run ../tools/cmd/gen-types network.yml

const (
	configDir     = "config"
	tmConfigFile  = "tendermint.toml"
	accConfigFile = "accumulate.toml"
)

const DevNet = "devnet"

type NetworkType string

const (
	BlockValidator NetworkType = "block-validator"
	Directory      NetworkType = "directory"
)

type NodeType string

const (
	Validator NodeType = "validator"
	Follower  NodeType = "follower"
)

type StorageType string

const (
	MemoryStorage StorageType = "memory"
	BadgerStorage StorageType = "badger"
	EtcdStorage   StorageType = "etcd"
)

// LogLevel defines the default and per-module log level for Accumulate's
// logging.
type LogLevel struct {
	Default string
	Modules [][2]string
}

// Parse parses a string such as "error;accumulate=info" into a LogLevel.
func (l LogLevel) Parse(s string) LogLevel {
	for _, s := range strings.Split(s, ";") {
		s := strings.SplitN(s, "=", 2)
		if len(s) == 1 {
			l.Default = s[0]
		} else {
			l.Modules = append(l.Modules, *(*[2]string)(s))
		}
	}
	return l
}

// SetDefault sets the default log level.
func (l LogLevel) SetDefault(level string) LogLevel {
	l.Default = level
	return l
}

// SetModule sets the log level for a module.
func (l LogLevel) SetModule(module, level string) LogLevel {
	l.Modules = append(l.Modules, [2]string{module, level})
	return l
}

// String convers the log level into a string, for example
// "error;accumulate=debug".
func (l LogLevel) String() string {
	s := new(strings.Builder)
	s.WriteString(l.Default)
	for _, m := range l.Modules {
		fmt.Fprintf(s, ";%s=%s", m[0], m[1]) //nolint:rangevarref
	}
	return s.String()
}

var DefaultLogLevels = LogLevel{}.
	SetDefault("error").
	SetModule("snapshot", "info").
	// SetModule("accumulate", "info").
	// SetModule("main", "info").
	// SetModule("state", "info").
	// SetModule("statesync", "info").
	// SetModule("accumulate", "debug").
	SetModule("executor", "info").
	// SetModule("storage", "debug").
	// SetModule("database", "debug").
	// SetModule("disk-monitor", "info").
	// SetModule("init", "info").
	String()

func Default(netName string, net NetworkType, node NodeType, subnetId string) *Config {
	c := new(Config)
	c.Accumulate.Network.Name = netName
	c.Accumulate.NetworkType = net
	c.Accumulate.SubnetId = subnetId
	c.Accumulate.API.PrometheusServer = "http://18.119.26.7:9090"
	c.Accumulate.SentryDSN = "https://glet_78c3bf45d009794a4d9b0c990a1f1ed5@gitlab.com/api/v4/error_tracking/collector/29762666"
	c.Accumulate.Website.Enabled = true
	c.Accumulate.API.TxMaxWaitTime = 10 * time.Minute
	c.Accumulate.API.EnableDebugMethods = true
	c.Accumulate.API.ConnectionLimit = 500
	c.Accumulate.Storage.Type = BadgerStorage
	c.Accumulate.Storage.Path = filepath.Join("data", "accumulate.db")
	c.Accumulate.Snapshots.Directory = "snapshots"
	c.Accumulate.Snapshots.RetainCount = 10
	// c.Accumulate.Snapshots.Frequency = 2
	switch node {
	case Validator:
		c.Config = *tm.DefaultValidatorConfig()
	default:
		c.Config = *tm.DefaultConfig()
	}
	c.LogLevel = DefaultLogLevels
	c.Instrumentation.Prometheus = true
	c.ProxyApp = ""
	return c
}

type Config struct {
	tm.Config
	Accumulate Accumulate
}

type Describe struct {
	SubnetId     string      `toml:"subnet" mapstructure:"subnet"`
	NetworkType  NetworkType `toml:"type" mapstructure:"type"`
	LocalAddress string      `toml:"address" mapstructure:"address"`
	Network      `toml:"network" mapstructure:"network"`
}

type Accumulate struct {
	SentryDSN string `toml:"sentry-dsn" mapstructure:"sentry-dsn"`
	Describe
	Snapshots Snapshots `toml:"snapshots" mapstructure:"snapshots"`
	Storage   Storage   `toml:"storage" mapstructure:"storage"`
	API       API       `toml:"api" mapstructure:"api"`
	Website   Website   `toml:"website" mapstructure:"website"`
}

type Network struct {
	Name    string   `json:"name" toml:"name" mapstructure:"name"`
	Subnets []Subnet `json:"subnets" toml:"subnets" mapstructure:"subnets"`
}

func (v *Network) Equal(u *Network) bool {
	if len(v.Subnets) != len(u.Subnets) || v.Name != u.Name {
		return false
	}
	for i, s := range v.Subnets {
		if s.Equal(&u.Subnets[i]) {
			return false
		}
	}
	return true
}

func (v *Network) Copy() *Network {
	r := new(Network)
	r.Name = v.Name
	for _, s := range v.Subnets {
		r.Subnets = append(r.Subnets, *s.Copy())
	}
	return r
}

type Subnet struct {
	ID       string      `json:"name" toml:"name" mapstructure:"name"`
	Type     NetworkType `json:"type" toml:"type" mapstructure:"type"`
	BasePort int         `json:"port" toml:"port" mapstructure:"port"`
	Nodes    []Node      `json:"nodes" toml:"nodes" mapstructure:"nodes"`
}

func (v *Subnet) Equal(u *Subnet) bool {
	if v.ID != u.ID || v.Type != u.Type || v.BasePort != u.BasePort || len(v.Nodes) != len(u.Nodes) {
		return false
	}
	for i, n := range v.Nodes {
		if n.Equal(&u.Nodes[i]) {
			return false
		}
	}
	return true
}

func (v *Subnet) Copy() *Subnet {
	r := new(Subnet)
	r.Type = v.Type
	r.ID = v.ID
	r.BasePort = v.BasePort
	for _, n := range v.Nodes {
		r.Nodes = append(r.Nodes, *n.Copy())
	}
	return r
}

type Node struct {
	Address string   `json:"ip" toml:"address" mapstructure:"address"`
	Type    NodeType `json:"type" toml:"type" mapstructure:"type"`
}

func (v *Node) Equal(u *Node) bool {
	if v.Address != u.Address || v.Type != u.Type {
		return false
	}
	return true
}

func (v *Node) Copy() *Node {
	r := new(Node)
	r.Address = v.Address
	r.Type = v.Type
	return r
}

//
//type Node struct {
//	IP   string          `json:"ip"`
//	Type NodeType `json:"type"`
//}
//
//type Subnet struct {
//	Name  string             `json:"name"`
//	Type  NetworkType `json:"type"`
//	Port  int                `json:"port"`
//	Nodes []Node             `json:"nodes"`
//}
//
//type Network struct {
//	Network string   `json:"network"`
//	Subnet  []Subnet `json:"subnet"`
//}

type Snapshots struct {
	// Directory is the directory to store snapshots in
	Directory string `toml:"directory" mapstructure:"directory"`

	// RetainCount is the number of snapshots to retain
	RetainCount int `toml:"retain" mapstructure:"retain"`

	// // Frequency is how many major blocks should occur before another snapshot
	// // is taken
	// Frequency int `toml:"frequency" mapstructure:"frequency"`
}

type Storage struct {
	Type StorageType  `toml:"type" mapstructure:"type"`
	Path string       `toml:"path" mapstructure:"path"`
	Etcd *etcd.Config `toml:"etcd" mapstructure:"etcd"`
}

type API struct {
	TxMaxWaitTime      time.Duration `toml:"tx-max-wait-time" mapstructure:"tx-max-wait-time"`
	PrometheusServer   string        `toml:"prometheus-server" mapstructure:"prometheus-server"`
	ListenAddress      string        `toml:"listen-address" mapstructure:"listen-address"`
	DebugJSONRPC       bool          `toml:"debug-jsonrpc" mapstructure:"debug-jsonrpc"`
	EnableDebugMethods bool          `toml:"enable-debug-methods" mapstructure:"enable-debug-methods"`
	ConnectionLimit    int           `toml:"connection-limit" mapstructure:"connection-limit"`
}

type Website struct {
	Enabled       bool   `toml:"website-enabled" mapstructure:"website-enabled"`
	ListenAddress string `toml:"website-listen-address" mapstructure:"website-listen-address"`
}

func MakeAbsolute(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func OffsetPort(addr string, offset int) (*url.URL, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %v", addr, err)
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("invalid URL %q: has no scheme, so this probably isn't a URL", addr)
	}

	port, err := strconv.ParseInt(u.Port(), 10, 17)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q: %v", u.Port(), err)
	}

	port += int64(offset)
	u.Host = fmt.Sprintf("%s:%d", u.Hostname(), port)
	return u, nil
}

func (n *Network) GetBvnNames() []string {
	var names []string
	for _, subnet := range n.Subnets {
		if subnet.Type == BlockValidator {
			names = append(names, subnet.ID)
		}
	}
	return names
}

func (n *Network) GetSubnetByID(subnetID string) Subnet {
	for _, subnet := range n.Subnets {
		if subnet.ID == subnetID {
			return subnet
		}
	}
	panic(fmt.Sprintf("Subnet ID %s does not exist", subnetID))
}

func Load(dir string) (*Config, error) {
	return loadFile(dir, filepath.Join(dir, configDir, tmConfigFile), filepath.Join(dir, configDir, accConfigFile))
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
	err := config.Config.WriteToTemplate(filepath.Join(config.RootDir, configDir, tmConfigFile))
	if err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(config.RootDir, configDir, accConfigFile))
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
