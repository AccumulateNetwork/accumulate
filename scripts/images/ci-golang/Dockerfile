ARG VERSION=1.20
FROM golang:1.22 as build

WORKDIR /root/build
COPY go.mod go.sum ./

ENV CGO_ENABLED 0
RUN go install github.com/sourcegraph/scip/cmd/scip && \
    go install github.com/sourcegraph/scip-go/cmd/scip-go

FROM golang:${VERSION}

RUN apt update -y && apt install -y git-lfs

COPY --from=build /go/bin/scip /go/bin/scip-go /bin/
