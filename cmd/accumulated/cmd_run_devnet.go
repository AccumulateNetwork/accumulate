package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/AccumulateNetwork/accumulate/internal/accumulated"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/fatih/color"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/libs/log"
)

var cmdRunDevnet = &cobra.Command{
	Use:   "devnet",
	Short: "Run all nodes of a local devnet",
	Run:   runDevNet,
	Args:  cobra.NoArgs,
}

func init() {
	cmdRun.AddCommand(cmdRunDevnet)
}

var colors = []*color.Color{
	color.New(color.FgRed),
	color.New(color.FgGreen),
	color.New(color.FgYellow),
	color.New(color.FgBlue),
	color.New(color.FgMagenta),
	color.New(color.FgCyan),
}

var fallbackColor = color.New(color.FgHiBlack)

func runDevNet(*cobra.Command, []string) {
	type Node struct {
		Subnet string
		Number int
	}
	var nodes []Node

	dir := flagMain.WorkDir
	for _, node := range getNodesFromSubnetDir(filepath.Join(dir, "dn")) {
		nodes = append(nodes, Node{"dn", node})
		id := fmt.Sprintf("dn.%d", node)
		if len(id) > nodeIdLen {
			nodeIdLen = len(id)
		}
	}

	ent, err := os.ReadDir(dir)
	checkf(err, "failed to read %q", dir)

	for _, ent := range ent {
		if !ent.IsDir() || ent.Name() == "dn" {
			continue
		}

		subnet := ent.Name()
		for _, node := range getNodesFromSubnetDir(filepath.Join(dir, subnet)) {
			nodes = append(nodes, Node{subnet, node})
			id := fmt.Sprintf("%s.%d", subnet, node)
			if len(id) > nodeIdLen {
				nodeIdLen = len(id)
			}
		}
	}

	stop := make(chan struct{})
	done := new(sync.WaitGroup)

	for _, node := range nodes {
		// Load the node
		dir := filepath.Join(dir, node.Subnet, fmt.Sprintf("Node%d", node.Number))
		daemon, err := accumulated.Load(dir, func(format string) (io.Writer, error) {
			return newNodeWriter(format, node.Subnet, node.Number)
		})
		check(err)

		// Prometheus cannot be enabled when running multiple nodes in one process
		daemon.Config.Instrumentation.Prometheus = false

		// Start it
		check(daemon.Start())

		// On signal, stop the node
		done.Add(1)
		go func() {
			defer done.Done()
			<-stop
			check(daemon.Stop())
		}()
	}

	color.HiBlack("----- Started -----")

	// Wait for SIGINT
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	<-sigs

	// Overwrite the ^C
	print("\r")
	color.HiBlack("----- Stopping -----")

	// Turn of signal handling, so that another SIGINT will exit immediately
	signal.Stop(sigs)

	// Signal everyone to stop
	close(stop)

	// Wait for everyone to stop
	done.Wait()
}

func getNodesFromSubnetDir(dir string) []int {
	var nodes []int

	ent, err := os.ReadDir(dir)
	checkf(err, "failed to read %q", dir)

	for _, ent := range ent {
		// We only want directories starting with Node
		if !ent.IsDir() || !strings.HasPrefix(ent.Name(), "Node") {
			continue
		}

		// We only want directories named Node#, e.g. Node0
		node, err := strconv.ParseInt(ent.Name()[4:], 10, 16)
		if err != nil {
			continue
		}

		nodes = append(nodes, int(node))
	}

	return nodes
}

var nodeIdLen int

func newNodeWriter(format, subnet string, node int) (io.Writer, error) {
	w, err := logging.NewConsoleWriter(format)
	if err != nil {
		return nil, err
	}

	switch strings.ToLower(format) {
	case log.LogFormatPlain, log.LogFormatText:
		break
	default:
		return w, nil
	}

	c := fallbackColor
	if len(colors) > 0 {
		c = colors[0]
		colors = colors[1:]
	}

	id := fmt.Sprintf("%s.%d", subnet, node)
	prefix := c.Sprintf("[%s]", id) + strings.Repeat(" ", nodeIdLen-len(id)+1)

	cw := w.(*zerolog.ConsoleWriter)
	cw.Out = &nodeWriter{prefix, cw.Out}
	return cw, nil
}

type nodeWriter struct {
	prefix string
	w      io.Writer
}

func (w *nodeWriter) Write(b []byte) (int, error) {
	c := make([]byte, len(w.prefix)+len(b))
	n := copy(c, []byte(w.prefix))
	copy(c[n:], b)

	n, err := w.w.Write(c)
	if n >= len(w.prefix) {
		n -= len(w.prefix)
	}
	return n, err
}
