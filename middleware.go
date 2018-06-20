package fluent

import "github.com/streadway/amqp"

type SenderHandler interface {
	Handle(msg *amqp.Publishing) (bool)
}

type ReceiverHandler interface {
	Handle(msg *amqp.Delivery) (bool)
}
