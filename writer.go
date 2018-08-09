package fluent

import (
	"github.com/streadway/amqp"
	"context"
	"encoding/json"
	"time"
	"strconv"
)

const DefaultSignatureHeader = "X-Signature"

type WriterConfig struct {
	exchange   string
	kind       string
	middleware []SenderHandler
	topic      string
	broker     *Server
}

func newWriter(broker *Server) *WriterConfig {
	return &WriterConfig{broker: broker}
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

func (wc *WriterConfig) DefaultTopic(name string) *WriterConfig  { return wc.withExchange(name, "topic") }
func (wc *WriterConfig) DefaultDirect(name string) *WriterConfig { return wc.withExchange(name, "direct") }
func (wc *WriterConfig) DefaultFanout(name string) *WriterConfig { return wc.withExchange(name, "fanout") }

func (wc *WriterConfig) DefaultKey(routingKey string) *WriterConfig {
	wc.topic = routingKey
	return wc
}

func (wc *WriterConfig) Create() *Writer {
	pub := &publisher{
		config: *wc,
		stream: make(chan msg),
	}
	wc.broker.handle(pub)
	return &Writer{
		pub: pub,
		ctx: wc.broker.config.ctx,
	}
}

type msg struct {
	exchange string
	key      string
	msg      amqp.Publishing
	try      chan error
}

type publisher struct {
	config  WriterConfig
	stream  chan msg
	lastMsg *msg
}

func (pub *publisher) ChannelReady(ctx context.Context, ch *amqp.Channel) error {
	if pub.config.exchange != "" {
		if err := ch.ExchangeDeclare(pub.config.exchange, pub.config.kind, true, false, false, false, nil); err != nil {
			return err
		}
	}
	for {
		if pub.lastMsg != nil {
			err := ch.Publish(pub.lastMsg.exchange, pub.lastMsg.key, false, false, pub.lastMsg.msg)
			if pub.lastMsg.try != nil {
				pub.lastMsg.try <- err
				close(pub.lastMsg.try)
				pub.lastMsg = nil
			}
			if err != nil {
				return err
			}
			pub.lastMsg = nil
		}
		select {
		case msg := <-pub.stream:
			pub.lastMsg = &msg
		case <-ctx.Done():
			return ctx.Err()
		}
	}

}

type Writer struct {
	pub *publisher
	ctx context.Context
}

func (writer *Writer) Prepare() *Message {
	return &Message{
		exchange: writer.pub.config.exchange,
		key:      writer.pub.config.topic,
		writer:   writer,
	}
}

func (writer *Writer) Reply(msg *amqp.Delivery) *Message {
	ms := &Message{
		exchange: "",
		key:      msg.ReplyTo,
		writer:   writer,
	}

	ms.msg.CorrelationId = msg.CorrelationId
	return ms
}

type Message struct {
	msg      amqp.Publishing
	exchange string
	key      string
	writer   *Writer
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

func (msg *Message) Reply(correlationId, queueName string) *Message {
	msg.msg.CorrelationId = correlationId
	msg.msg.ReplyTo = queueName
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

func (msg *Message) TTL(tm time.Duration) *Message {
	msg.msg.Expiration = strconv.FormatInt(int64(tm/time.Millisecond), 10)
	return msg
}

func (ms *Message) Send() {
	m := msg{msg: ms.msg, key: ms.key, exchange: ms.exchange}
	if !ms.processMessage(&m.msg) {
		return
	}

	select {
	case ms.writer.pub.stream <- m:
	case <-ms.writer.ctx.Done():
	}
}

func (ms *Message) TrySend() error {
	t := make(chan error, 1)
	m := msg{msg: ms.msg, key: ms.key, exchange: ms.exchange, try: t}
	if !ms.processMessage(&m.msg) {
		return nil
	}
	select {
	case ms.writer.pub.stream <- m:
		return <-t
	case <-ms.writer.ctx.Done():
		return ms.writer.ctx.Err()
	}
}

func (ms *Message) processMessage(msg *amqp.Publishing) (bool) {
	for _, handle := range ms.writer.pub.config.middleware {
		ok := handle.Handle(msg)
		if !ok {
			return false
		}
	}
	return true
}
