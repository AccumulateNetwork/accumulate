FROM golang:1.23 as build

# Build
WORKDIR /root
COPY . .
ENV CGO_ENABLED 0
RUN go install github.com/go-delve/delve/cmd/dlv@latest
RUN go build ./cmd/accumulated-bootstrap

FROM alpine:3

# Install tools
RUN apk add --no-cache bash jq curl nano

# Copy binaries
COPY --from=build /root/accumulated-bootstrap /go/bin/dlv /bin/

ENTRYPOINT ["accumulated-bootstrap"]
CMD ["help"]