package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/fatih/color"
)

func main() {
	if !run() {
		os.Exit(1)
	}
}

// Run the devnet and run a test against it.
func run() bool {
	resetColor := color.New(color.Reset)

	cwd, err := os.Getwd()
	checkf(err, "getwd")

	_, err = os.Stat(filepath.Join(cwd, "go.mod"))
	checkf(err, "stat go.mod failed - are we in the repo root?")

	// Build the CLI
	buildCmd := exec.Command("go", "install", "./cmd/accumulate")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stdout
	err = buildCmd.Run()
	checkf(err, "build cli")

	// Build the daemon
	buildCmd = exec.Command("go", "build", "./cmd/accumulated")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stdout
	err = buildCmd.Run()
	checkf(err, "build daemon")

	// Initialize the devnet command
	args := append([]string{"init", "devnet", "--work-dir", ".nodes"}, os.Args[1:]...)
	initCmd := exec.Command("./accumulated", args...)
	initCmd.Stdout = os.Stdout
	initCmd.Stderr = os.Stdout
	err = initCmd.Run()
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
	defer runCmd.Process.Kill()
	<-started

	// Configure the validator script command
	valCmd := exec.Command("./scripts/ci/validate.sh")
	valCmd.Env = os.Environ()
	valCmd.Env = append(valCmd.Env, "ACC_API=http://127.0.1.1:26660/v2") // won't work if --ip was passed
	valCmd.Env = append(valCmd.Env, "NODE_ROOT=.nodes/dn/Node0")

	// Forward output
	valRd, valWr := io.Pipe()
	valCmd.Stdout = valWr
	valCmd.Stderr = valWr

	go func() {
		c := color.New(color.Faint, color.Bold)
		bufRd := bufio.NewReader(valRd)
		for {
			line, err := bufRd.ReadString('\n')
			if errors.Is(err, io.EOF) {
				return
			}
			checkf(err, "read validator stdout/stderr")

			if line == "\n" {
				continue
			}

			print(c.Sprint("[script]") + " " + line + resetColor.Sprint(""))
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() { <-sig; print("\r"); signal.Stop(sig) }()

	err = valCmd.Start()
	checkf(err, "start script")

	// Run the validation script, but do not immediately check the return value
	err = valCmd.Wait()
	ok := err == nil
	if !ok {
		fmt.Printf("Error: validation script failed: %v\n", err)
	}

	err = runCmd.Process.Signal(os.Interrupt)
	if err != nil {
		fmt.Printf("Error: interrupt devnet: %v\n", err)
	}

	err = runCmd.Wait()
	if err != nil {
		fmt.Printf("Error: wait for devnet: %v\n", err)
	}

	return !ok
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

func checkf(err error, format string, otherArgs ...interface{}) {
	if err != nil {
		fatalf(format+": %v", append(otherArgs, err)...)
	}
}
