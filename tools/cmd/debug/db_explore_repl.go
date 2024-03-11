// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var repl = newCmd(
	&cobra.Command{
		SilenceErrors: true,
		SilenceUsage:  true,
	},
	newCmd(
		&cobra.Command{
			Use:  "account",
			Args: cobra.ExactArgs(1),
			RunE: acctPrintVal[protocol.Account]((*database.Account).Main),
		},
		newCmd(&cobra.Command{
			Use:  "pending",
			Args: cobra.ExactArgs(1),
			RunE: acctPrintVal[[]*url.TxID]((*database.Account).Pending),
		}),
		newCmd(&cobra.Command{
			Use:  "directory",
			Args: cobra.ExactArgs(1),
			RunE: acctPrintVal[[]*url.URL]((*database.Account).Directory),
		}),
		newCmd(&cobra.Command{
			Use:  "chain",
			Args: cobra.RangeArgs(1, 4),
			RunE: acct(acctChain),
		}),
	),
	newCmd(
		&cobra.Command{
			Use:  "transaction",
			Args: cobra.ExactArgs(1),
			RunE: txn(txnMain),
		},
		newCmd(&cobra.Command{
			Use:  "status",
			Args: cobra.ExactArgs(1),
			RunE: txnPrintVal[*protocol.TransactionStatus]((*database.Transaction).Status),
		}),
		newCmd(&cobra.Command{
			Use:  "produced",
			Args: cobra.ExactArgs(1),
			RunE: txnPrintVal[[]*url.TxID]((*database.Transaction).Produced),
		}),
		newCmd(&cobra.Command{
			Use:  "signatures",
			Args: cobra.ExactArgs(2),
			RunE: txn(txnSignatures),
		}),
	),
	newCmd(
		&cobra.Command{
			Use: "bpt",
		},
		newCmd(&cobra.Command{
			Use:  "root",
			Args: cobra.ExactArgs(0),
			RunE: db(func(cmd *cobra.Command, batch *database.Batch, args []string) error {
				hash, err := batch.GetBptRootHash()
				if err != nil {
					return err
				}
				fmt.Printf("%x\n", hash)
				return nil
			}),
		}),
	),
)

var Db *database.Database

func db(run func(cmd *cobra.Command, batch *database.Batch, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		batch := Db.Begin(false)
		defer batch.Discard()
		return run(cmd, batch, args)
	}
}

func acct(run func(*cobra.Command, *database.Account, []string) error) func(*cobra.Command, []string) error {
	return db(func(cmd *cobra.Command, batch *database.Batch, args []string) error {
		u, err := url.Parse(args[0])
		if err != nil {
			return err
		}
		return run(cmd, batch.Account(u), args[1:])
	})
}

func acctPrintVal[T1 any, T2 Getter[T1]](get func(*database.Account) T2) func(*cobra.Command, []string) error {
	return acct(func(cmd *cobra.Command, acct *database.Account, _ []string) error {
		return getAndPrintValue[T1](cmd, get(acct))
	})
}

func acctChain(cmd *cobra.Command, acct *database.Account, args []string) error {
	var chain *database.Chain
	var start, count uint64
	var err error

	if len(args) > 0 {
		chain, err = acct.GetChainByName(args[0])
		if err != nil {
			return err
		}
	}

	if len(args) > 1 {
		start, err = strconv.ParseUint(args[1], 10, 64)
		if err != nil {
			return err
		}
	}

	if len(args) > 2 {
		count, err = strconv.ParseUint(args[2], 10, 64)
		if err != nil {
			return err
		}
	} else {
		count = 10
	}

	switch len(args) {
	case 0:
		return getAndPrintValue[[]*protocol.ChainMetadata](cmd, acct.Chains())

	case 1:
		return printValue(cmd, chain.CurrentState())

	case 2, 3:
		if start >= uint64(chain.Height()) {
			return printValue(cmd, []any{})
		}
		if x := uint64(chain.Height()) - start; count > x {
			count = x
		}
		entries, err := chain.Entries(int64(start), int64(start+count))
		if err != nil {
			return err
		}
		var s []string
		for _, e := range entries {
			s = append(s, hex.EncodeToString(e))
		}
		return printValue(cmd, s)

	default:
		panic("")
	}
}

func txn(run func(*cobra.Command, *database.Transaction, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		batch := Db.Begin(false)
		defer batch.Discard()

		txid, err := url.ParseTxID(args[0])
		if err == nil {
			hash := txid.Hash()
			return run(cmd, batch.Transaction(hash[:]), args[1:])
		}

		hash, err := hex.DecodeString(args[0])
		if err == nil {
			return run(cmd, batch.Transaction(hash), args[1:])
		}

		return fmt.Errorf("`%s` is not a transaction ID or hash", args[0])
	}
}

func txnPrintVal[T1 any, T2 Getter[T1]](get func(*database.Transaction) T2) func(*cobra.Command, []string) error {
	return txn(func(cmd *cobra.Command, txn *database.Transaction, _ []string) error {
		return getAndPrintValue[T1](cmd, get(txn))
	})
}

func txnMain(cmd *cobra.Command, txn *database.Transaction, _ []string) error {
	v, err := txn.Main().Get()
	if err != nil {
		return err
	}
	switch {
	case v.Transaction != nil:
		return printValue(cmd, v.Transaction)
	case v.Signature != nil:
		return printValue(cmd, v.Signature)
	default:
		fmt.Fprintln(cmd.OutOrStdout(), "State is empty (no transaction or signature")
		return nil
	}
}

func txnSignatures(cmd *cobra.Command, txn *database.Transaction, args []string) error {
	u, err := url.Parse(args[0])
	if err != nil {
		return err
	}
	sigs, err := txn.ReadSignatures(u)
	if err != nil {
		return err
	}
	return printValue(cmd, sigs.Entries())
}
