# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /consensus-testnet ./cmd/consensus-testnet

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /consensus-testnet /usr/local/bin/consensus-testnet

ENTRYPOINT ["/usr/local/bin/consensus-testnet"]
