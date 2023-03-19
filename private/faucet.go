// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package private

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"math"
	"math/big"
	"sync"

	"github.com/tendermint/tendermint/libs/log"
	ip2p "gitlab.com/accumulatenetwork/accumulate/internal/api/p2p"
	v3impl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type PrivateP2P = ip2p.Node

func CreateLiteFaucet(lite *url.URL) ([]byte, error) {
	store := memory.New(nil)
	batch := store.Begin(true)
	defer batch.Discard()
	bpt := pmt.NewBPTManager(batch)

	lta := new(protocol.LiteTokenAccount)
	lta.Url = lite.JoinPath(protocol.ACME)
	lta.TokenUrl = protocol.AcmeUrl()
	lta.Balance = *big.NewInt(200_000_000 * protocol.AcmePrecision)
	fmt.Printf("Faucet: %v\n", lta.Url)

	hasher := make(hash.Hasher, 4)
	lookup := map[[32]byte]*snapshot.Account{}

	hasher = hasher[:0]
	b, _ := lta.MarshalBinary()
	hasher.AddBytes(b)
	hasher.AddHash(new([32]byte))
	hasher.AddHash(new([32]byte))
	hasher.AddHash(new([32]byte))
	bpt.InsertKV(lta.Url.AccountID32(), *(*[32]byte)(hasher.MerkleHash()))
	lookup[lta.Url.AccountID32()] = &snapshot.Account{Main: lta}

	lid := new(protocol.LiteIdentity)
	lid.Url = lite
	lid.CreditBalance = math.MaxUint64
	a := new(snapshot.Account)
	a.Url = lid.Url
	a.Main = lid
	a.Directory = []*url.URL{lta.Url}
	hasher = hasher[:0]
	b, _ = lid.MarshalBinary()
	hasher.AddBytes(b)
	hasher.AddValue(hashSecondaryState(a))
	hasher.AddHash(new([32]byte))
	hasher.AddHash(new([32]byte))
	bpt.InsertKV(lid.Url.AccountID32(), *(*[32]byte)(hasher.MerkleHash()))
	lookup[lid.Url.AccountID32()] = a

	err := bpt.Bpt.Update()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	buf := new(ioutil2.Buffer)
	w, err := snapshot.Create(buf, new(snapshot.Header))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("initialize snapshot: %w", err)
	}

	sw, err := w.Open(snapshot.SectionTypeAccounts)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open accounts snapshot: %w", err)
	}

	err = bpt.Bpt.SaveSnapshot(sw, func(key storage.Key, _ [32]byte) ([]byte, error) {
		b, err := lookup[key].MarshalBinary()
		if err != nil {
			return nil, errors.EncodingError.WithFormat("marshal account: %w", err)
		}
		return b, nil
	})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	err = sw.Close()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return buf.Bytes(), nil
}

func hashSecondaryState(a *snapshot.Account) hash.Hasher {
	var hasher hash.Hasher
	for _, u := range a.Directory {
		hasher.AddUrl(u)
	}
	// Hash the hash to allow for future expansion
	dirHash := hasher.MerkleHash()
	return hash.Hasher{dirHash}
}

func LaunchFaucet(network string, liteKey ed25519.PrivateKey, logger log.Logger, done *sync.WaitGroup, stop chan struct{}, connectCb func(*ip2p.Node) error) error {
	faucet, err := protocol.LiteTokenAddress(liteKey[32:], "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		return err
	}

	// Create the faucet node
	opts := p2p.Options{
		Network: network,
		Key:     liteKey,
		Logger:  logger,
	}
	inode, err := ip2p.New(opts)
	if err != nil {
		return err
	}

	connectCb(inode)

	node, err := p2p.NewWith(inode, opts)
	if err != nil {
		return err
	}

	// Create the faucet service
	faucetSvc, err := v3impl.NewFaucet(context.Background(), v3impl.FaucetParams{
		Logger:    logger.With("module", "faucet"),
		Account:   faucet,
		Key:       build.ED25519PrivateKey(liteKey),
		Submitter: node,
		Querier:   node,
		Events:    node,
	})
	if err != nil {
		return err
	}

	// Cleanup
	done.Add(1)
	go func() {
		defer done.Done()
		<-stop
		faucetSvc.Stop()
		err = node.Close()
		if err != nil {
			logger.Error("Close faucet node", "error", err)
		}
	}()

	// Register it
	handler, err := message.NewHandler(logger, message.Faucet{Faucet: faucetSvc})
	if err != nil {
		return err
	}
	if !node.RegisterService(faucetSvc.Type().AddressForUrl(protocol.AcmeUrl()), handler.Handle) {
		return errors.InternalError.With("failed to register faucet service")
	}

	return nil
}
