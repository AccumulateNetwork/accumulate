//go:build !testing
// +build !testing

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

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/testing"
)

var cmdRunDevnet = &cobra.Command{
	Use:   "devnet",
	Short: "Run all nodes of a local devnet",
	Run:   runDevNet,
	Args:  cobra.NoArgs,
}

var flagRunDevnet = struct {
	Except []int
	Debug  bool
}{}

func init() {
	cmdRun.AddCommand(cmdRunDevnet)

	cmdRunDevnet.Flags().IntSliceVarP(&flagRunDevnet.Except, "except", "x", nil, "Numbers of nodes that should not be launched")
	cmdRunDevnet.Flags().BoolVar(&flagRunDevnet.Debug, "debug", false, "Enable debugging features")

	if os.Getenv("FORCE_COLOR") != "" {
		color.NoColor = false
	}
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
	fmt.Println("Starting devnet")

	if flagRunDevnet.Debug {
		testing.EnableDebugFeatures()
	}

	skip := map[int]bool{}
	for _, id := range flagRunDevnet.Except {
		skip[id] = true
	}

	nodes := getNodeDirs(flagMain.WorkDir)
	for _, node := range nodes {
		id := fmt.Sprint(node)
		if len(id) > nodeIdLen {
			nodeIdLen = len(id)
		}
	}

	stop := make(chan struct{})
	didStop := make(chan struct{}, len(nodes)*2)
	done := new(sync.WaitGroup)

	logWriter := newLogWriter(nil)

	started := new(sync.WaitGroup)
	for _, node := range nodes {
		if skip[node] {
			continue
		}

		name := fmt.Sprintf("node-%d", node)

		// Load the DNN
		dnn, err := accumulated.Load(
			filepath.Join(flagMain.WorkDir, name, "dnn"),
			func(c *config.Config) (io.Writer, error) {
				return logWriter(c.LogFormat, func(w io.Writer, format string, color bool) io.Writer {
					return newNodeWriter(w, format, "dn", node, color)
				})
			},
		)
		check(err)

		bvnn, err := accumulated.Load(
			filepath.Join(flagMain.WorkDir, name, "bvnn"),
			func(c *config.Config) (io.Writer, error) {
				return logWriter(c.LogFormat, func(w io.Writer, format string, color bool) io.Writer {
					return newNodeWriter(w, format, strings.ToLower(c.Accumulate.PartitionId), node, color)
				})
			},
		)
		check(err)

		startDevNetNode(dnn, started, done, stop, didStop)
		startDevNetNode(bvnn, started, done, stop, didStop)

		// Connect once everything is setup
		go func() {
			started.Wait()
			check(dnn.ConnectDirectly(bvnn))
		}()

	}

	started.Wait()
	color.HiBlack("----- Started -----")

	// Wait for SIGINT
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	select {
	case <-sigs:
	case <-didStop:
	}

	// Turn of signal handling, so that another SIGINT will exit immediately
	signal.Stop(sigs)

	// Overwrite the ^C
	print("\r")
	color.HiBlack("----- Stopping -----")

	// Signal everyone to stop
	close(stop)

	// Wait for everyone to stop
	done.Wait()
}

func startDevNetNode(daemon *accumulated.Daemon, started, done *sync.WaitGroup, stop, didStop chan struct{}) {
	// Disable features not compatible with multi-node, single-process
	daemon.Config.Instrumentation.Prometheus = false

	started.Add(1)
	go func() {
		defer started.Done()

		// Start it
		check(daemon.Start())

		// On stop, send the signal
		go func() {
			<-daemon.Done()
			didStop <- struct{}{}
		}()

		// On signal, stop the node
		done.Add(1)
		go func() {
			defer done.Done()
			<-stop
			check(daemon.Stop())
		}()
	}()
}

func getNodeDirs(dir string) []int {
	var nodes []int

	ent, err := os.ReadDir(dir)
	checkf(err, "failed to read %q", dir)

	for _, ent := range ent {
		// We only want directories starting with node-
		if !ent.IsDir() || !strings.HasPrefix(ent.Name(), "node-") {
			continue
		}

		// We only want directories named node-#, e.g. node-1
		node, err := strconv.ParseInt(ent.Name()[5:], 10, 16)
		if err != nil {
			continue
		}

		nodes = append(nodes, int(node))
	}

	return nodes
}

var nodeIdLen int

var partitionColor = map[string]*color.Color{}

func newNodeWriter(w io.Writer, format, partition string, node int, color bool) io.Writer {
	switch format {
	case log.LogFormatPlain, log.LogFormatText:
		id := fmt.Sprintf("%s.%d", partition, node)
		s := fmt.Sprintf("[%s]", id) + strings.Repeat(" ", nodeIdLen+len("bvnxx")-len(id)+1)
		if !color {
			return &plainNodeWriter{s, w}
		}

		c, ok := partitionColor[partition]
		if !ok {
			c = fallbackColor
			if len(colors) > 0 {
				c = colors[0]
				colors = colors[1:]
			}
			partitionColor[partition] = c
		}

		s = c.Sprint(s)
		return &plainNodeWriter{s, w}

	case log.LogFormatJSON:
		s := fmt.Sprintf(`"partition":"%s","node":%d`, partition, node)
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
