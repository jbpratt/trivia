FROM	docker.io/library/golang:1.16-buster	as	build
MAINTAINER	jbpratt <jbpratt78@gmail.com>

ARG	BOT
ENV	GO111MODULE	on

WORKDIR	/go/src/bots
COPY	.	/go/src/bots

RUN	: \
	&& set -x \
	&& go get -d -v ./... \
	&& go build -o /go/bin/bot ./cmd/${BOT}/ \
	&& :

FROM	gcr.io/distroless/base-debian10
COPY	--from=build /go/bin/bot	/
ENTRYPOINT	["/bot"]
