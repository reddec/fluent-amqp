package main

import (
	"context"
	"fmt"
	"github.com/reddec/fluent-amqp"
	"github.com/streadway/amqp"
	"log"
	"os"
	"time"
)

func main() {
	ctx := fluent.SignalContext(nil)
	broker := fluent.Broker("amqp://guest:guest@127.0.0.1").Logger(log.New(os.Stderr, "[broker] ", log.LstdFlags)).Context(ctx).Start()

	publisher := broker.Publisher().DefaultTopic("sample").DefaultKey("test").Sign("test-resources/all.pem").Create()

	requeue := broker.Requeue("test").Timeout(10 * time.Second).Create()

	broker.Sink("test").
		Validate("test-resources/all.pem").
		DeadLetter("", "deads").
		Retries(1).Lazy().
		Topic("sample").
		Key("*").
		TransactFunc(
			func(ctx context.Context, msg amqp.Delivery) error {
				println("gor")
				return requeue.Requeue(&msg)
			})

	broker.Sink("echo").TransactFunc(func(ctx context.Context, msg amqp.Delivery) error {
		fmt.Println("Hi!")
		return publisher.Reply(&msg).JSON("hello world!").PublishWait(ctx)
	})

	rpc := fluent.BuildRPC(broker, "bot", "", "echo")

	var ans string
	err := rpc.JSON("Hi!").JSON(&ans)
	if err != nil {
		panic(err)
	}
	fmt.Println(ans)

	for i := 0; i < 5; i++ {
		publisher.Prepare().String("Hello world!").Send()
	}

	broker.WaitToFinish()
}
