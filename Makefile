all: build

# Go handles build caching, so Go targets should always be marked phony.
.PHONY: all build

VERSION = github.com/AccumulateNetwork/accumulated.Version=$(shell git describe --dirty)
COMMIT = github.com/AccumulateNetwork/accumulated.Commit=$(shell git rev-parse HEAD)
LDFLAGS = '-X "$(VERSION)" -X "$(COMMIT)"'

build:
	git fetch --tags
	go build -ldflags $(LDFLAGS) ./cmd/accumulated

install:
	git fetch --tags
	go install -ldflags $(LDFLAGS) ./cmd/accumulated