FROM golang:1.11-alpine3.8
RUN apk add --no-cache git
WORKDIR /go/src/github.com/reddec
RUN git clone https://github.com/reddec/fluent-amqp.git && cd fluent-amqp && git checkout v0.1.10
RUN cd fluent-amqp  && go get -d -v ./cmd/...
RUN go install -v github.com/reddec/fluent-amqp/cmd/...

FROM alpine:3.8
COPY --from=0 /go/bin/* /usr/bin/