package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
)

// Run the devnet and run a test against it.
func main() {
	resetColor := color.New(color.Reset)

	cwd, err := os.Getwd()
	checkf(err, "getwd")

	_, err = os.Stat(filepath.Join(cwd, "go.mod"))
	checkf(err, "stat go.mod failed - are we in the repo root?")

	// Initialize the devnet command
	args := append([]string{"run", "./cmd/accumulated", "init", "devnet", "--work-dir", ".nodes"}, os.Args[1:]...)
	initCmd := exec.Command("go", args...)
	initCmd.Stdout = os.Stdout
	initCmd.Stderr = os.Stdout
	err = initCmd.Run()
	checkf(err, "init devnet")

	// Configure the devnet command
	runCmd := exec.Command("go", "run", "./cmd/accumulated", "run", "devnet", "--work-dir", ".nodes")
	runCmd.Env = os.Environ()
	runCmd.Env = append(runCmd.Env, "FORCE_COLOR=true")

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

	// Configure the validator script command
	valCmd := exec.Command("./scripts/ci/validate.sh")
	valCmd.Env = os.Environ()
	valCmd.Env = append(valCmd.Env, "ACC_API=http://127.0.1.1:26660/v2") // won't work if --ip was passed
	valCmd.Env = append(valCmd.Env, "NODE_ROOT_0=.nodes/dn/Node0")
	valCmd.Env = append(valCmd.Env, "NODE_ROOT_1=.nodes/dn/Node1")

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

	err = valCmd.Start()
	checkf(err, "start script")

	// Run the validation script, but do not immediately check the return value
	err = valCmd.Wait()
	if err != nil {
		fmt.Printf("Error: validation script failed: %v\n", err)
	}

	// TODO Shut down gracefully. Every attempt I made to send SIGINT to the
	// devnet failed.
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
