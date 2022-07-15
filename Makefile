all: build

# Go handles build caching, so Go targets should always be marked phony.
.PHONY: all build tags

GIT_DESCRIBE = $(shell git fetch --tags -q ; git describe --dirty)
GIT_COMMIT = $(shell git rev-parse HEAD)
VERSION = gitlab.com/accumulatenetwork/accumulate.Version=$(GIT_DESCRIBE)
COMMIT = gitlab.com/accumulatenetwork/accumulate.Commit=$(GIT_COMMIT)

# TODO Add mainnet to tags
LDFLAGS = '-X "$(VERSION)" -X "$(COMMIT)"'
FLAGS = $(BUILDFLAGS) -tags production -ldflags $(LDFLAGS)

build:
	go build $(FLAGS) ./cmd/accumulated

install:
	go install $(FLAGS) ./cmd/accumulated

accumulate:
	go build $(FLAGS) ./cmd/accumulate

check-swagger:
	#go get -u github.com/hashicorp/hcl/hcl/printer@v1.0.1-vault-3
	#which swagger || (GO111MODULE=off go get -u github.com/go-swagger/go-swagger/cmd/swagger)
	#which swagger || (go get -u github.com/go-swagger/go-swagger/cmd/swagger)
	which swagger || (go install github.com/go-swagger/go-swagger/cmd/swagger)

swagger: check-swagger
	GO111MODULE=on go mod vendor  && GO111MODULE=off swagger generate spec -o ./swagger.yaml --scan-models

serve-swagger: check-swagger
	swagger serve -F=swagger swagger.yaml
