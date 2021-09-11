FROM	docker.io/library/golang:1.16-buster	as	build
MAINTAINER	jbpratt <jbpratt78@gmail.com>
LABEL org.opencontainers.image.source https://github.com/jbpratt/bots

ARG	BOT
ENV	GO111MODULE	on

WORKDIR	/go/src/bots
COPY	.	/go/src/bots

RUN go get -d -v ./...

RUN CGO_ENABLED=1 go build -o /go/bin/bot ./cmd/${BOT}/

FROM	gcr.io/distroless/base-debian10:latest
COPY	--from=build /go/bin/bot	/
ENTRYPOINT	["/bot"]
