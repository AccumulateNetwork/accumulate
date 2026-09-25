// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	badger "github.com/dgraph-io/badger"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// lostMap names the lost keys a partition's blocks can account for. For every
// block of the block store it generates the record keys the block implies —
// the records of each of its messages, of its block ledger, and of each chain
// entry the ledger lists — and reports, by record kind, how many lost keys
// they hit. A lost message body a block carries is written to recovered.db,
// keyed as the store keys it; the message hashes to its own key, so the value
// is checked, not trusted.
func lostMap(archiveArg, blockstore, lostPath, out string, workers int) error {
	lost := map[[32]byte]bool{}
	f, err := os.Open(lostPath)
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, err := hex.DecodeString(strings.Fields(sc.Text())[0])
		if err != nil || len(k) != 32 {
			return fmt.Errorf("lost.log: %q", sc.Text())
		}
		lost[[32]byte(k)] = true
	}
	f.Close()

	name, path, _ := strings.Cut(archiveArg, "=")
	db, err := badger.Open(badger.DefaultOptions(path).WithReadOnly(true).WithLogger(nil))
	if err != nil {
		return fmt.Errorf("archive %s: %w", name, err)
	}
	a := &archive{name: name, db: db, log: &vlog{dir: path}}
	store, err := leveldb.OpenFile(blockstore, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if err != nil {
		return err
	}
	defer store.Close()
	recovered, err := leveldb.OpenFile(out, nil)
	if err != nil {
		return err
	}
	defer recovered.Close()

	lu, err := url.Parse(*flagLedgerURL)
	if err != nil {
		return fmt.Errorf("-ledger-url: %w", err)
	}

	var mu sync.Mutex
	hits := map[string]int{}       // record kind -> lost keys hit
	found := map[[32]byte]string{} // lost key -> kind
	var bodies, badBodies, blocks, ledgers int64
	var ctlTxns, ctlMessage, ctlTxMain, ctlTxStatus, ctlSame, ctlDiffer int64
	exists := func(k *record.Key) bool {
		h := k.Hash()
		txn := a.db.NewTransaction(false)
		defer txn.Discard()
		_, err := txn.Get(h[:])
		return err == nil
	}

	// Transaction records have no exported accessor; their keys are built by
	// hand, so the derivation is proved on a control before it is trusted
	txKey := func(h [32]byte, field string) *record.Key { return record.NewKey("Transaction", h, field) }

	hit := func(kind string, k *record.Key) bool {
		h := k.Hash()
		if !lost[h] {
			return false
		}
		mu.Lock()
		if _, ok := found[h]; !ok {
			found[h] = kind
			hits[kind]++
		}
		mu.Unlock()
		return true
	}

	base, height := storeRange(store)
	if *flagFrom > base {
		base = *flagFrom
	}
	var next atomic.Uint64
	next.Store(base)
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	start := time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A batch caches what it reads, so one is not kept for long
			batch := coredb.New(&verifiedStore{[]*archive{a}}, nil).Begin(false)
			defer func() { batch.Discard() }()
			for done := 0; ; done++ {
				if done%2000 == 1999 {
					batch.Discard()
					batch = coredb.New(&verifiedStore{[]*archive{a}}, nil).Begin(false)
				}
				n := next.Add(1) - 1
				if n > height {
					return
				}
				if n%100000 == 0 {
					mu.Lock()
					log.Printf("block %d of %d..%d: %d lost keys named, %d bodies recovered, %v", n, base, height, len(found), bodies, time.Since(start).Round(time.Second))
					mu.Unlock()
				}
				txs, ok, err := cometBlock(store, n)
				if err != nil {
					errs <- err
					return
				}
				if ok {
					atomic.AddInt64(&blocks, 1)
				}

				// The records of each message in the block
				for _, tx := range txs {
					env := new(messaging.Envelope)
					if env.UnmarshalBinary(tx) != nil {
						continue
					}
					msgs, err := env.Normalize()
					if err != nil {
						continue
					}
					var visit func(m messaging.Message)
					visit = func(m messaging.Message) {
						if m == nil {
							return
						}
						h := m.Hash()
						rec := batch.Message(h)
						if hit("message.main", rec.Main().Key()) {
							// The block carries the lost body
							b, err := m.MarshalBinary()
							if err == nil {
								kh := rec.Main().Key().Hash()
								if err := recovered.Put(kh[:], b, nil); err != nil {
									errs <- err
								}
								atomic.AddInt64(&bodies, 1)
							} else {
								atomic.AddInt64(&badBodies, 1)
							}
						}
						hit("message.cause", rec.Cause().Key())
						hit("message.produced", rec.Produced().Key())
						hit("message.signers", rec.Signers().Key())
						for _, f := range []string{"Main", "Status", "Produced", "Chains"} {
							hit("transaction."+strings.ToLower(f), txKey(h, f))
						}
						switch m := m.(type) {
						case *messaging.TransactionMessage:
							th := *(*[32]byte)(m.Transaction.GetHash())
							// The control: keys the archive should hold
							if atomic.AddInt64(&ctlTxns, 1) <= 2000 {
								if exists(batch.Message(th).Main().Key()) {
									atomic.AddInt64(&ctlMessage, 1)
									// A recovered body is the block's message
									// marshaled again; it must be the bytes the
									// node stored
									kh := batch.Message(th).Main().Key().Hash()
									txn := a.db.NewTransaction(false)
									if item, err := txn.Get(kh[:]); err == nil {
										stored, err1 := a.value(item)
										mine, err2 := m.MarshalBinary()
										if err1 == nil && err2 == nil && bytes.Equal(stored, mine) {
											atomic.AddInt64(&ctlSame, 1)
										} else {
											atomic.AddInt64(&ctlDiffer, 1)
										}
									}
									txn.Discard()
								}
								if exists(txKey(th, "Main")) {
									atomic.AddInt64(&ctlTxMain, 1)
								}
								if exists(txKey(th, "Status")) {
									atomic.AddInt64(&ctlTxStatus, 1)
								}
							}
							for _, f := range []string{"Main", "Status", "Produced", "Chains"} {
								hit("transaction."+strings.ToLower(f), txKey(th, f))
							}
							if p := m.Transaction.Header.Principal; p != nil {
								at := batch.Account(p).Transaction(th)
								hit("account.transaction.payments", at.Payments().Key())
								hit("account.transaction.votes", at.Votes().Key())
								hit("account.transaction.signatures", at.Signatures().Key())
								hit("account.transaction.validatorSignatures", at.ValidatorSignatures().Key())
								hit("account.transaction.history", at.History().Key())
								hit("account.pending", batch.Account(p).Pending().Key())
							}
						case *messaging.SignatureMessage:
							if m.Signature != nil && m.TxID != nil {
								at := batch.Account(m.Signature.GetSigner()).Transaction(m.TxID.Hash())
								hit("account.transaction.signatures", at.Signatures().Key())
								hit("account.transaction.history", at.History().Key())
								hit("account.transaction.votes", at.Votes().Key())
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
					for _, m := range msgs {
						visit(m)
					}
				}

				// The block ledger and the chain entries it lists
				acct := batch.Account(lu.JoinPath(fmt.Sprint(n)))
				hit("ledger.main", acct.Main().Key())
				hit("ledger.chains", acct.Chains().Key())
				var bl *protocol.BlockLedger
				if acct.Main().GetAs(&bl) != nil {
					continue
				}
				atomic.AddInt64(&ledgers, 1)
				for _, e := range bl.Entries {
					ea := batch.Account(e.Account)
					hit("account.main", ea.Main().Key())
					hit("account.chains", ea.Chains().Key())
					hit("account.pending", ea.Pending().Key())
					chain, err := ea.ChainByName(e.Chain)
					if err != nil {
						continue
					}
					c := chain.Inner()
					kind := chainKind.ReplaceAllString(e.Chain, "()")
					hit("chain.element:"+kind, c.Element(e.Index).Key())
					hit("chain.head:"+kind, c.Head().Key())
					if (int64(e.Index)+1)%c.MarkFreq() == 0 {
						hit("chain.markpoint:"+kind, c.States(e.Index).Key())
					}
					if v, err := c.Element(e.Index).Get(); err == nil {
						hit("chain.elementIndex:"+kind, c.ElementIndex(v).Key())
					}
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}

	fmt.Printf("%s: blocks %d..%d, %d blocks read, %d block ledgers; %d of %d lost keys named; %d bodies recovered (%d would not marshal)\n",
		name, base, height, blocks, ledgers, len(found), len(lost), bodies, badBodies)
	fmt.Printf("control, first 2000 transactions: Message.Main held %d (stored bytes = remarshaled %d, differ %d), hand-built Transaction.Main held %d, Transaction.Status held %d\n",
		ctlMessage, ctlSame, ctlDiffer, ctlTxMain, ctlTxStatus)
	var kinds []string
	for k := range hits {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool { return hits[kinds[i]] > hits[kinds[j]] })
	for _, k := range kinds {
		fmt.Printf("%10d %s\n", hits[k], k)
	}

	// What is still unnamed
	un, err := os.Create(out + ".unnamed")
	if err != nil {
		return err
	}
	defer un.Close()
	for k := range lost {
		if _, ok := found[k]; !ok {
			fmt.Fprintf(un, "%x\n", k)
		}
	}
	return nil
}
