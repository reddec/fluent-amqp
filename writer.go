package fluent

import (
	"context"
	"encoding/json"
	"github.com/pkg/errors"
	"github.com/reddec/fluent-amqp/internal"
	"github.com/streadway/amqp"
	"strconv"
	"time"
)

const DefaultSignatureHeader = "X-Signature"

type WriterConfig struct {
	exchange   string
	kind       string
	middleware []SenderHandler
	topic      string
	broker     *Server
	overflow   struct {
		MaxSize int
		Handler func(size int)
	}
}

func newWriter(broker *Server) *WriterConfig {
	return &WriterConfig{broker: broker, exchange: "", kind: "direct"}
}

func (wc *WriterConfig) withExchange(name, kind string) *WriterConfig {
	wc.exchange = name
	wc.kind = kind
	return wc
}

func (wc *WriterConfig) Use(handler SenderHandler) *WriterConfig {
	wc.middleware = append(wc.middleware, handler)
	return wc
}

// Sign body and add signature to DefaultSignatureHeader header. Panic if private key couldn't be read
func (wc *WriterConfig) Sign(privateFile string) *WriterConfig {
	mw, err := NewSignerFromFile(privateFile, DefaultSignatureHeader)
	if err != nil {
		panic(err)
	}
	return wc.Use(mw)
}

func (wc *WriterConfig) DefaultTopic(name string) *WriterConfig { return wc.withExchange(name, "topic") }
func (wc *WriterConfig) DefaultDirect(name string) *WriterConfig {
	return wc.withExchange(name, "direct")
}
func (wc *WriterConfig) DefaultFanout(name string) *WriterConfig {
	return wc.withExchange(name, "fanout")
}

func (wc *WriterConfig) DefaultKey(routingKey string) *WriterConfig {
	wc.topic = routingKey
	return wc
}

func (wc *WriterConfig) Overflow(maxQueueSize int, handler func(int)) *WriterConfig {
	wc.overflow.Handler = handler
	wc.overflow.MaxSize = maxQueueSize
	return wc
}

func (wc *WriterConfig) Create() *Writer {
	pub := &publisher{
		config: *wc,
	}
	wc.broker.handle(pub)
	return &Writer{
		pub: pub,
		ctx: wc.broker.config.ctx,
	}
}

type publisher struct {
	config WriterConfig
	stream internal.MessageQueue
}

func (pub *publisher) ChannelReady(ctx context.Context, ch *amqp.Channel) error {
	if pub.config.exchange != "" {
		if err := ch.ExchangeDeclare(pub.config.exchange, pub.config.kind, true, false, false, false, nil); err != nil {
			return err
		}
	}
	var overflow bool
	for {
		n := pub.stream.Len()
		if n < pub.config.overflow.MaxSize {
			overflow = false
		} else if !overflow && pub.config.overflow.Handler != nil {
			overflow = true
			pub.config.overflow.Handler(n)
		}
		msg, err := pub.stream.Peek(ctx)
		if err != nil {
			return err
		}
		err = ch.Publish(msg.Exchange, msg.Key, false, false, msg.Msg)
		if msg.Try != nil {
			msg.Try <- err
			close(msg.Try)
			pub.stream.Commit() // try-only should be discarded after first attempt
		}
		if err != nil {
			return err
		}
		if msg.Done != nil {
			close(msg.Done)
		}
		pub.stream.Commit()
	}

}

type Writer struct {
	pub *publisher
	ctx context.Context
}

func (writer *Writer) Prepare() *Message {
	msg := &Message{
		exchange: writer.pub.config.exchange,
		key:      writer.pub.config.topic,
		writer:   writer,
	}
	msg.msg.Timestamp = time.Now().UTC()
	msg.msg.DeliveryMode = amqp.Persistent
	return msg
}

func (writer *Writer) Reply(msg *amqp.Delivery) *Message {
	ms := writer.Prepare()
	ms.exchange = ""
	ms.key = msg.ReplyTo
	ms.msg.CorrelationId = msg.CorrelationId
	return ms
}

type Message struct {
	msg      amqp.Publishing
	exchange string
	key      string
	writer   *Writer
}

func (msg *Message) Raw() *amqp.Publishing {
	return &msg.msg
}

func (msg *Message) Time(stamp time.Time) *Message {
	msg.msg.Timestamp = stamp
	return msg
}

func (msg *Message) ID(id string) *Message {
	msg.msg.MessageId = id
	return msg
}

func (msg *Message) Exchange(name string) *Message {
	msg.exchange = name
	return msg
}

func (msg *Message) Header(name string, data interface{}) *Message {
	if msg.msg.Headers == nil {
		msg.msg.Headers = make(amqp.Table)
	}
	msg.msg.Headers[name] = data
	return msg
}

func (msg *Message) Type(contentType string) *Message {
	msg.msg.ContentType = contentType
	return msg
}

func (msg *Message) Key(name string) *Message {
	msg.key = name
	return msg
}

func (msg *Message) ReplyTo(correlationId, queueName string) *Message {
	msg.msg.CorrelationId = correlationId
	msg.msg.ReplyTo = queueName
	return msg
}

func (msg *Message) Reply(correlationId, queueName string) *Message {
	msg.msg.CorrelationId = correlationId
	msg.exchange = ""
	msg.key = queueName
	return msg
}

func (msg *Message) String(content string) *Message {
	msg.msg.Body = []byte(content)
	return msg
}

func (msg *Message) Bytes(content []byte) *Message {
	cp := make([]byte, len(content))
	copy(cp, content)
	msg.msg.Body = cp
	return msg
}

func (msg *Message) JSON(obj interface{}) *Message {
	v, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		panic(err)
	}
	msg.msg.Body = v
	msg.msg.ContentType = "application/json"
	return msg
}

func (msg *Message) ContentType(mime string) *Message {
	msg.msg.ContentType = mime
	return msg
}

func (msg *Message) JSONContent(data []byte) *Message {
	return msg.ContentType("application/json").Bytes(data)
}

func (msg *Message) TTL(tm time.Duration) *Message {
	if tm != 0 {
		msg.msg.Expiration = strconv.FormatInt(int64(tm/time.Millisecond), 10)
	}
	return msg
}

func (ms *Message) Publish(ctx context.Context) (<-chan struct{}, error) {
	done := make(chan struct{})
	m := &internal.Message{Msg: ms.msg, Key: ms.key, Exchange: ms.exchange, Done: done}
	if !ms.processMessage(&m.Msg) {
		close(done)
		return done, errors.New("rejected by middleware")
	}

	ms.writer.pub.stream.Put(m)
	return m.Done, nil
}

func (ms *Message) PublishWait(ctx context.Context) error {
	ch, err := ms.Publish(ctx)
	if err != nil {
		return err
	}
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (ms *Message) SendContext(ctx context.Context) error {
	_, err := ms.Publish(ctx)
	return err
}

func (ms *Message) Send() <-chan struct{} {
	ch, _ := ms.Publish(context.Background())
	return ch
}

func (ms *Message) TrySend() error {
	t := make(chan error, 1)
	m := &internal.Message{Msg: ms.msg, Key: ms.key, Exchange: ms.exchange, Try: t}
	if !ms.processMessage(&m.Msg) {
		return nil
	}
	ms.writer.pub.stream.Put(m)
	select {
	case err := <-t:
		return err
	case <-ms.writer.ctx.Done():
		return ms.writer.ctx.Err()
	}
}

func (ms *Message) processMessage(msg *amqp.Publishing) bool {
	for _, handle := range ms.writer.pub.config.middleware {
		ok := handle.Handle(msg)
		if !ok {
			return false
		}
	}
	return true
}
