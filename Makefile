all: build

# Go handles build caching, so Go targets should always be marked phony.
.PHONY: all build docker docker-push http http-docker http-docker-push faucet-docker faucet-docker-push healing healing-docker healing-docker-push

GIT_DESCRIBE = $(shell git fetch --tags -q ; git describe --dirty)
GIT_COMMIT = $(shell git rev-parse HEAD)
VERSION = gitlab.com/accumulatenetwork/accumulate.Version=$(GIT_DESCRIBE)
COMMIT = gitlab.com/accumulatenetwork/accumulate.Commit=$(GIT_COMMIT)
IMAGE = registry.gitlab.com/accumulatenetwork/accumulate

TAGS=production,mainnet
LDFLAGS = '-X "$(VERSION)" -X "$(COMMIT)"'
FLAGS = $(BUILDFLAGS) -tags $(TAGS) -ldflags $(LDFLAGS)

build:
	go build -trimpath $(FLAGS) ./cmd/accumulated

docker:
	docker build --build-arg "GIT_DESCRIBE=$(GIT_DESCRIBE)" --build-arg "GIT_COMMIT=$(GIT_COMMIT)" -t "$(IMAGE)" .

docker-push: docker
	docker push "$(IMAGE)"

healing-docker:
	$(MAKE) docker IMAGE="$(IMAGE):healing"

healing-docker-push: healing-docker
	docker push "$(IMAGE):healing"

http:
	go build -trimpath $(FLAGS) ./cmd/accumulated-http

http-docker:
	docker build --build-arg "GIT_DESCRIBE=$(GIT_DESCRIBE)" --build-arg "GIT_COMMIT=$(GIT_COMMIT)" -t "$(IMAGE)/http" -f cmd/accumulated-http/Dockerfile .

http-docker-push: http-docker
	docker push "$(IMAGE)/http"

faucet-docker:
	docker build --build-arg "GIT_DESCRIBE=$(GIT_DESCRIBE)" --build-arg "GIT_COMMIT=$(GIT_COMMIT)" -t "$(IMAGE)/faucet" -f cmd/accumulated-faucet/Dockerfile .

faucet-docker-push: faucet-docker
	docker push "$(IMAGE)/faucet"

sim:
	go build -trimpath $(FLAGS) ./tools/cmd/simulator

sim-docker:
	docker build --build-arg "GIT_DESCRIBE=$(GIT_DESCRIBE)" --build-arg "GIT_COMMIT=$(GIT_COMMIT)" -t "$(IMAGE)/simulator" -f tools/cmd/simulator/Dockerfile .

sim-docker-push: sim-docker
	docker push "$(IMAGE)/simulator"