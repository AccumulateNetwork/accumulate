all: build

# Go handles build caching, so Go targets should always be marked phony.
.PHONY: all build tags

GIT_DESCRIBE = $(shell git fetch --tags -q ; git describe --dirty)
GIT_COMMIT = $(shell git rev-parse HEAD)
VERSION = gitlab.com/accumulatenetwork/accumulate.Version=$(GIT_DESCRIBE)
COMMIT = gitlab.com/accumulatenetwork/accumulate.Commit=$(GIT_COMMIT)

TAGS=production,mainnet
LDFLAGS = '-X "$(VERSION)" -X "$(COMMIT)"'
FLAGS = $(BUILDFLAGS) -tags $(TAGS) -ldflags $(LDFLAGS)

build:
	go build -trimpath $(FLAGS) ./cmd/accumulated

install:
	go install -trimpath $(FLAGS) ./cmd/accumulated
