FROM golang:1.18 as build

# Build
WORKDIR /root
COPY . .
ENV CGO_ENABLED 0
ARG TAGS=production,mainnet
RUN make -B TAGS=$TAGS
RUN go install github.com/go-delve/delve/cmd/dlv@latest
RUN go install gitlab.com/accumulatenetwork/core/wallet/cmd/accumulate
RUN go build ./tools/cmd/snapshot

FROM alpine:3

# Install tools
RUN apk add --no-cache bash jq curl nano

# Copy scripts
WORKDIR /scripts
COPY scripts .

# Copy binaries
COPY --from=build /root/accumulated /root/snapshot /go/bin/accumulate /go/bin/dlv /bin/

# Set health check
HEALTHCHECK CMD curl --fail --silent http://localhost:26660/status || exit 1

ENTRYPOINT ["accumulated"]
CMD ["run-dual", "/node/dnn", "/node/bvnn"]