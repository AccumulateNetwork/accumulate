package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"os"

	"github.com/spf13/cobra"
	"github.com/traefik/yaegi/interp"
	playcmd "gitlab.com/accumulatenetwork/accumulate/cmd/play-accumulate/cmd"
	"gitlab.com/accumulatenetwork/accumulate/cmd/play-accumulate/pkg"
	. "gitlab.com/ethan.reesor/vscode-notebooks/go-playbooks/pkg/kernel"
)

func main() {
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
		Stdout:   kernel.Stdout(),
		Stderr:   kernel.Stderr(),
	}
	session.UseSimulator(3)
	playcmd.InterpUseSession(session, kernel)

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
