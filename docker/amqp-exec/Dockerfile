FROM golang:1.11-alpine3.8
RUN apk add --no-cache git dep
WORKDIR /go/src/github.com/reddec/fluent-amqp
ADD . ./
RUN dep ensure -v
RUN CGO_ENABLED=0 go build -v  -ldflags "-s -w" ./cmd/amqp-exec

FROM alpine:3.8
COPY --from=0  /go/src/github.com/reddec/fluent-amqp/amqp-exec /usr/bin/
ENV BROKER_URL "amqp://guest:guest@rabbitmq"
ENTRYPOINT ["/usr/bin/amqp-exec", "--"]