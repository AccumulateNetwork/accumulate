// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	f2 "github.com/FactomProject/factom"
	"github.com/FactomProject/factomd/common/entryBlock"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/factom"
)

var cmdPartition = &cobra.Command{
	Use:   "partition [count] [input directory] [output directory]",
	Short: "Partition a Factom object dump",
	Args:  cobra.ExactArgs(3),
	Run:   partition,
}

func init() {
	cmd.AddCommand(cmdPartition)
}

func partition(_ *cobra.Command, args []string) {
	count, err := strconv.ParseUint(args[0], 10, 8)
	checkf(err, "count")

	bvns := make([]string, count)
	for i := range bvns {
		bvns[i] = fmt.Sprintf("BVN%d", i)
	}

	// Create a simple routing table with the specified number of BVNs. Since
	// we're only routing LDAs there's no need for overrides.
	table := routing.BuildSimpleTable(bvns)
	router, err := routing.NewRouteTree(&protocol.RoutingTable{Routes: table})
	checkf(err, "create router")

	logger := newLogger()

	readFactomDir(args[1], func(input []byte, height int) {
		output := make(map[string]*os.File, count)
		for i, bvn := range bvns {
			filename := filepath.Join(args[2], fmt.Sprintf("objects-%d-%s.dat", height, bvn))
			output[bvns[i]], err = os.Create(filename)
			checkf(err, "create %s", filename)
		}

		// Read the object file
		err = factom.ReadObjectFile(input, logger, func(_ *factom.Header, object interface{}) {
			entry, ok := object.(*entryBlock.Entry)
			if !ok {
				return
			}

			qEntry := &f2.Entry{
				ChainID: entry.ChainID.String(),
				ExtIDs:  entry.ExternalIDs(),
				Content: entry.GetContent(),
			}

			accountId, err := hex.DecodeString(qEntry.ChainID)
			checkf(err, "decode chain ID")

			account, err := protocol.LiteDataAddress(accountId[:])
			checkf(err, "create LDA URL")

			partition, err := router.Route(account)
			checkf(err, "route LDA")

			file := output[partition]
			if file == nil {
				fatalf("missing output file for %v", partition)
			}

			data, err := entry.MarshalBinary()
			checkf(err, "marshal entry")

			header := new(factom.Header)
			header.Size = uint64(len(data))
			header.Tag = factom.TagEntry

			_, err = file.Write(header.MarshalBinary())
			checkf(err, "write header")
			_, err = file.Write(data)
			checkf(err, "write entry")
		})
		checkf(err, "process object file")
	})
}
