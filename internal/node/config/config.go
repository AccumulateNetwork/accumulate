// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	tm "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/privval"
	"github.com/mitchellh/mapstructure"
	"github.com/multiformats/go-multiaddr"
	"github.com/pelletier/go-toml"
	"github.com/spf13/viper"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum  --package config enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package config types.yml

const PortOffsetDirectory = 0
const PortOffsetBlockValidator = 100
const PortOffsetBlockSummary = 200

const (
	configDir     = "config"
	tmConfigFile  = "tendermint.toml"
	accConfigFile = "accumulate.toml"
)

const DevNet = "devnet"

type NodeType uint64
type PortOffset uint64

var PortOffsetMax = PortOffsetPrometheus

const (
	Validator = NodeTypeValidator
	Follower  = NodeTypeFollower
)

type StorageType string

const (
	MemoryStorage StorageType = "memory"
	BadgerStorage StorageType = "badger"
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

// String converts the log level into a string, for example
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
	SetModule("statesync", "info").
	SetModule("snapshot", "info").
	SetModule("restore", "info").
	// SetModule("accumulate", "info").
	// SetModule("main", "info").
	// SetModule("state", "info").
	// SetModule("statesync", "info").
	// SetModule("accumulate", "debug").
	SetModule("executor", "info").
	SetModule("synthetic", "info").
	// SetModule("storage", "debug").
	// SetModule("database", "debug").
	SetModule("website", "info").
	// SetModule("disk-monitor", "info").
	// SetModule("init", "info").
	String()

func Default(netName string, net protocol.PartitionType, _ NodeType, partitionId string) *Config {
	c := new(Config)
	c.Accumulate.Network.Id = netName
	c.Accumulate.NetworkType = net
	c.Accumulate.PartitionId = partitionId
	c.Accumulate.API.PrometheusServer = "http://18.119.26.7:9090"
	c.Accumulate.API.TxMaxWaitTime = 10 * time.Minute
	c.Accumulate.API.ConnectionLimit = 500
	c.Accumulate.Storage.Type = BadgerStorage
	c.Accumulate.Storage.Path = filepath.Join("data", "accumulate.db")
	c.Accumulate.Snapshots.Enable = false
	c.Accumulate.Snapshots.Directory = "snapshots"
	c.Accumulate.Snapshots.RetainCount = 10
	c.Accumulate.Snapshots.Schedule = protocol.DefaultMajorBlockSchedule
	c.Accumulate.AnalysisLog.Directory = "analysis"
	c.Accumulate.AnalysisLog.Enabled = false
	c.Accumulate.API.ReadHeaderTimeout = 10 * time.Second
	c.Accumulate.BatchReplayLimit = 500
	c.Accumulate.P2P.BootstrapPeers = api.BootstrapServers
	c.Config = *tm.DefaultConfig()
	c.LogLevel = DefaultLogLevels
	c.Instrumentation.Prometheus = true
	c.ProxyApp = ""
	return c
}

type Config struct {
	tm.Config
	Accumulate Accumulate
}

type Accumulate struct {
	Describe         `toml:"describe" mapstructure:"describe"`
	BatchReplayLimit int    `toml:"batch-replay-limit" mapstructure:"batch-replay-limit"`
	SummaryNetwork   string `toml:"summary-network" mapstructure:"summary-network"`

	// TODO: move network config to its own file since it will be constantly changing over time.
	//	NetworkConfig string      `toml:"network" mapstructure:"network"`
	Healing     Healing     `toml:"healing" mapstructure:"healing"`
	Snapshots   Snapshots   `toml:"snapshots" mapstructure:"snapshots"`
	Storage     Storage     `toml:"storage" mapstructure:"storage"`
	P2P         P2P         `toml:"p2p" mapstructure:"p2p"`
	API         API         `toml:"api" mapstructure:"api"`
	AnalysisLog AnalysisLog `toml:"analysis" mapstructure:"analysis"`
}

type Healing struct {
	// Enable enables healing
	Enable bool `toml:"enable" mapstructure:"enable"`
}

type Snapshots struct {
	// Enable enables snapshots
	Enable bool `toml:"enable" mapstructure:"enable"`

	// Directory is the directory to store snapshots in
	Directory string `toml:"directory" mapstructure:"directory"`

	// RetainCount is the number of snapshots to retain
	RetainCount int `toml:"retain" mapstructure:"retain"`

	// Schedule is the schedule for capturing snapshots.
	Schedule string `toml:"schedule" mapstructure:"schedule"`
}

type AnalysisLog struct {
	Directory  string `toml:"directory" mapstructure:"directory"`
	Enabled    bool   `toml:"enabled" mapstructure:"enabled"`
	dataSetLog *logging.DataSetLog
}

func (a *AnalysisLog) Init(workingDir string, partitionId string) {
	if !a.Enabled {
		return
	}
	a.dataSetLog = new(logging.DataSetLog)

	a.dataSetLog.SetProcessName(partitionId)
	if a.Enabled && a.Directory == "" {
		a.Directory = "analysis"
	}
	analysisDir := MakeAbsolute(workingDir, a.Directory)
	a.dataSetLog.SetPath(analysisDir)

	_ = os.MkdirAll(analysisDir, 0700)

	ymd, hm := logging.GetCurrentDateTime()
	a.dataSetLog.SetFileTag(ymd, hm)
}

func (a *AnalysisLog) InitDataSet(dataSetName string, opts logging.Options) {
	if a.dataSetLog != nil {
		a.dataSetLog.Initialize(dataSetName, opts)
	}
}

func (a *AnalysisLog) GetDataSet(dataSetName string) *logging.DataSet {
	if a.dataSetLog != nil {
		return a.dataSetLog.GetDataSet(dataSetName)
	}
	return nil
}

func (a *AnalysisLog) Flush() {
	if a.dataSetLog != nil {
		_, _ = a.dataSetLog.DumpDataSetToDiskFile()
	}
}

type Storage struct {
	Type StorageType `toml:"type" mapstructure:"type"`
	Path string      `toml:"path" mapstructure:"path"`
}

type API struct {
	TxMaxWaitTime      time.Duration `toml:"tx-max-wait-time" mapstructure:"tx-max-wait-time"`
	PrometheusServer   string        `toml:"prometheus-server" mapstructure:"prometheus-server"`
	ListenAddress      string        `toml:"listen-address" mapstructure:"listen-address"`
	DebugJSONRPC       bool          `toml:"debug-jsonrpc" mapstructure:"debug-jsonrpc"`
	EnableDebugMethods bool          `toml:"enable-debug-methods" mapstructure:"enable-debug-methods"`
	ConnectionLimit    int           `toml:"connection-limit" mapstructure:"connection-limit"`
	ReadHeaderTimeout  time.Duration `toml:"read-header-timeout" mapstructure:"read-header-timeout"`
}

func MakeAbsolute(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func OffsetPort(addr string, basePort int, offset int) (*url.URL, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %v", addr, err)
	}

	if u.Scheme == "" {
		ip := net.ParseIP(addr)
		if ip == nil {
			return nil, fmt.Errorf("invalid URL %q: has no scheme and is not an IP address, so this probably isn't a valid URL", addr)
		}

		u, err = url.Parse("tcp://" + ip.String())
		if err != nil {
			return nil, fmt.Errorf("invalid url from ip string")
		}
	}

	u.Host = fmt.Sprintf("%s:%d", u.Hostname(), basePort+offset)
	return u, nil
}

func (n *Network) GetBvnNames() []string {
	var names []string
	for _, partition := range n.Partitions {
		if partition.Type == protocol.PartitionTypeBlockValidator {
			names = append(names, partition.Id)
		}
	}
	return names
}

func (n *Network) GetPartitionByID(partitionID string) *Partition {
	for i, partition := range n.Partitions {
		if strings.EqualFold(partition.Id, partitionID) {
			return &n.Partitions[i]
		}
	}
	return nil
}

func LoadFilePV(keyFilePath, stateFilePath string) (*privval.FilePV, error) {
	// TODO Submit an MR to CometBFT to fix their bull**** (calling os.Exit if
	// the config file load fails)
	return privval.LoadFilePV(keyFilePath, stateFilePath), nil
}

func Load(dir string) (*Config, error) {
	return loadFile(dir, filepath.Join(dir, configDir, tmConfigFile), filepath.Join(dir, configDir, accConfigFile))
}

func LoadAcc(dir string) (*Accumulate, error) {
	return loadAccumulate(dir, filepath.Join(dir, accConfigFile))
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

func Store(config *Config) (err error) {
	// TODO Submit an MR to CometBFT to fix their bull**** (calling os.Exit if
	// the config file write fails)
	defer func() {
		r := recover()
		if e, ok := r.(error); ok {
			err = e
		} else if r != nil {
			err = fmt.Errorf("panicked: %v", r)
		}
	}()

	tm.WriteConfigFile(filepath.Join(config.RootDir, configDir, tmConfigFile), &config.Config)

	return StoreAcc(config, filepath.Join(config.RootDir, configDir))
}

func StoreAcc(config *Config, dir string) error {
	return writeTomlFile(config.Accumulate, filepath.Join(dir, accConfigFile))
}

func writeTomlFile(v any, file string) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(v)
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

	err = v.Unmarshal(c, viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		StringToEnumHookFunc(),
		StringToMultiaddrHookFunc(),
	)))

	if err != nil {
		return fmt.Errorf("unmarshal: %v", err)
	}

	return nil
}

// MarshalTOML marshals the Node Type to Toml as a string.
func (v NodeType) MarshalTOML() ([]byte, error) {
	return []byte("\"" + v.String() + "\""), nil
}

// StringToEnumHookFunc is a decode hook for mapstructure that converts enums to
// strings.
func StringToEnumHookFunc() mapstructure.DecodeHookFuncType {
	return func(
		_ reflect.Type,
		t reflect.Type,
		data interface{},
	) (interface{}, error) {
		switch t {
		case reflect.TypeOf(protocol.PartitionType(0)):
			ret, ok := protocol.PartitionTypeByName(data.(string))
			if !ok {
				return nil, fmt.Errorf("%s is not a partition type", data.(string))
			}
			return ret, nil
		case reflect.TypeOf(NodeType(0)):
			ret, ok := NodeTypeByName(data.(string))
			if !ok {
				return nil, fmt.Errorf("%s is not a node type", data.(string))
			}
			return ret, nil
		}
		return data, nil
	}
}

// StringToMultiaddrHookFunc is a decode hook for mapstructure that converts
// strings to multiaddrs.
func StringToMultiaddrHookFunc() mapstructure.DecodeHookFuncType {
	return func(
		_ reflect.Type,
		t reflect.Type,
		data interface{},
	) (interface{}, error) {
		switch t {
		case reflect.TypeOf(new(multiaddr.Multiaddr)).Elem():
			return multiaddr.NewMultiaddr(data.(string))
		}
		return data, nil
	}
}
