all: build

# Go handles build caching, so Go targets should always be marked phony.
.PHONY: all build tags

GIT_DESCRIBE = $(shell git fetch --tags -q ; git describe --dirty)
GIT_COMMIT = $(shell git rev-parse HEAD)
VERSION = gitlab.com/accumulatenetwork/accumulate.Version=$(GIT_DESCRIBE)
COMMIT = gitlab.com/accumulatenetwork/accumulate.Commit=$(GIT_COMMIT)

# TODO Add mainnet to tags
LDFLAGS = '-X "$(VERSION)" -X "$(COMMIT)"'
#FLAGS = $(BUILDFLAGS) -tags production -ldflags $(LDFLAGS)
FLAGS = $(BUILDFLAGS) -ldflags $(LDFLAGS)

build:
	go build $(FLAGS) ./cmd/accumulated

install:
	go install $(FLAGS) ./cmd/accumulated

accumulate:
	go build $(FLAGS) ./cmd/accumulate
