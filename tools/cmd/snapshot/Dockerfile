FROM golang:1.23 as build

# Build
WORKDIR /root
COPY . .
ENV CGO_ENABLED 0
RUN go install github.com/go-delve/delve/cmd/dlv@latest
RUN go build ./tools/cmd/snapshot

FROM alpine:3

# Install tools
RUN apk add --no-cache bash nano

# Copy scripts
WORKDIR /scripts
COPY scripts .

# Copy binaries
COPY --from=build /root/snapshot /go/bin/dlv /bin/

ENTRYPOINT ["snapshot"]
CMD ["help"]