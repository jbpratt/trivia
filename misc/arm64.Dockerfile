FROM	docker.io/library/golang:1.16-buster	as	build
MAINTAINER	jbpratt <jbpratt78@gmail.com>

ARG	BOT
ENV	GO111MODULE	on

WORKDIR	/go/src/bots
COPY	.	/go/src/bots

RUN	: \
	&& set -x \
	&& apt-get -y update \
	&& apt-get install -y \
	gcc-arm-linux-gnueabi \
	&& go get -d -v ./... \
	&& CC=arm-linux-gnueabi-gcc CGO_ENABLED=1 GOOS=linux GOARCH=arm go build -ldflags '-extldflags "-static"' -o /go/bin/bot ./cmd/${BOT}/ \
	&& :

FROM	gcr.io/distroless/base-debian10:latest-arm64
COPY	--from=build /go/bin/bot	/
ENTRYPOINT	["/bot"]
