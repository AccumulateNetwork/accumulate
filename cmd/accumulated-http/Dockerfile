FROM golang:3 as build

ARG GIT_DESCRIBE
ARG GIT_COMMIT

# Build
WORKDIR /root
COPY . .
ENV CGO_ENABLED 0
RUN make http GIT_DESCRIBE=$GIT_DESCRIBE GIT_COMMIT=$GIT_COMMIT
RUN go install github.com/go-delve/delve/cmd/dlv@latest

FROM alpine:3

# Install tools
RUN apk add --no-cache bash jq curl nano

# Copy binaries
COPY --from=build /root/accumulated-http /go/bin/dlv /bin/

ENTRYPOINT ["accumulated-http"]
CMD ["help"]