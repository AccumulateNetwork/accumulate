package main

import (
	"testing"
	"time"
)


func TestSimulator (t *testing.T){
	sim := new(Simulator)
	sim.Init()
	go sim.Run()

	time.Sleep(10*time.Second)
}