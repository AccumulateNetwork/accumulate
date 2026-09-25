// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Command token-registry seeds the explorer's token registry offline, from
// partition databases rather than the API: it walks every account in each
// database, keeps the token issuers, and records the block each partition is
// current as of, so an online follower can take over from exactly there
// (explorer#88).
//
// Databases are opened read-only. Point it at a stopped follower, or a copy.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strings"
	"time"

	cometLog "github.com/cometbft/cometbft/libs/log"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	kvleveldb "gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var flagOut = flag.String("out", "", "write the registry here (default stdout)")
var flagNetwork = flag.String("network", "", "network name recorded in the registry, e.g. mainnet")

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: token-registry -network <name> [-out <file>] <partition>=<leveldb dir> ...\n")
	fmt.Fprintf(os.Stderr, "  e.g. token-registry -network mainnet Cyclops=bvnn/data/accumulate.db Directory=dnn/data/accumulate.db\n")
	flag.PrintDefaults()
}

// Registry is the seed the online follower loads.
type Registry struct {
	Network    string       `json:"network"`
	Generated  time.Time    `json:"generated"`
	Partitions []*Partition `json:"partitions"`
	Tokens     []*Token     `json:"tokens"`
}

// Partition records the block a partition's database is current as of. The
// online follower starts at Height+1.
type Partition struct {
	ID       string    `json:"id"`
	Height   uint64    `json:"height"`
	Time     time.Time `json:"time"`
	Accounts int       `json:"accounts"`
	Issuers  int       `json:"issuers"`
	NoState  int       `json:"noState,omitempty"` // in the BPT, but with no main state
}

type Token struct {
	URL         string `json:"url"`
	Symbol      string `json:"symbol"`
	Precision   uint64 `json:"precision"`
	Issued      string `json:"issued"`                // base units
	Supply      string `json:"supply"`                // Issued in whole tokens
	SupplyLimit string `json:"supplyLimit,omitempty"` // base units
	Properties  string `json:"properties,omitempty"`
	Partition   string `json:"partition"`

	issued *big.Int
}

func main() {
	flag.Usage = usage
	flag.Parse()
	if *flagNetwork == "" || flag.NArg() == 0 {
		usage()
		os.Exit(2)
	}

	reg := &Registry{Network: *flagNetwork, Generated: time.Now().UTC()}
	for _, arg := range flag.Args() {
		id, path, ok := strings.Cut(arg, "=")
		if !ok {
			fatalf("%q: want <partition>=<leveldb dir>", arg)
		}
		part, tokens, err := scan(id, path)
		if err != nil {
			fatalf("%s: %v", id, err)
		}
		fmt.Fprintf(os.Stderr, "%s: height %d (%v), %d accounts (%d with no state), %d token issuers\n",
			id, part.Height, part.Time.Format(time.RFC3339), part.Accounts, part.NoState, part.Issuers)
		reg.Partitions = append(reg.Partitions, part)
		reg.Tokens = append(reg.Tokens, tokens...)
	}

	// Greatest supply first. Compare whole-token supply exactly: Issued
	// exceeds 2^53, and precisions differ between tokens.
	sort.SliceStable(reg.Tokens, func(i, j int) bool {
		a, b := reg.Tokens[i], reg.Tokens[j]
		if c := compareSupply(a, b); c != 0 {
			return c > 0
		}
		return a.URL < b.URL
	})

	out := os.Stdout
	if *flagOut != "" {
		f, err := os.Create(*flagOut)
		if err != nil {
			fatalf("%v", err)
		}
		defer f.Close()
		out = f
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(reg); err != nil {
		fatalf("write: %v", err)
	}
}

func scan(id, path string) (*Partition, []*Token, error) {
	// Read-only: never write to a follower's database. goleveldb still takes
	// the directory lock, so this also keeps the node from starting on it
	// mid-scan.
	ldb, err := leveldb.OpenFile(path, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}
	db := coredb.New(kvleveldb.New(ldb), cometLog.NewNopLogger())
	defer db.Close()

	batch := db.Begin(false)
	defer batch.Discard()

	part := &Partition{ID: id}
	var ledger *protocol.SystemLedger
	err = batch.Account(protocol.PartitionUrl(id).JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	if err != nil {
		return nil, nil, fmt.Errorf("load %s system ledger (is this the %s database?): %w", id, id, err)
	}
	part.Height, part.Time = ledger.Index, ledger.Timestamp

	var tokens []*Token
	err = batch.ForEachAccount(func(account *coredb.Account, _ [32]byte) error {
		part.Accounts++
		if part.Accounts%50000 == 0 {
			fmt.Fprintf(os.Stderr, "%s: %d accounts...\n", id, part.Accounts)
		}
		state, err := account.Main().Get()
		switch {
		case errors.Is(err, errors.NotFound):
			// Some accounts are in the BPT with no main state. None of
			// them is a token issuer we could report; count and move on.
			part.NoState++
			return nil
		case err != nil:
			return fmt.Errorf("load %v: %w", account.Url(), err)
		}
		issuer, ok := state.(*protocol.TokenIssuer)
		if !ok {
			return nil
		}
		tokens = append(tokens, newToken(id, issuer))
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	part.Issuers = len(tokens)
	return part, tokens, nil
}

func newToken(partition string, iss *protocol.TokenIssuer) *Token {
	t := &Token{
		URL:       iss.Url.String(),
		Symbol:    iss.Symbol,
		Precision: iss.Precision,
		Partition: partition,
		issued:    new(big.Int).Set(&iss.Issued),
	}
	t.Issued = t.issued.String()
	t.Supply = decimal(t.issued, iss.Precision)
	if iss.SupplyLimit != nil {
		t.SupplyLimit = iss.SupplyLimit.String()
	}
	if iss.Properties != nil {
		t.Properties = iss.Properties.String()
	}
	return t
}

// compareSupply compares a.Issued/10^a.Precision with b's, exactly.
func compareSupply(a, b *Token) int {
	x := new(big.Int).Mul(a.issued, pow10(b.Precision))
	y := new(big.Int).Mul(b.issued, pow10(a.Precision))
	return x.Cmp(y)
}

func pow10(n uint64) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), new(big.Int).SetUint64(n), nil)
}

// decimal renders v/10^precision without rounding.
func decimal(v *big.Int, precision uint64) string {
	s := new(big.Int).Abs(v).String()
	if precision > 0 {
		if n := int(precision); len(s) <= n {
			s = strings.Repeat("0", n-len(s)+1) + s
		}
		i := len(s) - int(precision)
		s = strings.TrimRight(s[:i]+"."+s[i:], "0")
		s = strings.TrimSuffix(s, ".")
	}
	if v.Sign() < 0 {
		s = "-" + s
	}
	return s
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
