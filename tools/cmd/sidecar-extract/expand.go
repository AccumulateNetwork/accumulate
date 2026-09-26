// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	badger "github.com/dgraph-io/badger"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// expand names lost keys that belong to messages no block carries — those the
// executor produced. Starting from every message of a height range of a block
// store, it follows each message's Produced and Cause sets, as the copies
// record them, to the messages they name, for -depth steps, and checks the
// records of every message it reaches against the lost list.
func expand(archiveArgs []string, blockstore, lostPath, out string, depth int) error {
	var copies []*archive
	for _, arg := range archiveArgs {
		n, path, _ := strings.Cut(arg, "=")
		db, err := badger.Open(badger.DefaultOptions(path).WithReadOnly(true).WithLogger(nil))
		if err != nil {
			return fmt.Errorf("archive %s: %w", n, err)
		}
		copies = append(copies, &archive{name: n, db: db, log: &vlog{dir: path}})
	}
	lost := map[[32]byte]bool{}
	f, err := os.Open(lostPath)
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, err := hex.DecodeString(strings.Fields(sc.Text())[0])
		if err == nil && len(k) == 32 {
			lost[[32]byte(k)] = true
		}
	}
	f.Close()
	store, err := leveldb.OpenFile(blockstore, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if err != nil {
		return err
	}
	defer store.Close()
	w, err := os.Create(out)
	if err != nil {
		return err
	}
	defer w.Close()

	found := map[[32]byte]bool{}
	kinds := map[string]int{}
	hit := func(k *record.Key, kind, what string) {
		h := k.Hash()
		if lost[h] && !found[h] {
			found[h] = true
			kinds[kind]++
			fmt.Fprintf(w, "%s\t%s\t%s\n", hex.EncodeToString(h[:]), kind, what)
		}
	}

	// The seeds: every message of the range
	type node struct {
		hash    [32]byte
		account *url.URL // the account a TxID named, if known
		depth   int
		via     string
	}
	var queue []node
	seen := map[[32]byte]bool{}
	for n := *flagFrom; n <= *flagTo; n++ {
		txs, ok, err := cometBlock(store, n)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		var visit func(m messaging.Message)
		visit = func(m messaging.Message) {
			if m == nil {
				return
			}
			h := m.Hash()
			if !seen[h] {
				seen[h] = true
				queue = append(queue, node{h, nil, 0, fmt.Sprintf("block %d %s", n, m.Type())})
			}
			switch m := m.(type) {
			case *messaging.TransactionMessage:
				th := *(*[32]byte)(m.Transaction.GetHash())
				if !seen[th] {
					seen[th] = true
					queue = append(queue, node{th, m.Transaction.Header.Principal, 0, fmt.Sprintf("block %d transaction", n)})
				}
			case *messaging.SyntheticMessage:
				visit(m.Message)
			case *messaging.BadSyntheticMessage:
				visit(m.Message)
			case *messaging.SequencedMessage:
				visit(m.Message)
			case *messaging.BlockAnchor:
				visit(m.Anchor)
			}
		}
		for _, m := range decodeBlock(txs) {
			visit(m)
		}
	}
	log.Printf("%d messages in blocks %d..%d", len(queue), *flagFrom, *flagTo)

	batch := coredb.New(&verifiedStore{copies}, nil).Begin(false)
	defer func() { batch.Discard() }()
	perDepth := map[int]int{}
	for i := 0; i < len(queue); i++ {
		if i%100000 == 99999 {
			batch.Discard()
			batch = coredb.New(&verifiedStore{copies}, nil).Begin(false)
			log.Printf("%d of %d messages; %d keys named", i, len(queue), len(found))
		}
		nd := queue[i]
		perDepth[nd.depth]++
		what := fmt.Sprintf("message %x (depth %d, %s)", nd.hash, nd.depth, nd.via)
		msg := batch.Message(nd.hash)
		hit(msg.Main().Key(), "message.main", what)
		hit(msg.Cause().Key(), "message.cause", what)
		hit(msg.Produced().Key(), "message.produced", what)
		hit(msg.Signers().Key(), "message.signers", what)
		for _, fld := range []string{"Main", "Status", "Produced", "Chains"} {
			hit(record.NewKey("Transaction", nd.hash, fld), "transaction."+strings.ToLower(fld), what)
		}
		if nd.account != nil {
			at := batch.Account(nd.account).Transaction(nd.hash)
			hit(at.Payments().Key(), "account.transaction.payments", what)
			hit(at.Votes().Key(), "account.transaction.votes", what)
			hit(at.Signatures().Key(), "account.transaction.signatures", what)
			hit(at.ValidatorSignatures().Key(), "account.transaction.validatorSignatures", what)
			hit(at.History().Key(), "account.transaction.history", what)
		}
		// Per-account records under every account the message names, for
		// every transaction it names
		if body, err := msg.Main().Get(); err == nil {
			var accts []*url.URL
			var ids []*url.TxID
			addID := func(id *url.TxID) {
				if id != nil {
					ids = append(ids, id)
					accts = append(accts, id.Account())
				}
			}
			switch m := body.(type) {
			case *messaging.TransactionMessage:
				accts = append(accts, m.Transaction.Header.Principal)
			case *messaging.SignatureMessage:
				accts = append(accts, signerOf(m.Signature))
				addID(m.TxID)
				if a, ok := m.Signature.(*protocol.AuthoritySignature); ok {
					accts = append(accts, a.Origin, a.Authority)
					accts = append(accts, a.Delegator...)
					addID(a.TxID)
					addID(a.Cause)
				}
			case *messaging.SignatureRequest:
				accts = append(accts, m.Authority)
				addID(m.TxID)
				addID(m.Cause)
			case *messaging.CreditPayment:
				accts = append(accts, m.Payer)
				addID(m.TxID)
				addID(m.Cause)
			case *messaging.BlockAnchor:
				accts = append(accts, signerOf(m.Signature))
			}
			hashes := [][32]byte{nd.hash}
			for _, id := range ids {
				hashes = append(hashes, id.Hash())
			}
			for _, acct := range accts {
				if acct == nil {
					continue
				}
				for _, th := range hashes {
					at := batch.Account(acct).Transaction(th)
					w := fmt.Sprintf("%v on transaction %x (from %s)", acct, th, what)
					hit(at.Payments().Key(), "account.transaction.payments", w)
					hit(at.Votes().Key(), "account.transaction.votes", w)
					hit(at.Signatures().Key(), "account.transaction.signatures", w)
					hit(at.ValidatorSignatures().Key(), "account.transaction.validatorSignatures", w)
					hit(at.History().Key(), "account.transaction.history", w)
					hit(batch.Account(acct).Pending().Key(), "account.pending", acct.String())
				}
			}
		}
		if nd.depth >= depth {
			continue
		}
		// Follow what it produced and what caused it
		for _, set := range []struct {
			name string
			get  func() ([]*url.TxID, error)
		}{{"produced", msg.Produced().Get}, {"cause", msg.Cause().Get}} {
			ids, err := set.get()
			if err != nil {
				continue
			}
			for _, id := range ids {
				h := id.Hash()
				if !seen[h] {
					seen[h] = true
					queue = append(queue, node{h, id.Account(), nd.depth + 1, fmt.Sprintf("%s of %x", set.name, nd.hash[:4])})
				}
			}
		}
	}

	fmt.Printf("%d messages reached %v; %d of %d lost keys named\n", len(queue), perDepth, len(found), len(lost))
	var ks []string
	for k := range kinds {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return kinds[ks[i]] > kinds[ks[j]] })
	for _, k := range ks {
		fmt.Printf("%8d %s\n", kinds[k], k)
	}
	return nil
}
