package fluent

import (
	"context"
	"github.com/streadway/amqp"
	"log"
	"os"
)

type SinkHandlerFunc func(ctx context.Context, msg amqp.Delivery)
type TransactionHandlerFunc func(ctx context.Context, msg amqp.Delivery) (error)

type Exchange struct {
	name    string
	kind    string
	binding []string
	sink    *SinkConfig
	attrs   amqp.Table
}

type SinkConfig struct {
	handler      SinkHandlerFunc
	queueName    string
	bindings     []*Exchange
	middleware   []ReceiverHandler
	broker       *Server
	autoAck      bool
	deadQueue    string
	deadExchange string
	attrs        amqp.Table
	retries      int
}

func newSink(queue string, broker *Server) *SinkConfig {
	return &SinkConfig{
		queueName: queue,
		broker:    broker,
		autoAck:   true,
		retries:   10,
	}
}

func (snk *SinkConfig) ManualAck() *SinkConfig {
	snk.autoAck = false
	return snk
}

func (snk *SinkConfig) Use(handler ReceiverHandler) *SinkConfig {
	snk.middleware = append(snk.middleware, handler)
	return snk
}

func (snk *SinkConfig) Validate(certFile string) *SinkConfig {
	mv, err := NewCertValidatorFromFile(certFile, DefaultSignatureHeader, log.New(os.Stderr, "[validator] ", log.LstdFlags))
	if err != nil {
		panic(err)
	}
	return snk.Use(mv)
}

func (snk *SinkConfig) deadLetterQueue(name string) *SinkConfig {
	snk.deadQueue = name
	return snk.Attr("x-dead-letter-routing-key", name)
}

func (snk *SinkConfig) deadLetterExchange(name string) *SinkConfig {
	snk.deadExchange = name
	return snk.Attr("x-dead-letter-exchange", name)
}

func (snk *SinkConfig) Lazy() *SinkConfig {
	return snk.Attr("x-queue-mode", "lazy")
}

func (snk *SinkConfig) Retries(count int) *SinkConfig {
	snk.retries = count
	return snk
}

func (snk *SinkConfig) DeadLetter(exchange, routingKey string) *SinkConfig {
	return snk.deadLetterExchange(exchange).deadLetterQueue(routingKey)
}

func (snk *SinkConfig) Attr(name string, value interface{}) *SinkConfig {
	if snk.attrs == nil {
		snk.attrs = make(amqp.Table)
	}
	snk.attrs[name] = value
	return snk
}

func (snk *SinkConfig) exchange(name string, kind string) *Exchange {
	exch := &Exchange{
		kind: kind,
		name: name,
		sink: snk,
	}
	snk.bindings = append(snk.bindings, exch)
	return exch
}

func (snk *SinkConfig) Topic(name string) *Exchange {
	return snk.exchange(name, "topic")
}

func (snk *SinkConfig) Direct(name string) *Exchange {
	return snk.exchange(name, "direct")
}

func (snk *SinkConfig) Fanout(name string) *Exchange {
	return snk.exchange(name, "fanout")
}

func (exc *Exchange) Key(routingKey string) *Exchange {
	exc.binding = append(exc.binding, routingKey)
	return exc
}

func (exc *Exchange) Topic(name string) *Exchange {
	return exc.sink.exchange(name, "topic")
}

func (exc *Exchange) Direct(name string) *Exchange {
	return exc.sink.exchange(name, "direct")
}

func (exc *Exchange) Fanout(name string) *Exchange {
	return exc.sink.exchange(name, "fanout")
}

func (exc *Exchange) HandlerFunc(fn SinkHandlerFunc) *Server {
	exc.sink.handler = fn
	exc.sink.broker.handle(&sink{*exc.sink})
	// help gc
	brk := exc.sink.broker
	exc.sink.broker = nil
	exc.sink = nil
	return brk
}

func (exc *Exchange) TransactFunc(fn TransactionHandlerFunc) *Server {
	exc.sink.ManualAck()
	return exc.HandlerFunc(func(ctx context.Context, msg amqp.Delivery) {
		err := fn(ctx, msg)
		if err != nil {
			msg.Nack(false, true)
		} else {
			msg.Ack(false)
		}
	})
}

func (exc *Exchange) Attr(name string, value interface{}) *Exchange {
	if exc.attrs == nil {
		exc.attrs = make(amqp.Table)
	}
	exc.attrs[name] = value
	return exc
}

type sink struct {
	config SinkConfig
}

func (s *sink) ChannelReady(ctx context.Context, ch *amqp.Channel) error {
	if s.config.deadExchange != "" {
		if err := ch.ExchangeDeclare(s.config.deadExchange, "fanout", true, false, false, false, nil); err != nil {
			return err
		}
	}

	if s.config.deadQueue != "" {
		_, err := ch.QueueDeclare(s.config.deadQueue, true, false, false, false, nil)
		if err != nil {
			return err
		}
	}

	if s.config.deadExchange != "" && s.config.deadQueue != "" {
		if err := ch.QueueBind(s.config.deadQueue, "", s.config.deadExchange, false, nil); err != nil {
			return err
		}
	}

	queueName, err := ch.QueueDeclare(s.config.queueName, s.config.queueName != "", s.config.queueName == "", false, false, s.config.attrs)
	if err != nil {
		return err
	}

	for _, exc := range s.config.bindings {
		if err := ch.ExchangeDeclare(exc.name, exc.kind, true, false, false, false, exc.attrs); err != nil {
			return err
		}
		for _, routingKey := range exc.binding {
			if err := ch.QueueBind(queueName.Name, routingKey, exc.name, false, nil); err != nil {
				return err
			}
		}
	}

	stream, err := ch.Consume(queueName.Name, "", s.config.autoAck, false, false, false, nil)
	if err != nil {
		return err
	}
LOOP:
	for {
		select {
		case msg, ok := <-stream:
			if !ok {
				break LOOP
			}
			if s.config.retries >= 0 && getRedelivery(&msg) > int64(s.config.retries) {
				// too much retries
				err = msg.Nack(false, false)
				if err != nil {
					return err
				}
				continue
			}
			msg.RoutingKey = restoreRoutingKey(&msg)

			filtered := false
			for _, handler := range s.config.middleware {
				if !handler.Handle(&msg) {
					filtered = true
					err = msg.Nack(false, false)
					if err != nil {
						return err
					}
					break
				}
			}
			if !filtered {
				s.config.handler(ctx, msg)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func getRedelivery(msg *amqp.Delivery) int64 {
	var redelivered int64
	if xDeathA, ok := msg.Headers["x-death"]; ok {
		// X-death is an array
		if xDeathA, ok := xDeathA.([]interface{}); ok && len(xDeathA) > 0 {
			xDeath := xDeathA[0]
			// This must be a table
			if table, ok := xDeath.(amqp.Table); ok {
				if count, ok := table["count"]; ok {
					// count must be a int
					if iCount, ok := count.(int64); ok {
						redelivered = iCount
					}
				}
			}
		}
	}
	return redelivered
}

func restoreRoutingKey(msg *amqp.Delivery) string {
	if xDeathA, ok := msg.Headers["x-death"]; ok {
		// X-death is an array
		if xDeathA, ok := xDeathA.([]interface{}); ok && len(xDeathA) > 0 {
			xDeath := xDeathA[0]
			// This must be a table
			if table, ok := xDeath.(amqp.Table); ok {
				if routingKeys, ok := table["routing-keys"]; ok {
					// routingKeys must be a array
					if array, ok := routingKeys.([]interface{}); ok && len(array) > 0 {
						// must be string
						if rk, ok := array[0].(string); ok {
							return rk
						}
					}
				}
			}
		}
	}
	return msg.RoutingKey
}
