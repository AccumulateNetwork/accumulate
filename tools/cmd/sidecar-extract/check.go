// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	v3 "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// checkPage is the API's largest page; the bridge reads history in pages.
const checkPage = 100

// checkResult is what the bridge's queries get for one account from one
// layering of the stores.
type checkResult struct {
	Account *url.URL
	Layer   string

	// query-tx-history over the whole main chain: Read of Total entries
	// expanded to their transaction; Wrong lists entries whose transaction is
	// not the entry (#4269 §2).
	Total      uint64
	Read       uint64
	Wrong      []uint64
	Types      map[string]int // transactions read, by body type
	HistoryErr error

	// query-data and query-data-set, for a data account: DataRead of
	// DataTotal entries expanded, LatestErr from the url-only query the
	// bridge makes (#4268). DataWrong lists entries whose transaction does
	// not write them.
	IsData    bool
	Skipped   bool   // not a data account, and only those were asked for
	Hidden    uint64 // sidecar index entries at and beyond the live count, which a restarted index hides
	DataTotal uint64
	DataRead  uint64
	DataWrong []uint64
	LatestErr error
	DataErr   error
}

// check runs, for each account, the queries the bridge makes — query-tx-history
// over the whole main chain, query-data for the newest entry and query-data-set
// over every entry — through the API's own Querier, from the live database
// alone and then with the sidecar behind it (#4268, #4269). Every store is
// opened read-only.
func check(currentPaths []string, sidecarPath, partition, accountsPath string, dataOnly bool, accounts []string) error {
	var live []*leveldb.DB
	for _, p := range currentPaths {
		db, err := leveldb.OpenFile(p, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
		if err != nil {
			return fmt.Errorf("current %s: %w", p, err)
		}
		defer db.Close()
		live = append(live, db)
	}
	side, err := leveldb.OpenFile(sidecarPath, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if err != nil {
		return fmt.Errorf("sidecar: %w", err)
	}
	defer side.Close()

	if accountsPath != "" {
		b, err := os.ReadFile(accountsPath)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(b), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				accounts = append(accounts, line)
			}
		}
	}
	var urls []*url.URL
	for _, s := range accounts {
		u, err := url.Parse(s)
		if err != nil {
			return err
		}
		urls = append(urls, u)
	}
	sums := map[string]*checkSummary{}
	for _, r := range runChecks(live, side, partition, dataOnly, urls) {
		if r.Skipped {
			continue
		}
		fmt.Println(r)
		if sums[r.Layer] == nil {
			sums[r.Layer] = new(checkSummary)
		}
		sums[r.Layer].add(r)
	}
	for _, layer := range []string{"live", "live+sidecar"} {
		if s := sums[layer]; s != nil {
			fmt.Printf("summary\t%s\t%s\n", layer, s)
		}
	}
	return nil
}

// checkSummary counts accounts by what their queries returned.
type checkSummary struct {
	Accounts, HistoryWhole, HistoryWrong, DataAccounts, DataWhole, LatestOK, Restarted int
}

func (s *checkSummary) add(r checkResult) {
	s.Accounts++
	if r.HistoryErr == nil && r.Read == r.Total {
		s.HistoryWhole++
	}
	if len(r.Wrong) > 0 {
		s.HistoryWrong++
	}
	if !r.IsData {
		return
	}
	s.DataAccounts++
	if r.DataErr == nil && r.DataRead == r.DataTotal && len(r.DataWrong) == 0 {
		s.DataWhole++
	}
	if r.LatestErr == nil {
		s.LatestOK++
	}
	if r.Hidden > 0 {
		s.Restarted++
	}
}

func (s *checkSummary) String() string {
	return fmt.Sprintf("accounts %d, history whole %d, history with a wrong entry %d, data accounts %d, data whole %d, latest ok %d, index restarted %d",
		s.Accounts, s.HistoryWhole, s.HistoryWrong, s.DataAccounts, s.DataWhole, s.LatestOK, s.Restarted)
}

// runChecks checks each account from the live stores alone, then with the
// sidecar behind them.
func runChecks(live []*leveldb.DB, side *leveldb.DB, partition string, dataOnly bool, accounts []*url.URL) []checkResult {
	layers := []struct {
		name string
		dbs  []*leveldb.DB
	}{{"live", live}, {"live+sidecar", append(live[:len(live):len(live)], side)}}

	var results []checkResult
	for _, u := range accounts {
		for _, l := range layers {
			db := coredb.New(layered(l.dbs), nil)
			q := api.Querier2{Querier: v3.NewQuerier(v3.QuerierParams{Database: db, Partition: partition})}
			r := checkAccount(q, db, u, dataOnly)
			r.Layer = l.name
			if l.name == "live" && r.IsData && r.DataTotal > 0 {
				r.Hidden = hidden(side, u, r.DataTotal)
			}
			results = append(results, r)
		}
	}
	return results
}

func checkAccount(q api.Querier2, db *coredb.Database, u *url.URL, dataOnly bool) checkResult {
	ctx := context.Background()
	r := checkResult{Account: u}

	batch := db.Begin(false)
	defer batch.Discard()
	main, err := batch.Account(u).Main().Get()
	if err == nil {
		switch main.Type() {
		case protocol.AccountTypeDataAccount, protocol.AccountTypeLiteDataAccount:
			r.IsData = true
		}
	}
	if dataOnly && !r.IsData {
		r.Skipped = true
		return r
	}

	// query-tx-history, page by page; a page the API refuses is read entry
	// by entry so the count is of entries, not pages
	expand := true
	for start := uint64(0); ; start += checkPage {
		count := uint64(checkPage)
		page, err := q.QueryMainChainEntries(ctx, u, &api.ChainQuery{Name: "main", Range: &api.RangeOptions{Start: start, Count: &count, Expand: &expand}})
		if err != nil {
			if r.HistoryErr == nil {
				r.HistoryErr = fmt.Errorf("history from %d: %w", start, err)
			}
			if r.Total == 0 {
				break
			}
			for i := start; i < start+checkPage && i < r.Total; i++ {
				e, err := q.QueryMainChainEntry(ctx, u, &api.ChainQuery{Name: "main", Index: &i})
				if err != nil {
					continue
				}
				r.countHistory(e)
			}
			if start+checkPage >= r.Total {
				break
			}
			continue
		}
		r.Total = page.Total
		for _, e := range page.Records {
			r.countHistory(e)
		}
		if start+checkPage >= page.Total {
			break
		}
	}

	// data: the newest entry as query-data asks for it, then every entry as
	// query-data-set does
	if !r.IsData {
		return r
	}
	_, r.LatestErr = q.QueryDataEntry(ctx, u, &api.DataQuery{})
	for start := uint64(0); ; start += checkPage {
		count := uint64(checkPage)
		page, err := q.QueryDataEntries(ctx, u, &api.DataQuery{Range: &api.RangeOptions{Start: start, Count: &count, Expand: &expand}})
		if err != nil {
			if r.DataErr == nil {
				r.DataErr = fmt.Errorf("data from %d: %w", start, err)
			}
			if r.DataTotal == 0 {
				break
			}
			for i := start; i < start+checkPage && i < r.DataTotal; i++ {
				e, err := q.QueryDataEntry(ctx, u, &api.DataQuery{Index: &i})
				if err != nil {
					continue
				}
				r.countData(e)
			}
			if start+checkPage >= r.DataTotal {
				break
			}
			continue
		}
		r.DataTotal = page.Total
		for _, e := range page.Records {
			r.countData(e)
		}
		if start+checkPage >= page.Total {
			break
		}
	}
	return r
}

// hidden counts the sidecar's index entries from the live count on: a data
// index the live store restarted after the reorg counts only its own entries,
// so the sidecar's at and beyond that count are never read.
func hidden(side *leveldb.DB, u *url.URL, liveCount uint64) uint64 {
	batch := coredb.New(memory.New(nil), nil).Begin(false)
	defer batch.Discard()
	key := batch.Account(u).Data().Entry().Key()
	var n uint64
	for ; n < 1<<20; n++ {
		h := key.Append(int(liveCount + n)).Hash()
		if _, err := side.Get(h[:], nil); err != nil {
			break
		}
	}
	return n
}

func (r *checkResult) countHistory(e *api.ChainEntryRecord[*api.MessageRecord[*messaging.TransactionMessage]]) {
	if e == nil || e.Value == nil || e.Value.Message == nil {
		return
	}
	r.Read++
	if e.Value.ID.Hash() != e.Entry {
		r.Wrong = append(r.Wrong, e.Index)
	}
	if r.Types == nil {
		r.Types = map[string]int{}
	}
	if e.Value.Message.Transaction != nil && e.Value.Message.Transaction.Body != nil {
		r.Types[e.Value.Message.Transaction.Body.Type().String()]++
	}
}

func (r *checkResult) countData(e *api.ChainEntryRecord[*api.MessageRecord[*messaging.TransactionMessage]]) {
	if e == nil || e.Value == nil || e.Value.Message == nil {
		return
	}
	r.DataRead++
	var entry protocol.DataEntry
	switch body := e.Value.Message.Transaction.Body.(type) {
	case *protocol.WriteData:
		entry = body.Entry
	case *protocol.WriteDataTo:
		entry = body.Entry
	case *protocol.SyntheticWriteData:
		entry = body.Entry
	case *protocol.SystemWriteData:
		entry = body.Entry
	}
	if entry == nil || [32]byte(entry.Hash()) != e.Entry {
		r.DataWrong = append(r.DataWrong, e.Index)
	}
}

func (r checkResult) String() string {
	s := fmt.Sprintf("%s\t%s\thistory %d of %d", r.Account, r.Layer, r.Read, r.Total)
	if len(r.Wrong) > 0 {
		s += fmt.Sprintf(", %d wrong (first %d)", len(r.Wrong), r.Wrong[0])
	}
	if r.HistoryErr != nil {
		s += fmt.Sprintf(" (%v)", r.HistoryErr)
	}
	if len(r.Types) > 0 {
		s += " [" + tally(r.Types) + "]"
	}
	if !r.IsData {
		return s
	}
	s += fmt.Sprintf("\tdata %d of %d", r.DataRead, r.DataTotal)
	if r.Hidden > 0 {
		s += fmt.Sprintf(", restarted: hides %d in the sidecar", r.Hidden)
	}
	if len(r.DataWrong) > 0 {
		s += fmt.Sprintf(", %d wrong (first %d)", len(r.DataWrong), r.DataWrong[0])
	}
	if r.DataErr != nil {
		s += fmt.Sprintf(" (%v)", r.DataErr)
	}
	if r.LatestErr != nil {
		s += fmt.Sprintf("\tlatest: %v", r.LatestErr)
	} else {
		s += "\tlatest ok"
	}
	return s
}

// layered reads each key from the first database that holds it. It cannot
// write.
type layered []*leveldb.DB

func (l layered) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	return memory.NewChangeSet(memory.ChangeSetOptions{
		Prefix: prefix,
		Get: func(key *record.Key) ([]byte, error) {
			h := key.Hash()
			for _, db := range l {
				v, err := db.Get(h[:], nil)
				if err == nil {
					return v, nil
				}
				if !errors.Is(err, leveldb.ErrNotFound) {
					return nil, err
				}
			}
			return nil, (*database.NotFoundError)(key)
		},
		Commit:  func(map[[32]byte]memory.Entry) error { return errors.New("read-only") },
		Discard: func() {},
	})
}
