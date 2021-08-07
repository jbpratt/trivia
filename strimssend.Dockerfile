FROM docker.io/library/golang:1.16-buster as build

ENV GO111MODULE=on
WORKDIR /go/src/bots
ADD . /go/src/bots

RUN go get -d -v ./...
RUN go build -o /go/bin/strimssend ./cmd/strimssend/

FROM gcr.io/distroless/base-debian10
COPY --from=build /go/bin/strimssend /
CMD ["/strimssend"]
