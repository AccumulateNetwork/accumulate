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
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/play-accumulate/pkg"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
)

var extraFlags []string

func main() {
	cmd.PersistentFlags().StringSliceVarP(&extraFlags, "flags", "X", nil, "Extra flags for init")
	cmd.AddCommand(cmdPlay)
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

var cmdPlay = &cobra.Command{
	Use:  "play [playbooks]",
	Args: cobra.MinimumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		if !runPlaybooks(args) {
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

// Run the devnet and runScript a test against it.
func runScript() bool {
	// Build tools
	assertInModuleRoot()
	build("./cmd/accumulate")
	build("./cmd/accumulated")

	// Launch the devnet
	runCmd := launch()
	defer func() { _ = runCmd.Process.Kill() }()

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

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() { <-sig; print("\r"); signal.Stop(sig) }()

	err := valCmd.Start()
	checkf(err, "start script")

	// Run the validation script, but do not immediately check the return value
	err = valCmd.Wait()
	ok := err == nil
	if !ok {
		fmt.Printf("Error: validation script failed: %v\n", err)
	}

	stop(runCmd)

	return ok
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

func runPlaybooks(filenames []string) bool {
	// Build tools
	assertInModuleRoot()
	build("./cmd/accumulated")

	// Launch the devnet
	runCmd := launch()
	defer func() { _ = runCmd.Process.Kill() }()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() { <-sig; print("\r"); signal.Stop(sig) }()

	// Run the playbooks
	client, err := client.New("http://127.0.1.1:26660/v2")
	checkf(err, "creating client")

	ok := true
	for _, filename := range filenames {
		err := pkg.ExecuteFile(filename, client)
		if err != nil {
			ok = false
			break
		}
	}

	stop(runCmd)

	return ok
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
