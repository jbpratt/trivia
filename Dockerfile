FROM	docker.io/library/golang:1.24-bookworm	as	build
LABEL org.opencontainers.image.source https://github.com/jbpratt/bots

ENV	GO111MODULE	on

WORKDIR	/go/src/bots
COPY go.mod go.sum	./
COPY cmd/ cmd/
COPY internal/ internal/

RUN go mod download
RUN CGO_ENABLED=1 go build -o /go/bin/bot ./cmd/triviabot/

FROM	gcr.io/distroless/base-debian12:latest
COPY	--from=build /go/bin/bot	/
ENTRYPOINT	["/bot"]
