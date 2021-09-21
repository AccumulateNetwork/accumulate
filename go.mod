module github.com/AccumulateNetwork/accumulated

go 1.15

require (
	github.com/AccumulateNetwork/SMT v0.0.24
	github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc v0.0.0-20201027214006-47dd1dba5f03
	github.com/AccumulateNetwork/jsonrpc2/v15 v15.0.0-20210802145948-43d2d974a106
	github.com/ChainSafe/go-schnorrkel v0.0.0-20200626160457-b38283118816 // indirect
	github.com/DataDog/zstd v1.4.5 // indirect
	github.com/Factom-Asset-Tokens/factom v0.0.0-20200222022020-d06cbcfe6ece
	github.com/FactomProject/factomd v6.7.0+incompatible
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13 // indirect
	github.com/go-playground/validator/v10 v10.9.0
	github.com/golang/protobuf v1.5.2
	github.com/gorilla/mux v1.7.4-0.20190720201435-e67b3c02c719
	github.com/mitchellh/mapstructure v1.4.1
	github.com/spf13/cobra v1.2.1
	github.com/spf13/viper v1.8.1
	github.com/tendermint/tendermint v0.34.12
	github.com/tendermint/tm-db v0.6.4
	github.com/ybbus/jsonrpc/v2 v2.1.6
	golang.org/x/crypto v0.0.0-20210711020723-a769d52b0f97
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.26.0
)

replace github.com/AccumulateNetwork/SMT => ../SMT
