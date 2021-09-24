package accumulate

import (
	"crypto/ed25519"

	"github.com/AccumulateNetwork/accumulated/blockchain/tendermint"
	"github.com/AccumulateNetwork/accumulated/blockchain/validator"
	"github.com/AccumulateNetwork/accumulated/config"
)

func CreateAccumulateBVC(config *config.Config) (*tendermint.AccumulatorVMApplication, error) {
	//create a tendermint AccumulatorVM
	acc := tendermint.NewAccumulatorVMApplication(config)

	//extract the signing key from the tendermint node
	key := ed25519.PrivateKey{}
	key = acc.Key.PrivKey.Bytes()

	//create the bvc and give it the signing key
	bvc, err := NewBVCNode(config, key)
	if err != nil {
		return nil, err
	}

	//give the tendermint application the validator node to use
	acc.SetAccumulateNode(bvc)

	//fire up the tendermint processing...
	err = acc.Start()
	if err != nil {
		return nil, err
	}

	return acc, nil
}

func CreateAccumulateDC(config *config.Config) (*tendermint.AccumulatorVMApplication, error) {
	//create a tendermint AccumulatorVM
	acc := tendermint.NewAccumulatorVMApplication(config)

	//extract the signing key from the tendermint node
	key := ed25519.PrivateKey{}
	key = acc.Key.PrivKey.Bytes()

	//create the bvc and give it the signing key
	dc, err := NewDCNode(config, key)
	if err != nil {
		return nil, err
	}

	//give the tendermint application the validator node to use
	acc.SetAccumulateNode(dc)

	//fire up the tendermint processing...
	err = acc.Start()
	if err != nil {
		return nil, err
	}

	return acc, nil
}

type BVCNode struct {
	validator.Node
}

func NewBVCNode(config *config.Config, key ed25519.PrivateKey) (*validator.Node, error) {
	bvc := &validator.Node{}
	err := bvc.Initialize(config, key, &validator.NewBlockValidatorChain().ValidatorContext)
	if err != nil {
		return nil, err
	}
	return bvc, nil
}

func NewDCNode(config *config.Config, key ed25519.PrivateKey) (*validator.Node, error) {
	bvc := &validator.Node{}
	//bvc.Initialize(config, key, &validator.NewDirectoryChain().ValidatorContext)
	return bvc, nil
}

func NewDSNode(config *config.Config, key ed25519.PrivateKey) (*validator.Node, error) {
	bvc := &validator.Node{}
	//bvc.Initialize(config, key, &validator.NewDataStore().ValidatorContext)
	return bvc, nil
}
