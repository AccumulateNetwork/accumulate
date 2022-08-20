package shared

import (
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
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
			return nil, nil, errors.StatusInternalError.Format("invalid key page URL: %v", account.Url)
		}
		return nil, bookUrl, nil

	default:
		return &protocol.AccountAuth{}, nil, nil
	}
}

func SignTransaction(network *config.Describe, nodeKey []byte, batch *database.Batch, txn *protocol.Transaction, destination *url.URL) (protocol.Signature, error) {
	// TODO Exporting this is not great

	if nodeKey == nil {
		return nil, errors.StatusInternalError.Format("attempted to sign with a nil key")
	}

	var page *protocol.KeyPage
	err := batch.Account(network.OperatorsPage()).GetStateAs(&page)
	if err != nil {
		return nil, errors.StatusUnknownError.Format("load operator key page: %w", err)
	}

	// Sign it
	bld := new(signing.Builder).
		SetType(protocol.SignatureTypeED25519).
		SetPrivateKey(nodeKey).
		SetUrl(config.NetworkUrl{URL: destination}.OperatorsPage()).
		SetVersion(1).
		SetTimestamp(1)

	keySig, err := bld.Sign(txn.GetHash())
	if err != nil {
		return nil, errors.StatusInternalError.Format("sign synthetic transaction: %w", err)
	}

	return keySig, nil
}

func PrepareBlockAnchor(network *config.Describe, nodeKey []byte, batch *database.Batch, anchor protocol.TransactionBody, sequenceNumber uint64, destPartUrl *url.URL) (*protocol.Envelope, error) {
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
		return nil, errors.StatusInternalError.Wrap(err)
	}

	// Create a key signature
	keySig, err := SignTransaction(network, nodeKey, batch, txn, initSig.DestinationNetwork)
	if err != nil {
		return nil, errors.StatusUnknownError.Wrap(err)
	}

	return &protocol.Envelope{Transaction: []*protocol.Transaction{txn}, Signatures: []protocol.Signature{initSig, keySig}}, nil
}
