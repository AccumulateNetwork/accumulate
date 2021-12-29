package main

import (
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"strings"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	var files []string
	err = filepath.WalkDir(cwd, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if strings.HasPrefix(path, ".") {
				return fs.SkipDir
			}
			return nil
		}

		if strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	args := append([]string{"run", "github.com/rinchsan/gosimports/cmd/gosimports", "-w"}, files...)
	cmd := exec.Command("go", args...)
	err = cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
}
