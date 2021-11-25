package cmd

import (
	"bytes"
	"fmt"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/spf13/cobra"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"
)

//import (
//	"os"
//	"strings"
//	"testing"
//)
//
//func _TestImportMneumonic(t *testing.T) {
//	if os.Getenv("CI") == "true" {
//		t.Skip("Depends on an external resource, and thus is not appropriate for CI")
//	}
//
//	mnemonic := "yellow yellow yellow yellow yellow yellow yellow yellow yellow yellow yellow yellow"
//
//	ImportMnemonic("seed", strings.Fields(mnemonic))
//
//}

type testCmd struct {
	rootCmd        *cobra.Command
	cmd            *exec.Cmd
	defaultWorkDir string
}

func (c *testCmd) finalize(t *testing.T) {
	c.cmd.Process.Kill()

	os.RemoveAll(c.defaultWorkDir)

}
func (c *testCmd) initalize(t *testing.T) *exec.Cmd {
	defaultWorkDir, err := ioutil.TempDir("", "cliTest")
	if err != nil {
		t.Fatal(err)
	}

	err = os.MkdirAll(defaultWorkDir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	c.rootCmd = InitRootCmd(initDB(defaultWorkDir))

	//start the Badlands node.
	cmd := exec.Command(fmt.Sprintf("go run ../../accumulated/main.go -w %s init -n Badlands", defaultWorkDir))
	cmd.Wait()
	cmd = exec.Command(fmt.Sprintf("go run ../../accumulated/main.go -w %s run -n 0", defaultWorkDir))
	return cmd
}

func (c *testCmd) execute(t *testing.T, cmdLine string) (string, error) {

	fullCommand := fmt.Sprintf("-s http://$v:%v/v1 %s", networks.Local["Badlands"].Nodes[0].IP, networks.Local["Badlands"].Port+4, cmdLine)
	args := strings.Split(fullCommand, " ")

	e := bytes.NewBufferString("")
	b := bytes.NewBufferString("")
	c.rootCmd.SetErr(e)
	c.rootCmd.SetOut(b)
	c.rootCmd.SetArgs(args)
	c.rootCmd.Execute()

	eprint, err := ioutil.ReadAll(e)
	if err != nil {
		t.Fatal(err)
	} else if len(eprint) != 0 {
		t.Fatalf("%s", string(eprint))
	}
	ret, err := ioutil.ReadAll(b)
	return string(ret), err
}

func TestKeys(t *testing.T) {

	tc := testCmd{}
	tc.initalize(t)

	argsKeyList := "-j key list"
	argsKeyGenerate := "-j key generate testkey1"

	out, err := tc.execute(t, argsKeyGenerate)
	if err != nil {
		t.Fatal(err)
	}

	if string(out) != "hi-via-args" {
		t.Fatalf("expected \"%s\" got \"%s\"", "hi-via-args", string(out))
	}

	out, err = tc.execute(t, argsKeyList)
	if err != nil {
		t.Fatal(err)
	}

	if string(out) != "hi-via-args" {
		t.Fatalf("expected \"%s\" got \"%s\"", "hi-via-args", string(out))
	}
}
