package accumulate

import (
	"crypto/ed25519"

	"github.com/AccumulateNetwork/accumulated/blockchain/tendermint"
	"github.com/AccumulateNetwork/accumulated/blockchain/validator"
)

func CreateAccumulateBVC(config string, path string) (*tendermint.AccumulatorVMApplication, error) {
	//create a tendermint AccumulatorVM
	acc := tendermint.NewAccumulatorVMApplication(config, path)

	//extract the signing key from the tendermint node
	key := ed25519.PrivateKey{}
	key = acc.Key.PrivKey.Bytes()

	//create the bvc and give it the signing key
	bvc := NewBVCNode(config, path, key)

	//give the tendermint application the validator node to use
	acc.SetAccumulateNode(&bvc.Node)

	//fire up the tendermint processing...
	go acc.Start()

	acc.Wait()

	return acc, nil
}

func CreateAccumulateDBVC(config string, path string) *tendermint.AccumulatorVMApplication {
	//create a tendermint AccumulatorVM
	acc := tendermint.NewAccumulatorVMApplication(config, path)

	//extract the signing key from the tendermint node
	key := ed25519.PrivateKey{}
	key = acc.Key.PrivKey.Bytes()

	//create the bvc and give it the signing key
	dc := NewDCNode(config, path, key)

	//give the tendermint application the validator node to use
	acc.SetAccumulateNode(&dc.Node)

	//fire up the tendermint processing...
	go acc.Start()

	acc.Wait()
	return acc
}

type BVCNode struct {
	validator.Node
}

func NewBVCNode(configFile string, path string, key ed25519.PrivateKey) *BVCNode {
	bvc := &BVCNode{}
	bvc.Initialize(configFile, path, key, &validator.NewBlockValidatorChain().ValidatorContext)
	return bvc
}

// DCNode Directory Chain Node
type DCNode struct {
	validator.Node
}

func NewDCNode(configFile string, path string, key ed25519.PrivateKey) *BVCNode {
	bvc := &BVCNode{}
	//bvc.Initialize(configFile, path, key, &validator.NewDirectoryChain().ValidatorContext)
	return bvc
}

// DSNode Data store node
type DSNode struct {
	validator.Node
}

func NewDSNode(configFile string, path string, key ed25519.PrivateKey) *BVCNode {
	bvc := &BVCNode{}
	//bvc.Initialize(configFile, path, key, &validator.NewDataStore().ValidatorContext)
	return bvc
}
