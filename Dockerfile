FROM golang:1.11-rc-alpine as build

RUN apk add --no-cache git=2.18.1-r0

WORKDIR /src
COPY go.mod go.sum *.go ./
RUN go get -d -v ./...
RUN CGO_ENABLED=0 go build -ldflags "-s -w"
RUN mkdir -p /tmp/badger

FROM scratch

COPY --from=build /src/store /store
COPY --from=build /tmp/badger /tmp/badger

ENTRYPOINT ["/store"]
