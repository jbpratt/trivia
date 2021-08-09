FROM docker.io/library/golang:1.16-buster as build

ARG BOT

ENV GO111MODULE=on
WORKDIR /go/src/bots
ADD . /go/src/bots

RUN go get -d -v ./...
RUN go build -o /go/bin/bot ./cmd/${BOT}/

FROM gcr.io/distroless/base-debian10
COPY --from=build /go/bin/bot /
ENTRYPOINT ["/bot"]
