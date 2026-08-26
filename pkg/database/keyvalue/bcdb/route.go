// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// BlockchainDB has two layers and the caller is meant to say which one
// a record belongs to.  The permanent layer is append-only: its sealed
// segments partition the key space, which is what lets a peer sync by
// copying files rather than replaying records.  The dynamic layer
// allows overwrites and is compacted to reclaim what they orphan.
//
// KV2.Put discovers the layer instead -- look in Dyna, look in Perm,
// and migrate the key if Perm holds it with a different value.  That
// puts every mutable record in the permanent layer on its first write
// and only notices on the second, so a measurement of "how much
// permanent data gets rewritten" measures the adapter rather than
// Accumulate.  isWriteOnce is what replaces that guess.
//
// # What decides it
//
// Mutability is a property of the record, which is to say of the last
// name in its key path -- not of whether a hash appears in the key.
// Transaction(Hash).Main and Transaction(Hash).Status share the hash
// and disagree: the transaction is what the hash is of, so it cannot
// change, while its status is the record of what has happened to it
// since.
//
// The rules below are therefore written as suffixes of the path, never
// as its whole shape.  A key reaches the store already prefixed by
// whatever the batch was opened with, and the same chain records appear
// under a dozen different chains; matching the tail is what makes one
// rule cover all of them.
//
// # Getting it wrong
//
// The two directions are not symmetric.  Calling a mutable record
// write-once is caught: the permanent layer refuses to overwrite, so
// the first time the record changes the store says so.  Calling a
// write-once record mutable is silent -- it costs space and denies that
// data the file-copy sync path, and nothing complains.
//
// So the store's refusal is the test, and this package treats it as a
// finding rather than a failure: a conflict is counted against the
// key's shape, the write goes to the dynamic layer, and the shape
// names itself in the report.  See Database.putRouted.
//
// # The classification
//
//	Write-once                        Mutable
//	----------                        -------
//	Message(H).Main                   Account(U).Main
//	Transaction(H).Main               Transaction(H).Status
//	Account(U).Url                    Account(U).Pending, .Directory, .Chains
//	<chain>.Element(I)                <chain>.Head
//	<chain>.ElementIndex(H)           BPT nodes and BPT.Root
//	<chain>.States(I)                 every set and counted collection
//	Account(U).Data.Entry(I)          Account(U).Data.Entry (the count)
//	Account(U).Data.Transaction(H)    Events, BlockLedger, Log blocks
//	SystemData(P).SyntheticIndexIndex(B)
//
// Anything not named here is treated as mutable, which is the direction
// that fails quietly rather than loudly.
func isWriteOnce(k *record.Key) bool {
	last, prev, trailing := tail(k)
	switch last {
	case "Element", "ElementIndex", "States":
		// A merkle chain is a log.  Element(I) is the I'th entry,
		// ElementIndex(H) is where entry H landed, and States(I) is
		// the mark point covering I -- all of them facts about a
		// position in the log, which does not move.  Head is the log's
		// current end and is excluded by requiring the parameter.
		return trailing == 1

	case "Main":
		// A message and a transaction are named by the hash of their
		// own content; an account is named by its URL and its main
		// state is what changes.
		return trailing == 0 && (prev == "Message" || prev == "Transaction")

	case "Url":
		// An account's URL is the URL the account is keyed by.
		return trailing == 0 && prev == "Account"

	case "Entry", "Transaction":
		// Data.Entry is a counted collection: Entry(I) is the I'th
		// entry of a data account and Entry alone is the count, which
		// changes with every entry.  Data.Transaction(H) records which
		// transaction wrote entry H.
		return trailing == 1 && prev == "Data"

	case "SyntheticIndexIndex":
		// Which synthetic index chain entry covers a given block.
		return trailing == 1
	}

	return false
}

// tail reports the last string element of k, the string element before
// it, and how many elements follow the last string.
//
// A record key alternates names and parameters -- ("Account", url,
// "MainChain", "Element", 5) -- so the last name is the record and what
// follows it is that record's parameters.  Walking back from the end
// finds both without allocating, which matters because this is on the
// write path.
func tail(k *record.Key) (last, prev string, trailing int) {
	if k == nil {
		return "", "", 0
	}
	i := k.Len() - 1
	for ; i >= 0; i-- {
		if s, ok := k.Get(i).(string); ok {
			last = s
			break
		}
		trailing++
	}
	for i--; i >= 0; i-- {
		if s, ok := k.Get(i).(string); ok {
			prev = s
			break
		}
	}
	return last, prev, trailing
}
