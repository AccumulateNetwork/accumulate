// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	tmconfig "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/libs/log"
	tmp2p "github.com/cometbft/cometbft/p2p"
	"github.com/fatih/color"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/multiformats/go-multiaddr"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/exp/faucet"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
)

var cmdRunDevnet = &cobra.Command{
	Use:   "devnet",
	Short: "Run all nodes of a local devnet",
	Run:   runDevNet,
	Args:  cobra.NoArgs,
}

var flagRunDevnet = struct {
	Except     []int
	FaucetSeed string
}{}

func init() {
	cmdRun.AddCommand(cmdRunDevnet)

	cmdRunDevnet.Flags().IntSliceVarP(&flagRunDevnet.Except, "except", "x", nil, "Numbers of nodes that should not be launched")
	cmdRunDevnet.Flags().StringVar(&flagRunDevnet.FaucetSeed, "faucet-seed", "", "If specified, runs a faucet service for the faucet generated by the given seed")

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

	if flagRun.Debug {
		testing.EnableDebugFeatures()
	}

	skip := map[int]bool{}
	for _, id := range flagRunDevnet.Except {
		skip[id] = true
	}

	vals, bsns, hasBS := getNodeDirs(flagMain.WorkDir)
	for _, node := range vals {
		name := fmt.Sprintf("node-%d", node)
		c, err := config.Load(filepath.Join(flagMain.WorkDir, name, "bvnn"))
		check(err)

		id := fmt.Sprintf("%s.%d", c.Accumulate.PartitionId, node)
		if len(id) > nodeIdLen {
			nodeIdLen = len(id)
		}
	}

	stop := make(chan struct{})
	didStop := make(chan struct{}, len(vals)*2+len(bsns))
	done := new(sync.WaitGroup)

	logWriter := newLogWriter()

	var daemons []*accumulated.Daemon
	started := new(sync.WaitGroup)

	if hasBS {
		startDevnetBootstrap(makeLogger(logWriter), done, stop)
	}

	for _, num := range bsns {
		if skip[-num] {
			continue
		}

		name := fmt.Sprintf("bsn-%d", num)
		daemon, err := accumulated.Load(
			filepath.Join(flagMain.WorkDir, name, "bsnn"),
			func(c *config.Config) (io.Writer, error) {
				return logWriter(c.LogFormat, func(w io.Writer, format string, color bool) io.Writer {
					return newNodeWriter(w, format, "bsn", num, color)
				})
			},
		)
		check(err)

		startDevNetNode(daemon, nil, started, done, stop, didStop)

		daemons = append(daemons, daemon)
	}

	// Wait for the BSN to start since the validators will send messages to it
	started.Wait()

	for _, node := range vals {
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

		startDevNetNode(dnn, bvnn, started, done, stop, didStop)

		daemons = append(daemons, dnn, bvnn)
	}

	started.Wait()

	// Connect every node to every other node
	for i, d := range daemons {
		for _, e := range daemons[i+1:] {
			err := d.ConnectDirectly(e)
			if err == nil {
				continue
			}
			if err.Error() != "cannot connect nodes directly as they have the same node key" {
				check(err)
			}
		}
	}

	if flagRunDevnet.FaucetSeed != "" {
		startDevnetFaucet(daemons, makeLogger(logWriter), done, stop)
	}

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

func startDevNetNode(primary, secondary *accumulated.Daemon, started, done *sync.WaitGroup, stop, didStop chan struct{}) {
	// Disable features not compatible with multi-node, single-process
	primary.Config.Instrumentation.Prometheus = false
	if secondary != nil {
		secondary.Config.Instrumentation.Prometheus = false
	}

	started.Add(1)
	go func() {
		defer started.Done()

		// Start it
		check(primary.Start(secondary))
		if secondary != nil {
			check(secondary.StartSecondary(primary))
		}

		// On stop, send the signal
		go func() {
			<-primary.Done()
			didStop <- struct{}{}
		}()
		if secondary != nil {
			go func() {
				<-secondary.Done()
				didStop <- struct{}{}
			}()
		}

		// On signal, stop the node
		done.Add(1)
		if secondary != nil {
			done.Add(1)
		}
		go func() {
			defer done.Done()
			<-stop
			check(primary.Stop())
		}()
		if secondary != nil {
			go func() {
				defer done.Done()
				<-stop
				check(secondary.Stop())
			}()
		}
	}()
}

func getNodeDirs(dir string) (vals, bsns []int, bootstrap bool) {
	ent, err := os.ReadDir(dir)
	checkf(err, "failed to read %q", dir)

	for _, ent := range ent {
		// We only want directories starting with node- or bsn-
		var nodes *[]int
		var num string
		switch {
		case !ent.IsDir():
			continue
		case ent.Name() == "bootstrap":
			bootstrap = true
			continue
		case strings.HasPrefix(ent.Name(), "node-"):
			nodes, num = &vals, ent.Name()[5:]
		case strings.HasPrefix(ent.Name(), "bsn-"):
			nodes, num = &bsns, ent.Name()[4:]
		default:
			continue
		}

		// We only want directories named node-#, e.g. node-1
		node, err := strconv.ParseInt(num, 10, 16)
		if err != nil {
			continue
		}

		*nodes = append(*nodes, int(node))
	}

	return vals, bsns, bootstrap
}

func makeLogger(lw func(string, logAnnotator) (io.Writer, error)) log.Logger {
	w, err := lw("plain", func(w io.Writer, format string, color bool) io.Writer {
		return &plainNodeWriter{s: "[faucet] ", w: w}
	})
	check(err)
	logLevel, logWriter, err := logging.ParseLogLevel("info", w)
	check(err)
	logger, err := logging.NewTendermintLogger(zerolog.New(logWriter), logLevel, false)
	check(err)
	return logger
}

func startDevnetBootstrap(logger log.Logger, done *sync.WaitGroup, stop chan struct{}) {
	// Load stuff
	dir := filepath.Join(flagMain.WorkDir, "bootstrap")
	cfg, err := config.LoadAcc(dir)
	check(err)

	nodeKey, err := tmp2p.LoadNodeKey(filepath.Join(dir, "node_key.json"))
	check(err)

	node, err := p2p.New(p2p.Options{
		Key:           nodeKey.PrivKey.Bytes(),
		Listen:        cfg.P2P.Listen,
		DiscoveryMode: dht.ModeAutoServer,
		// TODO External address
	})
	check(err)

	fmt.Println("Bootstrap")
	for _, a := range node.Addresses() {
		fmt.Printf("  %s\n", a)
	}
	fmt.Println()

	// Cleanup
	done.Add(1)
	go func() {
		defer done.Done()
		<-stop
		check(node.Close())
	}()
}

func startDevnetFaucet(daemons []*accumulated.Daemon, logger log.Logger, done *sync.WaitGroup, stop chan struct{}) {
	var seed storage.Key
	for _, s := range strings.Split(flagRunDevnet.FaucetSeed, " ") {
		seed = seed.Append(s)
	}
	sk := ed25519.NewKeyFromSeed(seed[:])
	u, err := protocol.LiteTokenAddress(sk[32:], "ACME", protocol.SignatureTypeED25519)
	check(err)

	// Directly connect to all the validator nodes
	var peers []multiaddr.Multiaddr
	for _, d := range daemons {
		peers = append(peers, d.P2P_TESTONLY().Addresses()...)
		// check(node.ConnectDirectly(d.P2P_TESTONLY()))
	}

	// Cleanup
	ctx, cancel := context.WithCancel(context.Background())
	done.Add(1)
	go func() {
		defer done.Done()
		<-stop
		cancel()
	}()

	// Start the faucet node
	node, err := faucet.StartLite(ctx, faucet.Options{
		Key:     sk,
		Network: daemons[0].Config.Accumulate.Network.Id,
		Logger:  logger,
		Peers:   peers,
	})
	check(err)

	fmt.Printf("Started faucet for %v on %v\n", u, node.Addresses()[0])
}

var nodeIdLen int

var partitionColor = map[string]*color.Color{}

func newNodeWriter(w io.Writer, format, partition string, node int, color bool) io.Writer {
	switch format {
	case tmconfig.LogFormatPlain:
		id := fmt.Sprintf("%s.%d", partition, node)
		if nodeIdLen < len(id) {
			nodeIdLen = len(id)
		}
		s := fmt.Sprintf("[%s]", id) + strings.Repeat(" ", nodeIdLen-len(id)+1)
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

	case tmconfig.LogFormatJSON:
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
