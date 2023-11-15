// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/chzyer/readline"
	"github.com/mattn/go-shellwords"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func init() {
	cmdDb.AddCommand(cmdDbExplore)
}

var cmdDbExplore = &cobra.Command{
	Use:   "explore [flags]",
	Short: "Explore a database",
	Run:   explore,
}

var flagExplore = struct {
	Node     string
	Badger   string
	Snapshot string
}{}

func init() {
	cmdDbExplore.Flags().StringVar(&flagExplore.Node, "node", "", "Explore a node's database")
	cmdDbExplore.Flags().StringVar(&flagExplore.Badger, "badger", "", "Explore a Badger database")
	cmdDbExplore.Flags().StringVar(&flagExplore.Snapshot, "snapshot", "", "Explore a snapshot (must be v2, must be indexed)")
	_ = cmdDbExplore.MarkFlagDirname("node")
	_ = cmdDbExplore.MarkFlagDirname("badger")
	_ = cmdDbExplore.MarkFlagFilename("snapshot")
	cmdDbExplore.MarkFlagsMutuallyExclusive("node", "badger", "snapshot")
	cmdDbExplore.MarkFlagsRequiredTogether()
}

func explore(_ *cobra.Command, args []string) {
	var err error
	switch {
	case flagExplore.Node != "":
		daemon, err := accumulated.Load(flagExplore.Node, nil)
		check(err)
		Db, err = database.Open(daemon.Config, nil)
		check(err)
		defer safeClose(Db)

	case flagExplore.Badger != "":
		Db, err = database.OpenBadger(flagExplore.Badger, nil)
		check(err)
		defer safeClose(Db)

	case flagExplore.Snapshot != "":
		f, err := os.Open(flagExplore.Snapshot)
		check(err)
		defer safeClose(f)
		rd, err := snapshot.Open(f)
		check(err)
		store, err := rd.AsStore()
		check(err)
		Db = database.New(memory.NewChangeSet(nil, func(key *record.Key) ([]byte, error) {
			return store.Get(key)
		}, nil, nil), nil)

	default:
		fatalf("no database specified")
	}

	// Run in immediate (non-repl) mode
	if len(args) > 0 {
		repl.SilenceErrors = false
		repl.SilenceUsage = false
		repl.SetArgs(args)
		_ = repl.Execute()
		return
	}

	var completer = readline.NewPrefixCompleter(makePcItem(repl).Children...)

	l, err := readline.NewEx(&readline.Config{
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
		Prompt:            "\033[31m»\033[0m ",
		HistoryFile:       "/tmp/acc-explore-history.log",
		AutoComplete:      completer,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
	})
	check(err)
	defer l.Clean()

	l.CaptureExitSignal()
	log.SetOutput(l.Stderr())

	for {
		line, err := l.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				break
			} else {
				continue
			}
		} else if err == io.EOF {
			break
		}

		line = strings.TrimSpace(line)
		switch {
		case line == "help":
			w := l.Stderr()
			_, _ = io.WriteString(w, "commands:\n")
			_, _ = io.WriteString(w, completer.Tree("    "))
			continue

		case line == "exit":
			return

		case line == "":
			continue
		}

		args, err := shellwords.Parse(line)
		if err != nil {
			fmt.Fprintf(l.Stderr(), "Error: %v\n", err)
			continue
		}

		cmd, a, err := repl.Find(args)
		if err != nil {
			if cmd == repl && strings.HasPrefix(err.Error(), "unknown command") {
				fmt.Fprintf(l.Stderr(), "Error: unknown command '%s'\n", args[0])
			} else {
				fmt.Fprintf(l.Stderr(), "Error: %v\n", err)
			}
			continue
		}

		if len(a) > 0 && cmd.Run == nil && cmd.RunE == nil {
			fmt.Fprintf(l.Stderr(), "Error: unknown command '%s' for '%s'\n", a[0], fullName(cmd))
			continue
		}

		repl.SetOut(l.Stdout())
		repl.SetErr(l.Stderr())
		repl.SetArgs(args)
		err = repl.Execute()
		if err != nil {
			fmt.Fprintf(l.Stderr(), "Error: %v\n", err)
			continue
		}
	}
}

func fullName(cmd *cobra.Command) string {
	if !cmd.HasParent() {
		return cmd.Name()
	}
	if cmd.Parent().Use == "" {
		return fullName(cmd.Parent()) + cmd.Name()
	}
	return fullName(cmd.Parent()) + " " + cmd.Name()
}

func newCmd(cmd *cobra.Command, sub ...*cobra.Command) *cobra.Command {
	cmd.AddCommand(sub...)
	return cmd
}

func makePcItem(cmd *cobra.Command) *readline.PrefixCompleter {
	pc := readline.PcItem(strings.SplitN(cmd.Use, " ", 2)[0])
	for _, cmd := range cmd.Commands() {
		pc.Children = append(pc.Children, makePcItem(cmd))
	}
	return pc
}

func printValue(cmd *cobra.Command, v interface{}) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", b)
	if err != nil {
		return err
	}
	return nil
}

type Getter[T any] interface {
	Get() (T, error)
}

func getAndPrintValue[T any](cmd *cobra.Command, v Getter[T]) error {
	u, err := v.Get()
	if err != nil {
		return err
	}
	return printValue(cmd, u)
}

type fakeChangeSet struct{ keyvalue.Store }

func (fakeChangeSet) Begin(*record.Key, bool) keyvalue.ChangeSet { panic("shim") }
func (fakeChangeSet) Commit() error                              { panic("shim") }
func (fakeChangeSet) Discard()                                   {}
