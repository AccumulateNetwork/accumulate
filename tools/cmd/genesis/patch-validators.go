// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdPatchValidators = &cobra.Command{
	Use:   "patch-validators [genesis] [validators]",
	Short: "Modify the validator set in-place",
	Run:   patchValidators,
	Args:  cobra.ExactArgs(2),
}

func init() {
	cmd.AddCommand(cmdPatchValidators)
}

func patchValidators(_ *cobra.Command, args []string) {
	// Load the genesis document
	genesis, err := types.GenesisDocFromFile(args[0])
	check(err)

	// Unmarshal validators
	var validators []*protocol.ValidatorInfo
	check(json.Unmarshal([]byte(args[1]), &validators))

	// Extract the partition ID from the chain ID
	partId := genesis.ChainID
	i := strings.LastIndex(partId, "-")
	if i >= 0 {
		partId = partId[i+1:]
	}
	networkUrl := protocol.PartitionUrl(partId).JoinPath(protocol.Network)
	networkAccountKey := storage.MakeKey("Account", networkUrl)

	var snapshotData []byte
	check(json.Unmarshal(genesis.AppState, &snapshotData))

	in, out := ioutil2.NewBuffer(snapshotData), new(ioutil2.Buffer)
	rd, wr := snapshot.NewReader(in), snapshot.NewWriter(out)
	for {
		// Go to the next section
		s, err := rd.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		checkf(err, "next section")

		if x := snapshot.SectionType(0); !x.SetEnumValue(s.Type().GetEnumValue()) {
			fatalf("unknown section type %v", s.Type())
		}

		if s.Type() == snapshot.SectionTypeAccounts {
			fmt.Printf("Patching %s section\n", s.Type())
		} else {
			fmt.Printf("Copying %s section\n", s.Type())
		}

		// Open the section for reading
		sr, err := s.Open()
		checkf(err, "read section")

		// Open the section for writing
		sw, err := wr.Open(s.Type())
		checkf(err, "write section")

		// If it's not an account section, copy it verbatim
		if s.Type() != snapshot.SectionTypeAccounts {
			_, err = io.Copy(sw, sr)
			checkf(err, "copy section")

			check(sw.Close())
			continue
		}

		// Read accounts
		memdb := memory.NewDB()
		membatch := memdb.Begin(true)
		bpt := pmt.NewBPTManager(membatch)
		accounts := map[storage.Key][]byte{}
		check(pmt.ReadSnapshot(sr, func(key storage.Key, hash [32]byte, reader ioutil2.SectionReader) error {
			if key != networkAccountKey {
				bpt.InsertKV(key, hash)
				b, err := io.ReadAll(reader)
				check(err)
				accounts[key] = b
				return nil
			}

			account := new(snapshot.Account)
			check(account.UnmarshalBinaryFrom(reader))
			if !account.Url.Equal(networkUrl) {
				fatalf("entry for %v does not match (expected %v, got %v)", networkUrl, networkUrl, account.Url)
			}

			data, ok := account.Main.(*protocol.DataAccount)
			if !ok {
				fatalf("entry for %v does not match (expected %v, got %v)", networkUrl, protocol.AccountTypeDataAccount, account.Main.Type())
			}
			if data.Entry == nil {
				fatalf("entry for %v does not have a data entry", networkUrl)
			}
			entry, ok := data.Entry.(*protocol.AccumulateDataEntry)
			if !ok {
				fatalf("entry for %v does not match (expected %v, got %v)", networkUrl, protocol.DataEntryTypeAccumulate, data.Entry.Type())
			}
			if len(entry.Data) != 1 {
				fatalf("entry for %v has an invalid data entry", networkUrl)
			}

			netdef := new(protocol.NetworkDefinition)
			checkf(netdef.UnmarshalBinary(entry.Data[0]), "unmarshal network definition")
			b, _ := json.Marshal(netdef.Validators)
			fmt.Printf("Validator set was: %s\n", b)

			netdef.Validators = validators
			entry.Data[0], err = netdef.MarshalBinary()
			check(err)
			b, _ = json.Marshal(netdef.Validators)
			fmt.Printf("Validator set is now: %s\n", b)

			account.Hash

			bpt.InsertKV(key, hash)

			b, err = account.MarshalBinary()
			b, err := io.ReadAll(reader)
			check(err)
			accounts[key] = b
			check(err)

			return nil
		}))
		check(bpt.Bpt.Update())

		if accounts[networkAccountKey] == nil {
			fatalf("%v is not present in this genesis document", networkUrl)
		}

		// Write accounts
		var didPatch bool
		check(bpt.Bpt.SaveSnapshot(sw, func(key storage.Key, hash [32]byte) ([]byte, error) {
			b, ok := accounts[key]
			if !ok {
				// Should not be possible
				fatalf("missing data for account %x", key)
			}

			if key != networkAccountKey {
				return b, nil
			}

			didPatch = true
			account := new(snapshot.Account)
			check(account.UnmarshalBinaryFrom(bytes.NewReader(b)))
			if !account.Url.Equal(networkUrl) {
				fatalf("entry for %v does not match (expected %v, got %v)", networkUrl, networkUrl, account.Url)
			}

			data, ok := account.Main.(*protocol.DataAccount)
			if !ok {
				fatalf("entry for %v does not match (expected %v, got %v)", networkUrl, protocol.AccountTypeDataAccount, account.Main.Type())
			}
			if data.Entry == nil {
				fatalf("entry for %v does not have a data entry", networkUrl)
			}
			entry, ok := data.Entry.(*protocol.AccumulateDataEntry)
			if !ok {
				fatalf("entry for %v does not match (expected %v, got %v)", networkUrl, protocol.DataEntryTypeAccumulate, data.Entry.Type())
			}
			if len(entry.Data) != 1 {
				fatalf("entry for %v has an invalid data entry", networkUrl)
			}

			netdef := new(protocol.NetworkDefinition)
			checkf(netdef.UnmarshalBinary(entry.Data[0]), "unmarshal network definition")
			b, _ = json.Marshal(netdef.Validators)
			fmt.Printf("Validator set was: %s\n", b)

			netdef.Validators = validators
			entry.Data[0], err = netdef.MarshalBinary()
			check(err)
			b, _ = json.Marshal(netdef.Validators)
			fmt.Printf("Validator set is now: %s\n", b)

			b, err = account.MarshalBinary()
			check(err)
			return b, nil
		}))
		if !didPatch {
			fatalf("did not patch %v", networkUrl)
		}

		check(sw.Close())
	}

	fmt.Printf("Overwrite %s? ", args[0])
	_, err = bufio.NewReader(os.Stdin).ReadBytes('\n')
	check(err)

	genesis.AppState, err = json.Marshal(out.Bytes())
	check(err)
	check(genesis.SaveAs(args[0]))
}

func recalculateHash(a *snapshot.Account) {
	var err error
	var hasher hash.Hasher
	hashState(&err, &hasher, true, a.Main)          // Add a simple hash of the main state
	hashState(&err, &hasher, false, hashSecondaryState(a)) // Add a merkle hash of the Secondary State which is a list of accounts contained by the adi
	hashState(&err, &hasher, false, a.hashChains)         // Add a merkle hash of chains
	hashState(&err, &hasher, false, a.hashTransactions)   // Add a merkle hash of transactions
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

// hashChains returns a merkle hash of the DAG root of every chain in alphabetic
// order.
func hashChains(a *snapshot.Account) (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	for _, chain := range a.Chains {
		if chain.Head.Count == 0 {
			hasher.AddHash(new([32]byte))
		} else {
			hasher.AddHash((*[32]byte)(chain.Head.GetMDRoot()))
		}
	}
	return hasher, err
}

// hashTransactions returns a merkle hash of the transaction hash and status of
// every pending transaction and every synthetic transaction waiting for an
// anchor.
func hashTransactions(a *snapshot.Account) (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	for _, txid := range a.Pending {
		h := txid.Hash()
		hashState(&err, &hasher, false, a.parent.Transaction(h[:]).GetState)
		hashState(&err, &hasher, false, a.parent.Transaction(h[:]).GetStatus)
	}

	// // TODO Include this
	// for _, anchor := range loadState(&err, false, a.SyntheticAnchors().Get) {
	// 	hasher.AddHash(&anchor) //nolint:rangevarref
	// 	for _, txid := range loadState(&err, false, a.SyntheticForAnchor(anchor).Get) {
	// 		h := txid.Hash()
	// 		hashState(&err, &hasher, false, a.batch.Transaction(h[:]).GetState)
	// 		hashState(&err, &hasher, false, a.batch.Transaction(h[:]).GetStatus)
	// 	}
	// }

	return hasher, err
}

func zero[T any]() (z T) { return z }

func hashState[T any](lastErr *error, hasher *hash.Hasher, allowMissing bool, v T) {
	if *lastErr != nil {
		return
	}

	if allowMissing && any(zero[T]()) == any(v) {
		hasher.AddHash(new([32]byte))
		return
	}

	switch v := interface{}(v).(type) {
	case interface{ MerkleHash() []byte }:
		hasher.AddValue(v)
	case interface{ GetHash() []byte }:
		hasher.AddHash((*[32]byte)(v.GetHash()))
	case encoding.BinaryValue:
		data, err := v.MarshalBinary()
		if err != nil {
			*lastErr = err
			return
		}
		hasher.AddBytes(data)
	default:
		panic(fmt.Errorf("unhashable type %T", v))
	}
}
