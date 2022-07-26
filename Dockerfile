FROM golang:1.18 as build

# Build
WORKDIR /root
COPY . .
ENV CGO_ENABLED 0
RUN make -B && make -B accumulate
RUN go install -ldflags "-s -w -extldflags '-static'" github.com/go-delve/delve/cmd/dlv

FROM alpine:3

# Install tools
RUN apk add --no-cache bash jq curl

# Copy scripts
WORKDIR /scripts
COPY scripts .

# Copy binaries
COPY --from=build /root/accumulate /root/accumulated /bin/
COPY --from=build /go/bin/dlv /bin/

# Set health check
HEALTHCHECK CMD curl --fail --silent http://localhost:26660/status || exit 1

ENTRYPOINT ["accumulated"]
CMD ["run-dual", "/node/dnn", "/node/bvnn"]