package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
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

func execTest(t *testing.T, args []string) (string, error) {
	defaultWorkDir, err := ioutil.TempDir("", "cliTest")
	if err != nil {
		t.Fatal(t)
	}

	rootCmd := InitRootCmd(initDB(defaultWorkDir))

	e := bytes.NewBufferString("")
	b := bytes.NewBufferString("")
	rootCmd.SetErr(e)
	rootCmd.SetOut(b)
	rootCmd.SetArgs(args)
	rootCmd.Execute()
	eprint, err := ioutil.ReadAll(e)
	if err != nil {
		return "", err
	} else if len(eprint) != 0 {
		return "", fmt.Errorf("%s", string(eprint))
	}
	ret, err := ioutil.ReadAll(b)
	return string(ret), err
}

func TestKeys(t *testing.T) {

	argsKeyList := []string{"-j", "key", "list"}
	argsKeyGenerate := []string{"-j", "key", "generate", "testkey1"}

	out, err := execTest(t, argsKeyGenerate)
	if err != nil {
		t.Fatal(err)
	}

	if string(out) != "hi-via-args" {
		t.Fatalf("expected \"%s\" got \"%s\"", "hi-via-args", string(out))
	}

	out, err = execTest(t, argsKeyList)
	if err != nil {
		t.Fatal(err)
	}

	if string(out) != "hi-via-args" {
		t.Fatalf("expected \"%s\" got \"%s\"", "hi-via-args", string(out))
	}
}
