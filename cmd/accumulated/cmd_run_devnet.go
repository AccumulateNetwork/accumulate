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
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/libs/log"
)

var cmdRunDevnet = &cobra.Command{
	Use:   "devnet",
	Short: "Run all nodes of a local devnet",
	Run:   runDevNet,
	Args:  cobra.NoArgs,
}

var flagRunDevnet = struct {
	Except []string
}{}

func init() {
	cmdRun.AddCommand(cmdRunDevnet)

	cmdRunDevnet.Flags().StringArrayVarP(&flagRunDevnet.Except, "except", "x", nil, "Nodes that should not be launched, e.g. bvn0.0")
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
	skip := map[string]bool{}
	for _, id := range flagRunDevnet.Except {
		skip[id] = true
	}

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

	logWriter := newLogWriter(nil)

	for _, node := range nodes {
		if skip[fmt.Sprintf("%s.%d", node.Subnet, node.Number)] {
			continue
		}

		// Load the node
		dir := filepath.Join(dir, node.Subnet, fmt.Sprintf("Node%d", node.Number))
		daemon, err := accumulated.Load(dir, func(format string) (io.Writer, error) {
			return logWriter(format, func(w io.Writer, format string, color bool) io.Writer {
				return newNodeWriter(w, format, node.Subnet, node.Number, color)
			})
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

var subnetColor = map[string]*color.Color{}

func newNodeWriter(w io.Writer, format, subnet string, node int, color bool) io.Writer {
	switch format {
	case log.LogFormatPlain, log.LogFormatText:
		id := fmt.Sprintf("%s.%d", subnet, node)
		s := fmt.Sprintf("[%s]", id) + strings.Repeat(" ", nodeIdLen-len(id)+1)
		if !color {
			return &plainNodeWriter{s, w}
		}

		c, ok := subnetColor[subnet]
		if !ok {
			c = fallbackColor
			if len(colors) > 0 {
				c = colors[0]
				colors = colors[1:]
			}
			subnetColor[subnet] = c
		}

		s = c.Sprint(s)
		return &plainNodeWriter{s, w}

	case log.LogFormatJSON:
		s := fmt.Sprintf(`"subnet":"%s","node":%d`, subnet, node)
		return &jsonNodeWriter{s, w}

	default:
		return w
	}
}

type plainNodeWriter struct {
	s string
	w io.Writer
}

func (w *plainNodeWriter) Write(b []byte) (int, error) {
	c := make([]byte, len(w.s)+len(b))
	n := copy(c, []byte(w.s))
	copy(c[n:], b)

	n, err := w.w.Write(c)
	if n >= len(w.s) {
		n -= len(w.s)
	}
	return n, err
}

type jsonNodeWriter struct {
	s string
	w io.Writer
}

func (w *jsonNodeWriter) Write(b []byte) (int, error) {
	if b[0] != '{' {
		return w.w.Write(b)
	}

	c := make([]byte, 0, len(w.s)+len(b)+1)
	c = append(c, '{')
	c = append(c, []byte(w.s)...)
	c = append(c, ',')
	c = append(c, b[1:]...)

	n, err := w.w.Write(c)
	if n >= len(w.s)+1 {
		n -= len(w.s) + 1
	}
	return n, err
}
