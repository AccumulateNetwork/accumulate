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
	acc.SetAccumulateNode(bvc)

	//fire up the tendermint processing...
	var err error
	go func() {
		_, err = acc.Start()
	}()

	acc.Wait()

	if err != nil {
		return nil, err
	}
	return acc, nil
}

func CreateAccumulateDC(config string, path string) *tendermint.AccumulatorVMApplication {
	//create a tendermint AccumulatorVM
	acc := tendermint.NewAccumulatorVMApplication(config, path)

	//extract the signing key from the tendermint node
	key := ed25519.PrivateKey{}
	key = acc.Key.PrivKey.Bytes()

	//create the bvc and give it the signing key
	dc := NewDCNode(config, path, key)

	//give the tendermint application the validator node to use
	acc.SetAccumulateNode(dc)

	//fire up the tendermint processing...
	go acc.Start()

	acc.Wait()
	return acc
}

type BVCNode struct {
	validator.Node
}

func NewBVCNode(configFile string, path string, key ed25519.PrivateKey) *validator.Node {
	bvc := &validator.Node{}
	bvc.Initialize(configFile, path, key, &validator.NewBlockValidatorChain().ValidatorContext)
	return bvc
}

func NewDCNode(configFile string, path string, key ed25519.PrivateKey) *validator.Node {
	bvc := &validator.Node{}
	//bvc.Initialize(configFile, path, key, &validator.NewDirectoryChain().ValidatorContext)
	return bvc
}

func NewDSNode(configFile string, path string, key ed25519.PrivateKey) *validator.Node {
	bvc := &validator.Node{}
	//bvc.Initialize(configFile, path, key, &validator.NewDataStore().ValidatorContext)
	return bvc
}
