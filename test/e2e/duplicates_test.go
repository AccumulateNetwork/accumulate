// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
)

// Every chain append in every test of this package is watched for a hash
// the chain already holds (database spec, "Duplicates are caught at
// entry").  A duplicate is caught where it enters -- the replay check on
// a message's status, a key entry's spent timestamps, staging's delivered
// index -- so by the time a hash reaches a chain it is new, and the only
// duplicates a chain may see are the permitted repeats:
//
//   - root chains: equal chains have equal anchors (genesis, one
//     transaction creating several accounts);
//   - signature chains: one cause per signer, and v1's signature path;
//   - main and scratch chains: the executor appends the transaction hash
//     from two sites and the second is rejected (DIFFERENCES E9).  This
//     entry goes when the append has one site.
//
// Anything else -- an index chain, the synthetic chain or a replica, the
// anchor sequence, block ledger or BPT chain, an anchor root or BPT
// chain -- fails the package: a duplicate there is the writer appending
// twice, not the chain's business to absorb.
var duplicates = struct {
	sync.Mutex
	seen map[string]int
}{seen: map[string]int{}}

func recordDuplicate(chain *record.Key, unique bool) {
	name := chain.String()
	// Reduce the key to the account's last path element and the chain,
	// so the report reads "tokens.MainChain unique=true"
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	duplicates.Lock()
	duplicates.seen[fmt.Sprintf("%s unique=%v", name, unique)]++
	duplicates.Unlock()
}

func permittedDuplicate(entry string) bool {
	chain := strings.SplitN(entry, " ", 2)[0]
	switch {
	case strings.HasSuffix(chain, ".RootChain"),
		strings.HasSuffix(chain, ".SignatureChain"),
		strings.HasSuffix(chain, ".MainChain"),
		strings.HasSuffix(chain, ".ScratchChain"):
		return true
	}
	return false
}

func TestMain(m *testing.M) {
	merkle.OnDuplicate = recordDuplicate
	code := m.Run()

	duplicates.Lock()
	defer duplicates.Unlock()
	if os.Getenv("ACC_DUPLICATES_REPORT") != "" {
		var all []string
		for entry, n := range duplicates.seen {
			all = append(all, fmt.Sprintf("  %6d  %s", n, entry))
		}
		sort.Strings(all)
		fmt.Fprintf(os.Stderr, "\nduplicate chain appends seen by the suite:\n%s\n", strings.Join(all, "\n"))
	}
	var bad []string
	for entry, n := range duplicates.seen {
		if !permittedDuplicate(entry) {
			bad = append(bad, fmt.Sprintf("  %6d  %s", n, entry))
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		fmt.Fprintf(os.Stderr, "\nFAIL: %d kind(s) of duplicate chain append outside the permitted repeats (database spec, \"Duplicates are caught at entry\"):\n%s\n", len(bad), strings.Join(bad, "\n"))
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}
