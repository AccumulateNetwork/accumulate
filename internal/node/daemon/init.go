// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"io"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func BuildGenesisDocs(network *NetworkInit, globals *core.GlobalValues, time time.Time, logger logging.Logger, factomAddresses func() (io.Reader, error), snapshots []func(*core.GlobalValues) (ioutil2.SectionReader, error)) (map[string][]byte, error) {
	if globals == nil {
		globals = new(core.GlobalValues)
	}
	docs := map[string][]byte{}
	var operators [][]byte
	netinfo := new(protocol.NetworkDefinition)
	netinfo.NetworkName = network.Id
	netinfo.AddPartition(protocol.Directory, protocol.PartitionTypeDirectory)

	var i int
	for _, bvn := range network.Bvns {
		netinfo.AddPartition(bvn.Id, protocol.PartitionTypeBlockValidator)

		for _, node := range bvn.Nodes {
			i++
			privKey := ed25519.PrivateKey(node.PrivValKey)
			pubKey := privKey.Public().(ed25519.PublicKey)
			operators = append(operators, pubKey)
			netinfo.AddValidator(pubKey, protocol.Directory, node.DnnType == config.Validator)
			netinfo.AddValidator(pubKey, bvn.Id, node.BvnnType == config.Validator)
		}
	}

	if network.Bsn != nil {
		for _, node := range network.Bsn.Nodes {
			privKey := ed25519.PrivateKey(node.PrivValKey)
			pubKey := privKey.Public().(ed25519.PublicKey)
			operators = append(operators, pubKey)
			netinfo.AddValidator(pubKey, network.Bsn.Id, node.BsnnType == config.Validator)
		}
	}

	globals.Network = netinfo

	if len(network.OperatorKeys) > 0 {
		operators = network.OperatorKeys
	}

	var ids []string
	ids = append(ids, protocol.Directory)
	for _, bvn := range network.Bvns {
		ids = append(ids, bvn.Id)
	}

	bsnSnapBuf := new(ioutil2.Buffer)
	var bsnSnap *snapshot.Writer
	var err error
	if network.Bsn != nil {
		header := new(snapshot.Header)
		header.Height = 1
		header.Timestamp = time
		header.PartitionSnapshotIDs = ids
		bsnSnap, err = snapshot.Create(bsnSnapBuf, header)
		if err != nil {
			return nil, err
		}
	}

	for _, id := range ids {
		netType := protocol.PartitionTypeBlockValidator
		if strings.EqualFold(id, protocol.Directory) {
			netType = protocol.PartitionTypeDirectory
		}
		snapBuf := new(ioutil2.Buffer)
		err = genesis.Init(snapBuf, genesis.InitOpts{
			NetworkID:       network.Id,
			PartitionId:     id,
			NetworkType:     netType,
			GenesisTime:     time,
			Logger:          logger.With("partition", id),
			GenesisGlobals:  globals,
			OperatorKeys:    operators,
			FactomAddresses: factomAddresses,
			Snapshots:       snapshots,
			ConsensusParams: cometbft.DefaultConsensusParams(),
		})
		if err != nil {
			return nil, fmt.Errorf("init %s: %w", id, err)
		}

		// Write the snapshot to the BSN snapshot
		if network.Bsn != nil {
			w, err := bsnSnap.Open(snapshot.SectionTypeSnapshot)
			if err != nil {
				return nil, err
			}
			_, err = bytes.NewBuffer(snapBuf.Bytes()).WriteTo(w)
			if err != nil {
				return nil, err
			}
			err = w.Close()
			if err != nil {
				return nil, err
			}
		}

		docs[id] = snapBuf.Bytes()
	}

	if network.Bsn != nil {
		docs[network.Bsn.Id] = bsnSnapBuf.Bytes()
	}

	return docs, nil
}
