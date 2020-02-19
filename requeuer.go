package fluent

import (
	"context"
	"github.com/streadway/amqp"
	"strconv"
	"time"
)

const LastErrorHeader = "x-fluent-amqp-last-message-error"

type ReQueueConfig struct {
	queueName   string
	targetQueue string
	timeout     time.Duration
	broker      *Server
}

func newRequeue(srv *Server, targetQueue string) *ReQueueConfig {
	return &ReQueueConfig{
		queueName:   targetQueue + "/requeue",
		targetQueue: targetQueue,
		timeout:     10 * time.Second,
		broker:      srv,
	}
}

func (rq *ReQueueConfig) Timeout(tm time.Duration) *ReQueueConfig {
	rq.timeout = tm
	return rq
}

func (rq *ReQueueConfig) Queue(name string) *ReQueueConfig {
	rq.queueName = name
	return rq
}

func (rq *ReQueueConfig) Create() Requeue {
	r := &requeue{
		globalCtx: rq.broker.config.ctx,
		config:    *rq,
		messages:  make(chan requeueMessage, 1),
	}
	rq.broker.handle(r)
	return r
}

type requeueMessage struct {
	msg  *amqp.Publishing
	done chan error
}

type requeue struct {
	config    ReQueueConfig
	messages  chan requeueMessage
	globalCtx context.Context
}

type Requeue interface {
	Requeue(original *amqp.Delivery) error
	RequeueWithError(original *amqp.Delivery, err error) error
}

func (s *requeue) ChannelReady(ctx context.Context, ch *amqp.Channel) error {
	args := amqp.Table{
		"x-dead-letter-routing-key": s.config.targetQueue,
		"x-dead-letter-exchange":    "",
	}
	queue, err := ch.QueueDeclare(s.config.queueName, s.config.queueName != "", s.config.queueName == "", false, false, args)
	if err != nil {
		return err
	}

	for {
		select {
		case msg := <-s.messages:
			err := ch.Publish("", queue.Name, false, false, *msg.msg)
			msg.done <- err
			close(msg.done)
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *requeue) Requeue(original *amqp.Delivery) error {
	m := requeueMessage{
		done: make(chan error, 1),
		msg: &amqp.Publishing{
			CorrelationId:   original.CorrelationId,
			ReplyTo:         original.ReplyTo,
			Body:            original.Body,
			Expiration:      strconv.FormatInt(int64(s.config.timeout/time.Millisecond), 10),
			ContentType:     original.ContentType,
			Headers:         original.Headers,
			ContentEncoding: original.ContentEncoding,
			MessageId:       original.MessageId,
			Timestamp:       original.Timestamp,
			Type:            original.Type,
		},
	}
	select {
	case <-s.globalCtx.Done():
		return s.globalCtx.Err()
	case s.messages <- m:
		select {
		case <-s.globalCtx.Done():
			return s.globalCtx.Err()
		case err := <-m.done:
			return err
		}
	}
}

func (s *requeue) RequeueWithError(original *amqp.Delivery, err error) error {
	if err != nil {
		original.Headers[LastErrorHeader] = err.Error()
	}
	return s.Requeue(original)
}
