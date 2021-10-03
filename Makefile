all: build

# Go handles build caching, so Go targets should always be marked phony.
.PHONY: all build

build:
	go build -ldflags "-X 'github.com/AccumulateNetwork/accumulated.Version=$(shell git describe --dirty)'" ./cmd/accumulated

install:
	go install -ldflags "-X 'github.com/AccumulateNetwork/accumulated.Version=$(shell git describe --dirty)'" ./cmd/accumulated