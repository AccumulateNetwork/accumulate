// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package shared

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// This package is a hack because query code got moved to the API

func GetAccountAuthoritySet(account protocol.Account) (*protocol.AccountAuth, *url.URL, error) {
	switch account := account.(type) {
	case *protocol.LiteIdentity:
		return &protocol.AccountAuth{
			Authorities: []protocol.AuthorityEntry{
				{Url: account.Url},
			},
		}, nil, nil

	case *protocol.LiteTokenAccount:
		return &protocol.AccountAuth{
			Authorities: []protocol.AuthorityEntry{
				{Url: account.Url.RootIdentity()},
			},
		}, nil, nil

	case protocol.FullAccount:
		return account.GetAuth(), nil, nil

	case *protocol.KeyPage:
		bookUrl, _, ok := protocol.ParseKeyPageUrl(account.Url)
		if !ok {
			return nil, nil, errors.InternalError.WithFormat("invalid key page URL: %v", account.Url)
		}
		return nil, bookUrl, nil

	default:
		return &protocol.AccountAuth{}, nil, nil
	}
}

func SignTransaction(network *protocol.NetworkDefinition, nodeKey []byte, batch *database.Batch, txn *protocol.Transaction, destination *url.URL) (protocol.Signature, error) {
	// TODO Exporting this is not great

	if nodeKey == nil {
		return nil, errors.InternalError.WithFormat("attempted to sign with a nil key")
	}

	// Sign it
	bld := new(signing.Builder).
		SetType(protocol.SignatureTypeED25519).
		SetPrivateKey(nodeKey).
		SetUrl(protocol.DnUrl().JoinPath(protocol.Network)).
		SetVersion(network.Version).
		SetTimestamp(1)

	keySig, err := bld.Sign(txn.GetHash())
	if err != nil {
		return nil, errors.InternalError.WithFormat("sign synthetic transaction: %w", err)
	}

	return keySig, nil
}

func PrepareBlockAnchor(network *config.Describe, netdef *protocol.NetworkDefinition, nodeKey []byte, batch *database.Batch, anchor protocol.TransactionBody, sequenceNumber uint64, destPartUrl *url.URL) (*messaging.Envelope, error) {
	// TODO Exporting this is not great

	txn := new(protocol.Transaction)
	txn.Header.Principal = destPartUrl.JoinPath(protocol.AnchorPool)
	txn.Body = anchor

	// Create a synthetic origin signature
	initSig, err := new(signing.Builder).
		SetUrl(network.NodeUrl()).
		SetVersion(sequenceNumber).
		InitiateSynthetic(txn, destPartUrl)
	if err != nil {
		return nil, errors.InternalError.Wrap(err)
	}

	// Create a key signature
	keySig, err := SignTransaction(netdef, nodeKey, batch, txn, initSig.DestinationNetwork)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return &messaging.Envelope{Transaction: []*protocol.Transaction{txn}, Signatures: []protocol.Signature{initSig, keySig}}, nil
}
