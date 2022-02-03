all: build

# Go handles build caching, so Go targets should always be marked phony.
.PHONY: all build tags

GIT_DESCRIBE = $(shell git fetch --tags -q ; git describe --dirty)
GIT_COMMIT = $(shell git rev-parse HEAD)
VERSION = gitlab.com/accumulatenetwork/accumulate.Version=$(GIT_DESCRIBE)
COMMIT = gitlab.com/accumulatenetwork/accumulate.Commit=$(GIT_COMMIT)

LDFLAGS = '-X "$(VERSION)" -X "$(COMMIT)"'

build:
	go build $(BUILDFLAGS) -ldflags $(LDFLAGS) ./cmd/accumulated

install:
	go install -ldflags $(LDFLAGS) ./cmd/accumulated

accumulate:
	go build $(BUILDFLAGS) -ldflags $(LDFLAGS) ./cmd/accumulate