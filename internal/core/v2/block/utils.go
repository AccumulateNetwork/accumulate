// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// shouldIndexChain returns true if the given chain should be indexed.
func shouldIndexChain(_ *url.URL, _ string, typ protocol.ChainType) (bool, error) {
	switch typ {
	case protocol.ChainTypeIndex:
		// Index chains are unindexed
		return false, nil

	case protocol.ChainTypeTransaction:
		// Transaction chains are indexed
		return true, nil

	case protocol.ChainTypeAnchor:
		// Anchor chains are indexed
		return true, nil

	default:
		// m.logError("Unknown chain type", "type", typ, "name", name, "account", account)
		return false, fmt.Errorf("unknown chain type")
	}
}

// addIndexChainEntry adds an entry to an index chain.
func addIndexChainEntry(chain *database.Chain2, entry *protocol.IndexEntry) (uint64, error) {
	// Load the index chain
	indexChain, err := chain.Get()
	if err != nil {
		return 0, err
	}

	// Marshal the entry
	data, err := entry.MarshalBinary()
	if err != nil {
		return 0, err
	}

	// Add the entry
	_ = data
	err = indexChain.AddEntry(data, false)
	if err != nil {
		return 0, err
	}

	// Return the index of the entry
	return uint64(indexChain.Height() - 1), nil
}

// addChainAnchor anchors the target chain into the root chain, adding an index
// entry to the target chain's index chain, if appropriate.
func addChainAnchor(rootChain *database.Chain, chain *database.Chain2, blockIndex uint64) (indexIndex uint64, didIndex bool, err error) {
	// Load the chain
	accountChain, err := chain.Get()
	if err != nil {
		return 0, false, err
	}

	// Add its anchor to the root chain
	err = rootChain.AddEntry(accountChain.Anchor(), false)
	if err != nil {
		return 0, false, err
	}

	// Check if it should be indexed
	shouldIndex, err := shouldIndexChain(chain.Account(), chain.Name(), chain.Type())
	if err != nil || !shouldIndex {
		return 0, false, err
	}

	// Add the index chain entry
	indexIndex, err = addIndexChainEntry(chain.Index(), &protocol.IndexEntry{
		BlockIndex: blockIndex,
		Source:     uint64(accountChain.Height() - 1),
		Anchor:     uint64(rootChain.Height() - 1),
	})
	if err != nil {
		return 0, false, err
	}

	return indexIndex, true, nil
}

func (x *Executor) GetAccountAuthoritySet(batch *database.Batch, account protocol.Account) (*protocol.AccountAuth, error) {
	auth, url, err := getAccountAuthoritySet(account)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if auth != nil {
		return auth, nil
	}

	account, err = batch.Account(url).GetState()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return x.GetAccountAuthoritySet(batch, account)
}

func getValidator[T any](x *Executor, typ protocol.TransactionType) (T, bool) {
	var zero T

	txn, ok := x.executors[typ]
	if !ok {
		return zero, false
	}

	val, ok := txn.(T)
	return val, ok
}

func getAccountAuthoritySet(account protocol.Account) (*protocol.AccountAuth, *url.URL, error) {
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

func signTransaction(network *protocol.NetworkDefinition, nodeKey []byte, batch *database.Batch, txn *protocol.Transaction, destination *url.URL) (protocol.Signature, error) {
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

func prepareBlockAnchor(network *config.Describe, netdef *protocol.NetworkDefinition, nodeKey []byte, batch *database.Batch, anchor protocol.TransactionBody, sequenceNumber uint64, destPartUrl *url.URL) (*protocol.Envelope, error) {
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
	keySig, err := signTransaction(netdef, nodeKey, batch, txn, initSig.DestinationNetwork)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return &protocol.Envelope{Transaction: []*protocol.Transaction{txn}, Signatures: []protocol.Signature{initSig, keySig}}, nil
}
