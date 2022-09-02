package main

import (
	"fmt"
	"os"
	"os/user"
	"path"
)

var ReportDirectory string

func main() {
	u, _ := user.Current()
	ReportDirectory = path.Join(u.HomeDir + "/StakingReports")
	if err := os.MkdirAll(ReportDirectory, os.ModePerm); err != nil {
		fmt.Println(err)
		return
	}

	s := new(StakingApp)
	s.Run()
}
