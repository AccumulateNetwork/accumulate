FROM golang:1.13.1 AS builder

ARG GOBIN=/go/bin/
ARG GOOS=linux
ARG GOARCH=amd64
ARG GOPATH=$HOME/go
ARG CGO_ENABLED=0
ARG PKG_NAME=github.com/AccumulateNetwork/accumulated
ARG PKG_PATH=${GOPATH}/src/${PKG_NAME}

WORKDIR ${PKG_PATH}
COPY . ${PKG_PATH}/

RUN go mod download
RUN go build -o /go/bin/accumulated main.go

FROM alpine:3.7

RUN set -xe && \
  apk --no-cache add bash ca-certificates inotify-tools && \
  addgroup -g 1000 app && \
  adduser -D -G app -u 1000 app

WORKDIR /home/app

COPY --from=builder /go/bin/accumulated ./
COPY ./entrypoint.sh ./entrypoint.sh

RUN \
  mkdir ./values && \
  chown -R app:app /home/app

USER app

EXPOSE 25999 26000

CMD [ "./accumulated" ]
#CMD [ "./accumulated", "-c", "/home/app/values/config.yaml" ]