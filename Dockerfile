FROM golang:1.20 as build

# Build
WORKDIR /root
COPY . .
ENV CGO_ENABLED 0
ARG TAGS=production,mainnet
RUN make -B TAGS=$TAGS
RUN go install github.com/go-delve/delve/cmd/dlv@latest
RUN go install github.com/tendermint/tendermint/cmd/tendermint
RUN go build ./tools/cmd/snapshot
RUN go build ./tools/cmd/debug
RUN go build ./cmd/accumulated-bootstrap

FROM alpine:3

# Install tools
RUN apk add --no-cache bash jq curl nano

# Copy scripts
WORKDIR /scripts
COPY scripts .

# Copy binaries
COPY --from=build /root/accumulated /root/snapshot /root/debug /root/accumulated-bootstrap /go/bin/tendermint /go/bin/dlv /bin/

# Set health check
HEALTHCHECK CMD curl --fail --silent http://localhost:26660/status || exit 1

ENTRYPOINT ["accumulated"]
CMD ["run-dual", "/node/dnn", "/node/bvnn"]