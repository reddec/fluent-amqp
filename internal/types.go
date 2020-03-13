package internal

import "github.com/streadway/amqp"

type Message struct {
	Exchange string
	Key      string
	Msg      amqp.Publishing
	Try      chan error
	Done     chan struct{}
}
