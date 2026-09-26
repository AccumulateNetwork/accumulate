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
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// identify names lost keys by generating every record key a partition's model
// can name — each account reachable from the partition's root identity, the
// account's own records, every record of every chain it lists and of those
// chains' index chains, the records of every message those chains reference,
// every block ledger, and the partition's system records — and writing each
// key that is lost, with the object it belongs to, to -out.
//
// Chain record keys are built by appending to the chain's key the components
// the model appends; the first of each is checked against the model's own.
func identify(archiveArgs []string, lostPath, sidecarPath, out, partition string, height uint64) error {
	var copies []*archive
	for _, arg := range archiveArgs {
		n, path, _ := strings.Cut(arg, "=")
		db, err := badger.Open(badger.DefaultOptions(path).WithReadOnly(true).WithLogger(nil))
		if err != nil {
			return fmt.Errorf("archive %s: %w", n, err)
		}
		copies = append(copies, &archive{name: n, db: db, log: &vlog{dir: path}})
	}

	// Lost: listed, and not since recovered into the sidecar
	side, err := leveldb.OpenFile(sidecarPath, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if err != nil {
		return err
	}
	lost := map[[32]byte]bool{}
	f, err := os.Open(lostPath)
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, err := hex.DecodeString(strings.Fields(sc.Text())[0])
		if err != nil || len(k) != 32 {
			return fmt.Errorf("lost list: %q", sc.Text())
		}
		if ok, _ := side.Has(k, nil); !ok {
			lost[[32]byte(k)] = true
		}
	}
	f.Close()
	side.Close()
	log.Printf("%d keys lost", len(lost))

	w, err := os.Create(out)
	if err != nil {
		return err
	}
	defer w.Close()
	found := map[[32]byte]bool{}
	kinds := map[string]int{}
	var generated int64
	hit := func(k *record.Key, kind, detail string) {
		generated++
		h := k.Hash()
		if !lost[h] || found[h] {
			return
		}
		found[h] = true
		kinds[kind]++
		fmt.Fprintf(w, "%s\t%s\t%s\n", hex.EncodeToString(h[:]), kind, detail)
	}

	newBatch := func() *coredb.Batch { return coredb.New(&verifiedStore{archives: copies}, nil).Begin(false) }
	batch := newBatch()
	defer func() { batch.Discard() }()

	checked := false
	checkChainKeys := func(c *coredb.Chain2) {
		if checked {
			return
		}
		checked = true
		m := c.Inner()
		for _, p := range []struct {
			mine, model *record.Key
		}{
			{m.Key().Append("Element", uint64(1)), m.Element(1).Key()},
			{m.Key().Append("States", uint64(255)), m.States(255).Key()},
			{m.Key().Append("Head"), m.Head().Key()},
			{m.Key().Append("ElementIndex", []byte{1, 2, 3}), m.ElementIndex([]byte{1, 2, 3}).Key()},
		} {
			if p.mine.Hash() != p.model.Hash() {
				log.Fatalf("chain key %v is not the model's %v", p.mine, p.model)
			}
		}
		log.Printf("chain record keys match the model's")
	}

	// Every record of a chain
	messages := map[[32]byte]*url.URL{} // message hash -> an account whose chain holds it
	chainRecords := func(acct *url.URL, c *coredb.Chain2, collect bool) {
		checkChainKeys(c)
		m := c.Inner()
		name := c.Name()
		d := func(s string) string { return fmt.Sprintf("%v %s %s", acct, name, s) }
		hit(m.Key().Append("Head"), "chain.head", d("head"))
		head, err := m.Head().Get()
		if err != nil {
			return
		}
		freq := m.MarkFreq()
		for i := int64(0); i < head.Count; i++ {
			hit(m.Key().Append("Element", uint64(i)), "chain.element", d(fmt.Sprintf("element %d of %d", i, head.Count)))
			if (i+1)%freq == 0 {
				hit(m.Key().Append("States", uint64(i)), "chain.markpoint", d(fmt.Sprintf("mark point %d of %d", i, head.Count)))
			}
		}
		// The entry hashes, from the mark points and the head
		var idx int64
		addHashes := func(hs [][]byte) {
			for _, h := range hs {
				hit(m.Key().Append("ElementIndex", h), "chain.elementIndex", d(fmt.Sprintf("element index of entry %d", idx)))
				if collect && len(h) == 32 {
					if _, ok := messages[[32]byte(h)]; !ok {
						messages[[32]byte(h)] = acct
					}
				}
				idx++
			}
		}
		for mk := freq - 1; mk < head.Count; mk += freq {
			s, err := m.States(uint64(mk)).Get()
			if err != nil {
				idx = mk + 1
				continue
			}
			addHashes(s.HashList)
		}
		idx = head.Count / freq * freq
		addHashes(head.HashList)
	}

	// Every account reachable through directories, with its records
	root := protocol.PartitionUrl(partition).RootIdentity()
	if partition == protocol.Directory {
		root = protocol.DnUrl()
	}
	queue := []*url.URL{root}
	seen := map[string]bool{root.String(): true}
	var accounts int
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		accounts++
		batch.Discard()
		batch = newBatch()
		a := batch.Account(u)
		d := u.String()
		hit(a.Main().Key(), "account.main", d)
		hit(a.Pending().Key(), "account.pending", d)
		hit(a.Chains().Key(), "account.chains", d)
		hit(a.Directory().Key(), "account.directory", d)
		hit(a.SyntheticAnchors().Key(), "account.syntheticAnchors", d)
		hit(a.DirectoryAnchorBlock().Key(), "account.directoryAnchorBlock", d)
		hit(a.Data().Entry().Key(), "account.data.entry", d)
		if dir, err := a.Directory().Get(); err == nil {
			for _, s := range dir {
				if !seen[s.String()] {
					seen[s.String()] = true
					queue = append(queue, s)
				}
			}
		}
		metas, err := a.Chains().Get()
		if err != nil {
			continue
		}
		for _, meta := range metas {
			c, err := a.ChainByName(meta.Name)
			if err != nil {
				continue
			}
			// A transaction chain's entries are messages
			chainRecords(u, c, c.Type() == merkle.ChainTypeTransaction)
			if c.Type() != merkle.ChainTypeIndex {
				chainRecords(u, c.Index(), false)
			}
		}
		log.Printf("account %v: %d chains; %d keys named so far", u, len(metas), len(found))
	}

	// Every message the chains reference
	batch.Discard()
	batch = newBatch()
	var n int
	for h, acct := range messages {
		if n++; n%200000 == 0 {
			batch.Discard()
			batch = newBatch()
			log.Printf("messages: %d of %d; %d keys named so far", n, len(messages), len(found))
		}
		msg := batch.Message(h)
		d := fmt.Sprintf("message %x (on a chain of %v)", h, acct)
		hit(msg.Main().Key(), "message.main", d)
		hit(msg.Cause().Key(), "message.cause", d)
		hit(msg.Produced().Key(), "message.produced", d)
		hit(msg.Signers().Key(), "message.signers", d)
		for _, fld := range []string{"Main", "Status", "Produced", "Chains"} {
			hit(record.NewKey("Transaction", h, fld), "transaction."+strings.ToLower(fld), d)
		}
		at := batch.Account(acct).Transaction(h)
		hit(at.Payments().Key(), "account.transaction.payments", d)
		hit(at.Votes().Key(), "account.transaction.votes", d)
		hit(at.Signatures().Key(), "account.transaction.signatures", d)
		hit(at.ValidatorSignatures().Key(), "account.transaction.validatorSignatures", d)
		hit(at.History().Key(), "account.transaction.history", d)
	}

	// Every block ledger, and the system records of every block
	lu := protocol.PartitionUrl(partition).JoinPath(protocol.Ledger)
	sys := batch.SystemData(partition)
	hit(sys.ExecutedBlock().Key(), "system.executedBlock", partition)
	for b := uint64(1); b <= height; b++ {
		if b%2000000 == 0 {
			batch.Discard()
			batch = newBatch()
			sys = batch.SystemData(partition)
			log.Printf("blocks: %d of %d; %d keys named so far", b, height, len(found))
		}
		bu := lu.JoinPath(fmt.Sprint(b))
		// Built from the account's key, so no record objects pile up
		ak := record.NewKey("Account", bu)
		hit(ak.Append("Main"), "ledger.main", bu.String())
		hit(ak.Append("Chains"), "ledger.chains", bu.String())
		hit(ak.Append("Pending"), "ledger.pending", bu.String())
		hit(sys.SyntheticIndexIndex(b).Key(), "system.syntheticIndexIndex", fmt.Sprintf("block %d", b))
	}
	if ak := record.NewKey("Account", lu.JoinPath("1")).Append("Main"); ak.Hash() != batch.Account(lu.JoinPath("1")).Main().Key().Hash() {
		return fmt.Errorf("block ledger keys do not match the model's")
	}

	fmt.Printf("%s: %d accounts, %d messages, %d keys generated; %d of %d lost keys named\n",
		partition, accounts, len(messages), generated, len(found), len(lost))
	var ks []string
	for k := range kinds {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return kinds[ks[i]] > kinds[ks[j]] })
	for _, k := range ks {
		fmt.Printf("%8d %s\n", kinds[k], k)
	}
	un, err := os.Create(out + ".unnamed")
	if err != nil {
		return err
	}
	defer un.Close()
	for k := range lost {
		if !found[k] {
			fmt.Fprintf(un, "%x\n", k)
		}
	}
	return nil
}
