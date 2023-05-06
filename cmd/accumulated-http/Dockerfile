FROM golang:1.19 as build

# Build
WORKDIR /root
COPY . .
ENV CGO_ENABLED 0
RUN go install github.com/go-delve/delve/cmd/dlv@latest
RUN go build ./cmd/accumulated-http

FROM alpine:3

# Install tools
RUN apk add --no-cache bash jq curl nano

# Copy binaries
COPY --from=build /root/accumulated-http /go/bin/dlv /bin/

ENTRYPOINT ["accumulated-http"]
CMD ["help"]