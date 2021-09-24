package main

import (
	"log"
	"os"
	"os/user"
	"path/filepath"

	"github.com/spf13/cobra"
)

var currentUser = func() *user.User {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	return usr
}()

var defaultWorkDir = filepath.Join(currentUser.HomeDir, ".accumulate")

var cmdMain = &cobra.Command{
	Use:   "accumulated",
	Short: "Accumulate network daemon",
	Run:   printUsageAndExit1,
}

var flagMain struct {
	WorkDir string
}

func init() {
	cmdMain.PersistentFlags().StringVarP(&flagMain.WorkDir, "work-dir", "w", defaultWorkDir, "Working directory for configuration and data")
}

func main() {
	_ = cmdMain.Execute()
}

func printUsageAndExit1(cmd *cobra.Command, args []string) {
	_ = cmd.Usage()
	os.Exit(1)
}
