package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var extraFlags []string

func main() {
	cmd.PersistentFlags().StringSliceVarP(&extraFlags, "flags", "X", nil, "Extra flags for init")
	_ = cmd.Execute()
}

var cmd = &cobra.Command{
	Use: "devnet",
	Run: func(*cobra.Command, []string) {
		_, err := initClient("http://localhost:26660/v2")
		if err != nil {
			os.Exit(1)
		}
	},
}

var resetColor = color.New(color.Reset)

func assertInModuleRoot() {
	cwd, err := os.Getwd()
	checkf(err, "getwd")

	_, err = os.Stat(filepath.Join(cwd, "go.mod"))
	checkf(err, "stat go.mod failed - are we in the repo root?")
}

func build(tool string) {
	buildCmd := exec.Command("go", "build", tool)
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stdout
	err := buildCmd.Run()
	checkf(err, "build %s", tool)
}

func launch() *exec.Cmd {
	// Initialize the devnet command
	args := append([]string{"init", "devnet", "--work-dir", ".nodes"}, extraFlags...)
	initCmd := exec.Command("./accumulated", args...)
	initCmd.Stdout = os.Stdout
	initCmd.Stderr = os.Stdout
	err := initCmd.Run()
	checkf(err, "init devnet")

	// Configure the devnet command
	runCmd := exec.Command("./accumulated", "run", "devnet", "--work-dir", ".nodes")
	runCmd.Env = os.Environ()
	runCmd.Env = append(runCmd.Env, "FORCE_COLOR=true")

	// Don't interrupt the run process if the parent process is interrupted
	runCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Forward output
	runRd, runWr := io.Pipe()
	runCmd.Stdout = runWr
	runCmd.Stderr = runWr
	started := make(chan struct{})

	go func() {
		bufRd := bufio.NewReader(runRd)
		for {
			line, err := bufRd.ReadString('\n')
			if errors.Is(err, io.EOF) {
				return
			}
			checkf(err, "read devnet stdout/stderr")

			if strings.Contains(line, "----- Started -----") {
				close(started)
			}

			print(line + resetColor.Sprint(""))
		}
	}()

	// Start the devnet
	err = runCmd.Start()
	checkf(err, "start devnet")
	<-started

	return runCmd
}

func stop(runCmd *exec.Cmd) {
	err := runCmd.Process.Signal(os.Interrupt)
	if err != nil {
		fmt.Printf("Error: interrupt devnet: %v\n", err)
	}

	go func() {
		time.Sleep(time.Minute)
		_ = runCmd.Process.Kill()
	}()

	err = runCmd.Wait()
	if err != nil {
		fmt.Printf("Error: wait for devnet: %v\n", err)
	}
}

// Init new client from server URL input using client.go
func initClient(server string) (string, error) {

	// build Accumulate deamon
	assertInModuleRoot()
	build("./cmd/accumulated")

	// Launch the devnet
	runCmd := launch()
	defer func() { _ = runCmd.Process.Kill() }()

	// Create new client on localhost
	client, err := client.New("http://127.0.1.1:26660/v2")
	checkf(err, "creating client")
	client.DebugRequest = true

	// Limit amount of goroutines
	maxGoroutines := 1
	guard := make(chan struct{}, maxGoroutines)

	// Add timer to measure TPS
	timer := time.NewTimer(time.Microsecond)

	// run key generation in cycle
	for i := 0; i < 1; i++ {
		guard <- struct{}{} // would block if guard channel is already filled

		// generate accounts and faucet in goroutines
		go func(n int) {
			// create accounts and store them
			acc, _ := createAccount(i)

			// start timer
			start := time.Now()

			// faucet account
			_, err = client.Faucet(context.Background(), &protocol.AcmeFaucet{Url: acc})

			// wait for timer to fire
			log.Printf("Execution time %s\n", time.Since(start))

			// reset timer
			timer.Reset(time.Microsecond)
			<-timer.C

			// time to release goroutine
			<-guard
		}(i)
	}

	// wait for goroutines to finish
	for i := 0; i < maxGoroutines; i++ {
		guard <- struct{}{}
	}

	stop(runCmd)

	return "", nil
}

// helper function to generate key and create account and return the address
func createAccount(i int) (*url.URL, error) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		fmt.Printf("Error: generating keys: %v\n", err)
	}

	acc, err := protocol.LiteTokenAddress(pub, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		fmt.Printf("Error: creating Lite Token account: %v\n", err)
	}

	fmt.Printf("Account %d: %s\n", i, acc)

	return acc, nil
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func checkf(err error, format string, otherArgs ...interface{}) {
	if err != nil {
		fatalf(format+": %v", append(otherArgs, err)...)
	}
}
