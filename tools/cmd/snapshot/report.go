// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/csv"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var reportCmd = &cobra.Command{
	Use:   "report <snapshot>",
	Short: "Report the contents of a snapshot",
	Args:  cobra.ExactArgs(1),
	Run:   reportSnapshot,
}

var reportFlag = struct {
	Index bool
}{}

func init() {
	cmd.AddCommand(reportCmd)
	reportCmd.Flags().BoolVar(&reportFlag.Index, "index", false, "Include index chains")
}

func reportSnapshot(_ *cobra.Command, args []string) {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()
	check(w.Write([]string{"Account", "Account Type", "Chain", "Chain Type", "Count"}))

	f, err := os.Open(args[0])
	check(err)
	defer f.Close()

	check(snapshot.Visit(f, reportVisitor{w}))
}

type reportVisitor struct {
	*csv.Writer
}

func (w reportVisitor) VisitAccount(a *snapshot.Account, _ int) error {
	// End of section
	if a == nil {
		return nil
	}

	// Backwards compatibility
	a.ConvertOldChains(8)

	// Get the account type
	typ := protocol.AccountTypeUnknown
	if a.Main != nil {
		typ = a.Main.Type()
	}

	// Print all the chains
	for _, c := range a.Chains {
		if !reportFlag.Index && c.Type == merkle.ChainTypeIndex {
			continue
		}
		check(w.Write([]string{
			a.Url.ShortString(),
			typ.String(),
			c.Name,
			c.Type.String(),
			strconv.FormatInt(c.Head.Count, 10),
		}))
	}
	return nil
}
