package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
)

var extraFlags []string

func main() {
	cmd.PersistentFlags().StringSliceVarP(&extraFlags, "flags", "X", nil, "Extra flags for init")
	_ = cmd.Execute()
}

var cmd = &cobra.Command{
	Use: "devnet",
	Run: func(*cobra.Command, []string) {
		if !runScript() {
			os.Exit(1)
		}
	},
}

var resetColor = color.New(color.Reset)

unc assertInModuleRoot() {
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

func runScript() bool {
	// Build tools
	assertInModuleRoot()
	build("./cmd/accumulate")
	build("./cmd/accumulated")

	// Launch the devnet
	runCmd := launch()
	defer func() { _ = runCmd.Process.Kill() }()

}

// Init new client from server URL input using client.go
func initClient(server string) (string, error) {

	// Create new client on localhost
	c := "http://127.0.1.1:26660/v2"
	client, err := client.New(c)
	checkf(err, "creating client")
	client.DebugRequest = true

	return c, err
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
