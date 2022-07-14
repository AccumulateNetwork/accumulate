package main

//lint:file-ignore ST1001 Don't care

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/play-accumulate/pkg"
	"gitlab.com/accumulatenetwork/accumulate/internal/testing"
	. "gitlab.com/ethan.reesor/vscode-notebooks/go-playbooks/pkg/kernel"
	"gitlab.com/ethan.reesor/vscode-notebooks/yaegi/interp"
)

func main() {
	testing.EnableDebugFeatures()

	_ = cmd.Execute()
}

var cmd = &cobra.Command{
	Use:  "play-accumulate-kernel [socket file]",
	Args: cobra.ExactArgs(1),
	Run:  run,
}

func check(err error) {
	if err == nil {
		return
	}

	if errors.Is(err, io.EOF) {
		return
	}

	log.Println(err)
	os.Exit(1)
}

func run(_ *cobra.Command, args []string) {
	// Create the connection
	conn, err := net.Dial("unix", args[0])
	check(err)
	kernel := New(context.Background(), conn, interp.Options{})

	// Setup the Accumulate session
	session := &pkg.Session{
		Filename: "notebook.go",
		Output: func(o ...pkg.Output) {
			out := kernel.Output()
			for _, o := range o {
				switch o.Mime {
				case "text/json", "application/json":
					var buf bytes.Buffer
					if json.Indent(&buf, o.Value, "", "  ") == nil {
						out = out.Add(o.Mime, buf.Bytes())
					} else {
						out = out.Add(o.Mime, o.Value)
					}
				default:
					out = out.Add(o.Mime, o.Value)
				}
			}
			kernel.SendSafe(out)
		},
	}
	session.UseSimulator(3)
	pkg.InterpUseSession(session, kernel)

	// Handle incoming events
	rd := NewEventReader(conn)
	for {
		event, err := rd.Read()
		if err != nil {
			check(err)
			break
		}

		if !kernel.Event(event) {
			break
		}
	}

	check(kernel.Err())
}
