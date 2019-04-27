FROM golang:1.11-alpine3.8
RUN apk add --no-cache git dep
WORKDIR /go/src/github.com/reddec/fluent-amqp
ADD . ./
RUN dep ensure -v
RUN CGO_ENABLED=0 ls ./cmd/ | xargs -n 1 -i go build -v  -ldflags "-s -w" ./cmd/'{}'
RUN mkdir -p dist && cp $(ls cmd/) dist/

FROM alpine:3.8
COPY --from=0 /go/src/github.com/reddec/fluent-amqp/dist/* /usr/bin/