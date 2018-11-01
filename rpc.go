package fluent

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
	"sync"
	"time"
)

type RPC struct {
	lock        sync.Mutex
	correlation map[string]chan *amqp.Delivery
	publisher   *Writer
	queue       string
}

func BuildRPC(broker *Server, consumerQueue string, targetExchange, targetRoutingKey string) *RPC {
	rpc := &RPC{
		correlation: make(map[string]chan *amqp.Delivery),
		publisher:   broker.Publisher().DefaultDirect(targetExchange).DefaultKey(targetRoutingKey).Create(),
		queue:       consumerQueue,
	}
	broker.Sink(consumerQueue).HandlerFunc(rpc.onMessage)
	go func() {
		broker.WaitToFinish()
		rpc.lock.Lock()
		for _, ch := range rpc.correlation {
			close(ch)
		}
		rpc.lock.Unlock()
	}() // release all reasources
	return rpc
}

func (rpc *RPC) onMessage(ctx context.Context, msg amqp.Delivery) {
	rpc.lock.Lock()
	ch, ok := rpc.correlation[msg.CorrelationId]
	if ok {
		delete(rpc.correlation, msg.CorrelationId)
	}
	rpc.lock.Unlock()
	if ok {
		ch <- &msg
		close(ch)
	}
}

func (rpc *RPC) RequestRaw(msg *amqp.Publishing) *Reply {
	msg.CorrelationId = uuid.New().String()
	msg.MessageId = msg.CorrelationId
	msg.ReplyTo = rpc.queue
	msg.Timestamp = time.Now()
	ms := rpc.publisher.Prepare()
	ms.msg = *msg
	reply := &Reply{
		ch: make(chan *amqp.Delivery, 1),
	}
	rpc.lock.Lock()
	rpc.correlation[msg.CorrelationId] = reply.ch
	rpc.lock.Unlock()
	<-ms.Send()
	return reply
}

func (rpc *RPC) RequestJSON(obj interface{}) *Reply {
	data, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return rpc.RequestRaw(&amqp.Publishing{
		Body:        data,
		ContentType: "application/json",
	})
}

type Reply struct {
	id   string
	ch   chan *amqp.Delivery
	data *amqp.Delivery
}

func (rp *Reply) ID() string { return rp.id }

func (rp *Reply) Data() chan<- *amqp.Delivery { return rp.ch }

func (rp *Reply) WaitJSON(target interface{}) error {
	msg := <-rp.ch
	if msg == nil {
		return errors.New("closed channel")
	}
	return json.Unmarshal(msg.Body, target)
}
