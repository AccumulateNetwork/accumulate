// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/FactomProject/factomd/common/interfaces"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/factom"
)

var cmdConvert = &cobra.Command{
	Use:   "convert",
	Short: "Convert Factom records to Accumulate",
}

var cmdConvertEntries = &cobra.Command{
	Use:   "entries [output directory] [input directory]",
	Short: "Convert entries from a Factom object dump to Accumulate transactions",
	Args:  cobra.ExactArgs(2),
	Run:   convertEntries,
}

var cmdConvertChains = &cobra.Command{
	Use:   "chains [input directory] [output file]",
	Short: "Read converted Factom entries and create LDAs and chains for them",
	Args:  cobra.ExactArgs(2),
	Run:   convertChains,
}

var cmdConvertBalances = &cobra.Command{
	Use:   "balances [input directory] [output file]",
	Short: "Read balances from a Factom object dump and convert them to LTAs",
	Args:  cobra.ExactArgs(2),
	Run:   convertBalances,
}

var flagConvert = struct {
	LogLevel  string
	StartFrom int
	StopAt    int
}{}

func init() {
	cmd.AddCommand(cmdConvert)
	cmdConvert.AddCommand(
		cmdConvertEntries,
		cmdConvertChains,
		cmdConvertBalances,
	)
	cmdConvert.PersistentFlags().StringVarP(&flagConvert.LogLevel, "log-level", "l", "error", "Set the logging level")
	cmdConvert.PersistentFlags().IntVarP(&flagConvert.StartFrom, "start-from", "s", 0, "Factom height to start from")
	cmdConvert.PersistentFlags().IntVar(&flagConvert.StopAt, "stop-at", 0, "Factom height to stop at (if zero, run to completion)")
}

type EntryData struct {
	Metadata *factom.EntryMetadata
	Entry    interfaces.IEntry
}

type Metrics struct {
	Time    time.Time
	Blocks  int
	Entries int
	Read    int
	Written int
}

func (m *Metrics) Diff(n *Metrics) (time time.Duration, blocks, entries int, read, written uint64) {
	return m.Time.Sub(n.Time),
		m.Blocks - n.Blocks,
		m.Entries - n.Entries,
		uint64(m.Read - n.Read),
		uint64(m.Written - n.Written)
}

func convertEntries(_ *cobra.Command, args []string) {
	logger := newLogger()

	var start, last Metrics
	start.Time = time.Now()
	last = start
	readFactomDir(args[1], func(input []byte, height int) {
		current := last

		// Create a map of all the entries in the object file
		entries := map[[32]byte][]*EntryData{}

		// Entry index tracking map
		ebEntryIndex := map[[32]byte]int{}

		var dblockCount int
		var dblock, eblock interfaces.IHash
		var eblockChain [32]byte
		var dblockHeight uint32
		var eblockSeq uint32
		err := factom.ReadObjectFile(input, logger, func(header *factom.Header, object interface{}) {
			switch object := object.(type) {
			case interfaces.IDirectoryBlock:
				if object.GetDatabaseHeight() != uint32(height+dblockCount) {
					fatalf("Directory block height does not match calculated height")
				}
				dblockCount++
				dblock = object.GetKeyMR()
				dblockHeight = object.GetDatabaseHeight()

			case interfaces.IEntryBlock:
				if object.GetDatabaseHeight() != dblockHeight {
					fatalf("Entry block height does not match last directory block")
				}
				eblockChain = object.GetChainID().Fixed()
				eblockSeq = object.GetHeader().GetEBSequence()

				var err error
				eblock, err = object.KeyMR()
				check(err)

				// Reset index tracking map (save the space)
				for k := range ebEntryIndex {
					delete(ebEntryIndex, k)
				}

				// Build index tracking map
				for i, e := range object.GetBody().GetEBEntries() {
					ebEntryIndex[e.Fixed()] = i
				}

			case interfaces.IEntry:
				entry := object
				if entry.GetChainID().Fixed() != eblockChain {
					fatalf("Entry chain does not match last entry block")
				}

				index, ok := ebEntryIndex[entry.GetHash().Fixed()]
				if !ok {
					logger.Error("Cannot find entry in entry block", "height", dblockHeight, "eblock", logging.AsHex(eblock), "entry", logging.AsHex(entry.GetHash()))
					return
				}

				current.Entries++
				md := new(factom.EntryMetadata)
				md.DBlockHeight = uint64(dblockHeight)
				md.DBlockKeyMR = dblock.Fixed()
				md.EBlockSequenceNumber = uint64(eblockSeq)
				md.EBlockKeyMR = eblock.Fixed()
				md.EntryIndex = uint64(index)
				ed := EntryData{md, entry}
				chainId := entry.GetChainID().Fixed()
				entries[chainId] = append(entries[chainId], &ed)
			}
		})
		checkf(err, "process object file")

		// For each chain ID
		var txns []*snapshot.Transaction
		for chainId, entriesAll := range entries {
			// Format the URL
			chainId := chainId // See docs/developer/rangevarref.md
			address, err := protocol.LiteDataAddress(chainId[:])
			checkf(err, "create LDA URL")

			for len(entriesAll) > 0 {
				var entries []*EntryData
				if len(entriesAll) > 400 {
					entries = entriesAll[:400]
					entriesAll = entriesAll[400:]
				} else {
					entries = entriesAll
					entriesAll = entriesAll[:0]
				}

				// For each entry
				for _, e := range entries {
					// Convert the entry and calculate the entry hash
					entry := factom.ConvertEntry(e.Entry).Wrap()
					entryHash, err := protocol.ComputeFactomEntryHashForAccount(chainId[:], entry.GetData())
					checkf(err, "calculate entry hash")

					// Construct a transaction
					txn := new(protocol.Transaction)
					txn.Header.Principal = address
					txn.Body = &protocol.WriteData{Entry: entry}
					txn.Header.Metadata, err = e.Metadata.MarshalBinary()
					checkf(err, "marshal entry metadata")

					// Construct the transaction result
					result := new(protocol.WriteDataResult)
					result.AccountID = chainId[:]
					result.AccountUrl = address
					result.EntryHash = *(*[32]byte)(entryHash)

					// Construct the transaction status
					status := new(protocol.TransactionStatus)
					status.TxID = txn.ID()
					status.Code = errors.Delivered
					status.Result = result

					state := new(snapshot.Transaction)
					state.Transaction = txn
					state.Status = status
					txns = append(txns, state)
					b, _ := state.MarshalBinary()
					current.Written += len(b)
				}
			}
		}

		current.Time = time.Now()
		current.Read += len(input)
		current.Blocks = height + 2000

		time1, blocks1, entries1, read1, written1 := current.Diff(&last)
		time2, blocks2, entries2, read2, written2 := current.Diff(&start)
		time1s, time2s := time1.Seconds(), time2.Seconds()

		time1 = time1.Round(time.Millisecond)
		time2 = time2.Round(time.Millisecond)

		tw := tabwriter.NewWriter(os.Stdout, 1, 4, 1, ' ', 0)
		fmt.Fprintf(tw, "\tFile\t\tTotal\n")
		fmt.Fprintf(tw, "Run Time\t%v\t\t%v\n", time1, time2)
		fmt.Fprintf(tw, "Blocks\t%d\t(%.0f/s)\t%d\t(%.0f/s)\n", blocks1, float64(blocks1)/time1s, blocks2, float64(blocks2)/time2s)
		fmt.Fprintf(tw, "Entries\t%d\t(%.0f/s)\t%d\t(%.0f/s)\n", entries1, float64(entries1)/time1s, entries2, float64(entries2)/time2s)
		fmt.Fprintf(tw, "Data Read\t%s\t(%s/s)\t%s\t(%s/s)\n", humanize.IBytes(read1), humanize.IBytes(uint64(float64(read1)/time1s)), humanize.IBytes(read2), humanize.IBytes(uint64(float64(read2)/time2s)))
		fmt.Fprintf(tw, "Data Written\t%s\t(%s/s)\t%s\t(%s/s)\n", humanize.IBytes(written1), humanize.IBytes(uint64(float64(written1)/time1s)), humanize.IBytes(written2), humanize.IBytes(uint64(float64(written2)/time2s)))
		check(tw.Flush())

		last = current

		filename := filepath.Join(args[0], fmt.Sprintf("factom-%d.snapshot", height))
		fmt.Printf("Exporting as %s\n\n", filename)
		f, err := os.Create(filename)
		checkf(err, "create snapshot")
		defer f.Close()
		w, err := snapshot.Create(f, &snapshot.Header{Height: uint64(height)})
		checkf(err, "initialize snapshot")
		err = w.WriteTransactions(txns, true)
		checkf(err, "write transactions")
	})
}

func convertChains(_ *cobra.Command, args []string) {
	ok := true
	onInterrupt(func() { ok = false })
	height := flagConvert.StartFrom

	logger := newLogger()
	store := memory.New(logger.With("module", "storage"))
	batch := store.Begin(true)
	defer batch.Discard()

	v := new(chainVisitor)
	v.bpt = pmt.NewBPTManager(batch)
	for ok {
		filename := filepath.Join(args[0], fmt.Sprintf("factom-%d.snapshot", height))

		input, err := os.Open(filename)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		checkf(err, "read %s", filename)
		fmt.Printf("Processing %s\n", filename)
		check(snapshot.Visit(input, v))

		height += 2000
		if flagConvert.StopAt > 0 && height >= flagConvert.StopAt {
			break
		}
	}
	if !ok {
		fmt.Println("Interrupted")
	}

	hasher := make(hash.Hasher, 4)
	for _, a := range v.accounts {
		hasher = hasher[:0]
		b, _ := a.Main.MarshalBinary()
		hasher.AddBytes(b)
		hasher.AddValue(hashSecondaryState(a))
		hasher.AddValue(hashChains(a))
		hasher.AddValue(hashTransactions(a))
		chain, err := a.Main.(*protocol.LiteDataAccount).AccountId()
		check(err)
		v.bpt.InsertKV(*(*[32]byte)(chain), *(*[32]byte)(hasher.MerkleHash()))
	}
	check(v.bpt.Bpt.Update())

	f, err := os.Create(args[1])
	checkf(err, "create snapshot")
	w, err := snapshot.Create(f, new(snapshot.Header))
	checkf(err, "initialize snapshot")
	sw, err := w.Open(snapshot.SectionTypeAccounts)
	checkf(err, "open accounts snapshot")
	check(v.bpt.Bpt.SaveSnapshot(sw, func(key storage.Key, _ [32]byte) ([]byte, error) {
		b, err := v.lookup[key].MarshalBinary()
		if err != nil {
			return nil, errors.EncodingError.WithFormat("marshal account: %w", err)
		}
		return b, nil
	}))
	check(sw.Close())
}

type chainVisitor struct {
	accounts []*snapshot.Account
	lookup   map[[32]byte]*snapshot.Account
	bpt      *pmt.Manager
}

func (v *chainVisitor) VisitTransaction(txn *snapshot.Transaction, _ int) error {
	if txn == nil {
		return nil
	}

	body, ok := txn.Transaction.Body.(*protocol.WriteData)
	if !ok {
		return errors.BadRequest.WithFormat("expected %v, got %v", protocol.TransactionTypeWriteData, txn.Transaction.Body.Type())
	}
	entry, ok := body.Entry.(*protocol.FactomDataEntryWrapper)
	if !ok {
		return errors.BadRequest.WithFormat("expected %v, got %v", protocol.DataEntryTypeFactom, body.Entry.Type())
	}

	if v.lookup == nil {
		v.lookup = map[[32]byte]*snapshot.Account{}
	}

	account, ok := v.lookup[entry.AccountId]
	if ok {
		c := account.Chains[0]
		c.AddEntry(txn.Transaction.GetHash())
		return nil
	}

	address, err := protocol.LiteDataAddress(entry.AccountId[:])
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	lda := new(protocol.LiteDataAccount)
	lda.Url = address

	chain := new(snapshot.Chain)
	chain.Name = "main"
	chain.Type = protocol.ChainTypeTransaction
	chain.Head = new(database.MerkleState)
	chain.AddEntry(txn.Transaction.GetHash())

	account = new(snapshot.Account)
	account.Url = lda.Url
	account.Main = lda
	account.Chains = []*snapshot.Chain{chain}

	v.bpt.InsertKV(entry.AccountId, entry.AccountId)
	v.lookup[entry.AccountId] = account
	v.accounts = append(v.accounts, account)
	return nil
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

func hashChains(a *snapshot.Account) hash.Hasher {
	var hasher hash.Hasher
	for _, c := range a.Chains {
		if c.Head.Count == 0 {
			hasher.AddHash(new([32]byte))
		} else {
			hasher.AddHash((*[32]byte)(c.Head.GetMDRoot()))
		}
	}
	return hasher
}

func hashTransactions(*snapshot.Account) hash.Hasher {
	// No pending transactions
	return hash.Hasher{}
}

func convertBalances(_ *cobra.Command, args []string) {
	logger := newLogger()

	fct2acme := big.NewInt(5)
	balances := map[string]*big.Int{}
	update := func(addr string, amount uint64, fn func(z, x, y *big.Int) *big.Int) {
		balance, ok := balances[addr]
		if !ok {
			balance = new(big.Int)
			balances[addr] = balance
		}
		v := new(big.Int)
		v.SetUint64(amount)
		v.Mul(v, fct2acme)
		fn(balance, balance, v)
	}

	start := time.Now()
	last := start
	readFactomDir(args[0], func(input []byte, height int) {
		err := factom.ReadObjectFile(input, logger, func(header *factom.Header, object interface{}) {
			switch object := object.(type) {
			case interfaces.ITransaction:
				for _, in := range object.GetInputs() {
					update(in.GetUserAddress(), in.GetAmount(), (*big.Int).Sub)
				}
				for _, out := range object.GetOutputs() {
					update(out.GetUserAddress(), out.GetAmount(), (*big.Int).Add)
				}
			}
		})
		checkf(err, "process object file")

		current := time.Now()
		fmt.Printf("Run Time\t%v of %v\n", current.Sub(last), current.Sub(start))
		last = current
	})

	accounts := make([]*protocol.LiteTokenAccount, 0, len(balances))
	factoid := make(map[*protocol.LiteTokenAccount]string)
	for addr, balance := range balances {
		if balance.Sign() == 0 {
			// Do not import zero-balance factoid accounts
			continue
		}
		if balance.Sign() < 0 {
			fmt.Printf("%s has a negative balance\n", addr)
			continue
		}
		u, err := protocol.GetLiteAccountFromFactoidAddress(addr)
		check(err)
		account := new(protocol.LiteTokenAccount)
		account.Url = u
		account.Balance = *balance
		account.TokenUrl = protocol.AcmeUrl()
		accounts = append(accounts, account)
		factoid[account] = addr
	}

	store := memory.New(logger.With("module", "storage"))
	batch := store.Begin(true)
	defer batch.Discard()
	bpt := pmt.NewBPTManager(batch)

	hasher := make(hash.Hasher, 4)
	lookup := map[[32]byte]*snapshot.Account{}
	for _, lta := range accounts {
		hasher = hasher[:0]
		b, _ := lta.MarshalBinary()
		hasher.AddBytes(b)
		hasher.AddHash(new([32]byte))
		hasher.AddHash(new([32]byte))
		hasher.AddHash(new([32]byte))
		bpt.InsertKV(lta.Url.AccountID32(), *(*[32]byte)(hasher.MerkleHash()))
		lookup[lta.Url.AccountID32()] = &snapshot.Account{Main: lta}

		lid := new(protocol.LiteIdentity)
		lid.Url = lta.Url.RootIdentity()
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
	}
	check(bpt.Bpt.Update())

	f, err := os.Create(args[1])
	checkf(err, "create snapshot")
	w, err := snapshot.Create(f, new(snapshot.Header))
	checkf(err, "initialize snapshot")
	sw, err := w.Open(snapshot.SectionTypeAccounts)
	checkf(err, "open accounts snapshot")
	check(bpt.Bpt.SaveSnapshot(sw, func(key storage.Key, _ [32]byte) ([]byte, error) {
		b, err := lookup[key].MarshalBinary()
		if err != nil {
			return nil, errors.EncodingError.WithFormat("marshal account: %w", err)
		}
		return b, nil
	}))
	check(sw.Close())
}
