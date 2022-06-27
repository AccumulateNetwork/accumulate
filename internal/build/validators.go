package build

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// AddOperator constructs an envelope that will add an operator to the network.
// If partition is non-empty, the envelope will also add the operator as a
// validator to the partition.
func AddOperator(values *core.GlobalValues, operatorCount int, newPubKey, newKeyHash []byte, partition string, signers ...*signing.Builder) ([]*protocol.Envelope, error) {
	env1, err1 := AddToOperatorPage(values, operatorCount, newKeyHash, signers...)
	for _, signer := range signers {
		signer.Version++
	}
	env2, err2 := AddValidator(values, operatorCount, newPubKey, partition, signers...)
	if err1 != nil {
		return nil, err1
	} else if err2 != nil {
		return nil, err2
	} else {
		return []*protocol.Envelope{env1, env2}, nil
	}
}

func AddToOperatorPage(values *core.GlobalValues, operatorCount int, newKeyHash []byte, signers ...*signing.Builder) (*protocol.Envelope, error) {
	// Add the key hash to the page and update the threshold
	addKey := new(protocol.AddKeyOperation)
	addKey.Entry.KeyHash = newKeyHash
	setThreshold := new(protocol.SetThresholdKeyPageOperation)
	setThreshold.Threshold = values.Globals.OperatorAcceptThreshold.Threshold(operatorCount + 1)
	updatePage := new(protocol.UpdateKeyPage)
	updatePage.Operation = []protocol.KeyPageOperation{addKey, setThreshold}
	return initiateTransaction(signers, protocol.DnUrl().JoinPath(protocol.Operators, "1"), updatePage)
}

func AddValidator(values *core.GlobalValues, operatorCount int, newPubKey []byte, partition string, signers ...*signing.Builder) (*protocol.Envelope, error) {
	// Add the key to the network definition
	return updateNetworkDefinition(values, signers, partition, func(def *protocol.PartitionDefinition) {
		def.ValidatorKeys = append(def.ValidatorKeys, newPubKey)
	})
}

// RemoveOperator constructs an envelope that will remove an operator from the
// network. If partition is non-empty, the envelope will also remove the operator
// as a validator from the partition.
func RemoveOperator(values *core.GlobalValues, operatorCount int, oldPubKey, oldKeyHash []byte, partition string, signers ...*signing.Builder) ([]*protocol.Envelope, error) {
	env1, err1 := RemoveFromOperatorPage(values, operatorCount, oldKeyHash, signers...)
	for _, signer := range signers {
		signer.Version++
	}
	env2, err2 := RemoveValidator(values, operatorCount, oldPubKey, partition, signers...)
	if err1 != nil {
		return nil, err1
	} else if err2 != nil {
		return nil, err2
	} else {
		return []*protocol.Envelope{env1, env2}, nil
	}
}

func RemoveFromOperatorPage(values *core.GlobalValues, operatorCount int, oldKeyHash []byte, signers ...*signing.Builder) (*protocol.Envelope, error) {
	// Remove the key hash from the page and update the threshold
	removeKey := new(protocol.RemoveKeyOperation)
	removeKey.Entry.KeyHash = oldKeyHash
	setThreshold := new(protocol.SetThresholdKeyPageOperation)
	setThreshold.Threshold = values.Globals.OperatorAcceptThreshold.Threshold(operatorCount - 1)
	updatePage := new(protocol.UpdateKeyPage)
	updatePage.Operation = []protocol.KeyPageOperation{removeKey, setThreshold}
	return initiateTransaction(signers, protocol.DnUrl().JoinPath(protocol.Operators, "1"), updatePage)
}

func RemoveValidator(values *core.GlobalValues, operatorCount int, oldPubKey []byte, partition string, signers ...*signing.Builder) (*protocol.Envelope, error) {
	// Remove the key from the network definition
	return updateNetworkDefinition(values, signers, partition, func(def *protocol.PartitionDefinition) {
		for i, k := range def.ValidatorKeys {
			if bytes.Equal(k, oldPubKey) {
				def.ValidatorKeys = append(def.ValidatorKeys[:i], def.ValidatorKeys[i+1:]...)
				break
			}
		}
	})
}

// UpdateOperatorKey constructs an envelope that will update an operator's key.
// If partition is non-empty, the envelope will also update the operator's key in
// the network definition.
func UpdateOperatorKey(values *core.GlobalValues, oldPubKey, oldKeyHash, newPubKey, newKeyHash []byte, partition string, signers ...*signing.Builder) ([]*protocol.Envelope, error) {
	env1, err1 := UpdateKeyOnOperatorPage(oldKeyHash, newKeyHash, signers...)
	for _, signer := range signers {
		signer.Version++
	}
	env2, err2 := UpdateValidatorKey(values, oldPubKey, newPubKey, partition, signers...)
	if err1 != nil {
		return nil, err1
	} else if err2 != nil {
		return nil, err2
	} else {
		return []*protocol.Envelope{env1, env2}, nil
	}
}

func UpdateKeyOnOperatorPage(oldKeyHash, newKeyHash []byte, signers ...*signing.Builder) (*protocol.Envelope, error) {
	// Update the key hash
	updateKey := new(protocol.UpdateKeyOperation)
	updateKey.OldEntry.KeyHash = oldKeyHash
	updateKey.NewEntry.KeyHash = newKeyHash
	updatePage := new(protocol.UpdateKeyPage)
	updatePage.Operation = []protocol.KeyPageOperation{updateKey}
	return initiateTransaction(signers, protocol.DnUrl().JoinPath(protocol.Operators, "1"), updatePage)
}

func UpdateValidatorKey(values *core.GlobalValues, oldPubKey, newPubKey []byte, partition string, signers ...*signing.Builder) (*protocol.Envelope, error) {
	// Update the key in the network
	return updateNetworkDefinition(values, signers, partition, func(def *protocol.PartitionDefinition) {
		for i, k := range def.ValidatorKeys {
			if bytes.Equal(k, oldPubKey) {
				def.ValidatorKeys[i] = newPubKey
				break
			}
		}
	})
}

func updateNetworkDefinition(values *core.GlobalValues, signers []*signing.Builder, partition string, update func(*protocol.PartitionDefinition)) (*protocol.Envelope, error) {
	def := values.Network.Partition(partition)
	if def == nil {
		return nil, errors.Format(errors.StatusNotFound, "partition %s is not present in the network definition", partition)
	}

	update(def)

	writeData := new(protocol.WriteData)
	writeData.WriteToState = true
	writeData.Entry = values.FormatNetwork()
	env, err := initiateTransaction(signers, protocol.DnUrl().JoinPath(protocol.Network), writeData)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	return env, nil
}

func initiateTransaction(signers []*signing.Builder, principal *url.URL, body protocol.TransactionBody) (*protocol.Envelope, error) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = principal
	txn.Body = body
	env := new(protocol.Envelope)
	env.Transaction = append(env.Transaction, txn)

	for i, signer := range signers {
		var sig protocol.Signature
		var err error
		if i == 0 {
			sig, err = signer.Initiate(txn)
		} else {
			sig, err = signer.Sign(txn.GetHash())
		}
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "sign: %w", err)
		}
		env.Signatures = append(env.Signatures, sig)
	}
	return env, nil
}
