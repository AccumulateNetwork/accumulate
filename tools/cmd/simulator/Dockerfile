FROM golang:1.23 as build

ARG GIT_DESCRIBE
ARG GIT_COMMIT

# Build
WORKDIR /root
COPY . .
ENV CGO_ENABLED 0
RUN make sim GIT_DESCRIBE=$GIT_DESCRIBE GIT_COMMIT=$GIT_COMMIT
RUN go install github.com/go-delve/delve/cmd/dlv@latest
RUN mkdir /data

FROM alpine:3

# Install tools
RUN apk add --no-cache bash jq curl nano

# Copy binaries
COPY --from=build /root/simulator /go/bin/dlv /bin/

EXPOSE 26660

ENTRYPOINT ["simulator", "-b=1", "-v=1", "-s=100ms", "--database=/data/accumulate.db", "--globals={\"executorVersion\": \"v2vandenberg\"}"]
CMD ["-a=0.0.0.0"]