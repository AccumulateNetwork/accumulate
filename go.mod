module github.com/AccumulateNetwork/accumulate

go 1.16

require (
	github.com/AccumulateNetwork/jsonrpc2/v15 v15.0.0-20210802145948-43d2d974a106
	github.com/DataDog/zstd v1.4.5 // indirect
	github.com/Factom-Asset-Tokens/factom v0.0.0-20200222022020-d06cbcfe6ece
	github.com/boltdb/bolt v1.3.1
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/dgraph-io/badger v1.6.2
	github.com/dgraph-io/badger/v3 v3.2011.1
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13 // indirect
	github.com/dustin/go-humanize v1.0.0
	github.com/fatih/color v1.13.0
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/getsentry/sentry-go v0.11.0
	github.com/go-playground/validator/v10 v10.9.0
	github.com/golang/mock v1.5.0
	github.com/google/go-cmp v0.5.6 // indirect
	github.com/kardianos/service v1.2.0
	github.com/mdp/qrterminal v1.0.1
	github.com/mitchellh/mapstructure v1.4.2
	github.com/pelletier/go-toml v1.9.4
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/common v0.30.0
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/rinchsan/gosimports v0.1.2
	github.com/rs/zerolog v1.24.0
	github.com/spf13/cast v1.4.1 // indirect
	github.com/spf13/cobra v1.2.1
	github.com/spf13/viper v1.8.1
	github.com/stretchr/objx v0.3.0 // indirect
	github.com/stretchr/testify v1.7.0
	github.com/tendermint/tendermint v0.35.0-rc1
	github.com/tyler-smith/go-bip32 v1.0.0
	github.com/tyler-smith/go-bip39 v1.1.0
	github.com/ybbus/jsonrpc/v2 v2.1.6
	golang.org/x/crypto v0.0.0-20210711020723-a769d52b0f97
	golang.org/x/exp v0.0.0-20200331195152-e8c3332aa8e5
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/tools v0.1.7
	gopkg.in/ini.v1 v1.63.2 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
)

replace github.com/tendermint/tendermint v0.35.0-rc1 => github.com/AccumulateNetwork/tendermint v0.35.0-rc1.0.20211104230238-906615e51028
