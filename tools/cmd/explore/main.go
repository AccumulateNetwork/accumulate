// Copyright 2022 The Accumulate Authors
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
)

func main() {
	_ = cmd.Execute()
}

var cmd = &cobra.Command{
	Use:   "explore [flags]",
	Short: "Explore a database",
	Run:   explore,
	Args:  cobra.NoArgs,
}

var flag = struct {
	Node   string
	Badger string
}{}

func init() {
	cmd.Flags().StringVar(&flag.Node, "node", "", "Explore a node's database")
	cmd.Flags().StringVar(&flag.Badger, "badger", "", "Explore a Badger database")
	_ = cmd.MarkFlagDirname("node")
	_ = cmd.MarkFlagDirname("badger")
	cmd.MarkFlagsMutuallyExclusive("node", "badger")
	cmd.MarkFlagsRequiredTogether()
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func explore(*cobra.Command, []string) {
	var err error
	switch {
	case flag.Node != "":
		daemon, err := accumulated.Load(flag.Node, nil)
		check(err)
		Db, err = database.Open(daemon.Config, nil)
		check(err)

	case flag.Badger != "":
		Db, err = database.OpenBadger(flag.Badger, nil)
		check(err)

	default:
		fatalf("no database specified")
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
