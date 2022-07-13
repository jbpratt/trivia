FROM	docker.io/library/golang:1.18-buster	as	build
MAINTAINER	jbpratt <jbpratt78@gmail.com>
LABEL org.opencontainers.image.source https://github.com/jbpratt/bots

ENV	GO111MODULE	on

WORKDIR	/go/src/bots
COPY go.mod go.sum	.
COPY cmd/ cmd/
COPY internal/ internal/

RUN go mod download
RUN CGO_ENABLED=1 go build -o /go/bin/bot ./cmd/triviabot/

FROM	gcr.io/distroless/base-debian11:latest
COPY	--from=build /go/bin/bot	/
ENTRYPOINT	["/bot"]
