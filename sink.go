package fluent

import (
	"context"
	"github.com/streadway/amqp"
	"log"
	"os"
	"time"
)

type SinkHandlerFunc func(ctx context.Context, msg amqp.Delivery)
type TransactionHandlerFunc func(ctx context.Context, msg amqp.Delivery) (error)

// Handler for messages that reached retries amount. False return means request to drop message
type SinkExpiredHandlerFunc func(ctx context.Context, msg amqp.Delivery, retries int64) bool

type TransactionHandler interface {
	Handle(ctx context.Context, msg amqp.Delivery) (error)
}

type SimpleHandler interface {
	Handle(ctx context.Context, msg amqp.Delivery)
}

type Exchange struct {
	name    string
	kind    string
	binding []string
	sink    *SinkConfig
	attrs   amqp.Table
}

type SinkConfig struct {
	name                   string
	handler                SinkHandlerFunc
	queueName              string
	bindings               []*Exchange
	middleware             []ReceiverHandler
	broker                 *Server
	autoAck                bool
	deadQueue              string
	deadExchange           string
	attrs                  amqp.Table
	retries                int
	expiredMessagesHandler SinkExpiredHandlerFunc
	tooMuchRetries         struct {
		handler   SinkExpiredHandlerFunc
		threshold int64
	}

	rqConfig *ReQueueConfig
	requeue  Requeue
}

func newSink(queue string, broker *Server) *SinkConfig {
	snk := &SinkConfig{
		queueName: queue,
		broker:    broker,
		autoAck:   true,
		retries:   broker.config.defaultSink.retries,
	}
	if broker.config.defaultSink.expiredMessagesHandler != nil {
		snk = snk.OnExpired(func(ctx context.Context, msg amqp.Delivery, retries int64) bool {
			return broker.config.defaultSink.expiredMessagesHandler(ctx, snk.name, msg, retries)
		})
	}
	if broker.config.defaultSink.tooMuchRetries.handler != nil {
		snk = snk.OnTooMuchRetries(broker.config.defaultSink.tooMuchRetries.threshold, func(ctx context.Context, msg amqp.Delivery, retries int64) bool {
			return broker.config.defaultSink.tooMuchRetries.handler(ctx, snk.name, msg, retries)
		})
	}
	return snk
}

// Name of sink. Just a tag fo future identity (like in default handlers)
func (snk *SinkConfig) Name(name string) *SinkConfig {
	snk.name = name
	return snk
}

// Handler for expired (reached retries limit) messages
func (snk *SinkConfig) OnExpired(handler SinkExpiredHandlerFunc) *SinkConfig {
	snk.expiredMessagesHandler = handler
	return snk
}

// Handler for messages that reached defined threshold retires limit. Threshold can be less
// (in case of positive retries limit) or equal to retries limit and will executes before OnExpired (if applicable).
// The handler will be invoked each times after threshold limit. If handler returns false, message is dropped.
// In case when threshold is same as retries, requires at least one (from OnExpired or from OnTooMuschRetries) false to drop message.
func (snk *SinkConfig) OnTooMuchRetries(threshold int64, handler SinkExpiredHandlerFunc) *SinkConfig {
	snk.tooMuchRetries.handler = handler
	snk.tooMuchRetries.threshold = threshold
	return snk
}

func (snk *SinkConfig) Requeue(interval time.Duration) *SinkConfig {
	snk.rqConfig = snk.broker.Requeue(snk.queueName).Timeout(interval)
	return snk
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

func (snk *SinkConfig) KeepDead() *SinkConfig {
	return snk.DeadLetter("", snk.queueName+"/dead")
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

func (snk *SinkConfig) HandlerFunc(fn SinkHandlerFunc) *Server {
	snk.handler = fn
	snk.broker.handle(&sink{*snk})
	if snk.rqConfig != nil {
		snk.requeue = snk.rqConfig.Create()
	}
	return snk.broker
}

func (snk *SinkConfig) Handler(obj SimpleHandler) *Server {
	return snk.HandlerFunc(obj.Handle)
}

func (snk *SinkConfig) TransactFunc(fn TransactionHandlerFunc) *Server {
	snk.ManualAck()
	return snk.HandlerFunc(func(ctx context.Context, msg amqp.Delivery) {
		snk.broker.config.logger.Println(snk.name, "processing", msg.MessageId, "in transaction")
		start := time.Now()
		err := fn(ctx, msg)
		end := time.Now()
		if err == nil {
			snk.broker.config.logger.Println(snk.name, "message", msg.MessageId, "successfully processed within", end.Sub(start))
		} else {
			snk.broker.config.logger.Println(snk.name, "message", msg.MessageId, "failed after", end.Sub(start), ":", err)
		}
		if err == nil {
			msg.Ack(false)
		} else if snk.requeue == nil {
			// no requeue
			msg.Nack(false, true)
		} else if err = snk.requeue.Requeue(&msg); err == nil {
			// requeue exists and it's OK
			msg.Ack(false)
		} else {
			// requeue exists but failed
			msg.Nack(false, true)
		}
	})
}

func (snk *SinkConfig) Transact(fn TransactionHandler) *Server { return snk.TransactFunc(fn.Handle) }

func (exc *Exchange) Key(routingKeys ...string) *Exchange {
	for _, key := range routingKeys {
		exc.binding = append(exc.binding, key)
	}

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
	return exc.sink.HandlerFunc(fn)
}

func (exc *Exchange) Handler(obj SimpleHandler) *Server {
	return exc.HandlerFunc(obj.Handle)
}

func (exc *Exchange) TransactFunc(fn TransactionHandlerFunc) *Server {
	return exc.sink.TransactFunc(fn)
}

func (exc *Exchange) Transact(fn TransactionHandler) *Server { return exc.TransactFunc(fn.Handle) }

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
			retries := getRedelivery(&msg)
			var keepMessage = true

			if s.config.tooMuchRetries.handler != nil && retries >= s.config.tooMuchRetries.threshold {
				// too much retries
				keepMessage = s.config.tooMuchRetries.handler(ctx, msg, retries)
			}

			if s.config.retries >= 0 && retries > int64(s.config.retries) {
				// expired
				if s.config.expiredMessagesHandler != nil {
					keepMessage = keepMessage && s.config.expiredMessagesHandler(ctx, msg, retries)
				} else {
					keepMessage = false
				}
			}
			if !keepMessage {
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
