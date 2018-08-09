# Fluent AMQP
[![](https://godoc.org/github.com/reddec/fluent-amqp?status.svg)](https://godoc.org/github.com/reddec/fluent-amqp)

Library that provides fluent and easy wrapper over[streadway-amqp](https://github.com/streadway/amqp) API.
Adds such features like:

- Reconnectiong. Will restore all defined infrastructure
- Non-blocking processing of messages
- Optional auto-requeue (with delay)
- Signing and verifiying messages by public/private pair

[API documentation](https://godoc.org/github.com/reddec/fluent-amqp)