.PHONY: build install

build:
	go build -ldflags "-X main.BuildTag=$(shell git describe)" .

install:
	go install -ldflags "-X main.BuildTag=$(shell git describe)" .