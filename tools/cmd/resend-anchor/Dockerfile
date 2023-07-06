FROM golang:1.20 as build

# Build
WORKDIR /root
COPY . .
ENV CGO_ENABLED 0
RUN go install ./tools/cmd/resend-anchor
RUN go install github.com/go-delve/delve/cmd/dlv@latest

FROM alpine:3

# Install tools
RUN apk add --no-cache bash curl

# Copy binaries
COPY --from=build /go/bin/resend-anchor /go/bin/dlv /bin/

ENTRYPOINT ["resend-anchor"]
CMD ["heal", "--help"]