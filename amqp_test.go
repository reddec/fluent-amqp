package fluent

import (
	"context"
	"fmt"
	"github.com/ory/dockertest/v3"
	"github.com/streadway/amqp"
	"log"
	"os"
	"testing"
	"time"
)

var broker *Server

func TestMain(m *testing.M) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	// pulls an image, creates a container based on it and runs it
	resource, err := pool.Run("rabbitmq", "3.6.12", nil)
	if err != nil {
		log.Fatalf("Could not start resource: %s", err)
	}
	//nolint
	resource.Expire(120)

	ctx, cancel := context.WithCancel(SignalContext(nil))
	defer cancel()
	// exponential backoff-retry, because the application in the container might not be ready to accept connections yet
	if err = pool.Retry(func() error {
		port := resource.GetPort("5672/tcp")
		url := "amqp://guest:guest@localhost:" + port
		conn, err := amqp.Dial(url)
		if err != nil {
			return err
		}
		_ = conn.Close()

		broker = Broker(url).StdLogger("[broker] ").Context(ctx).Interval(500 * time.Millisecond).Start()
		return nil
	}); err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	code := m.Run()
	cancel()
	broker.WaitToFinish()
	// You can't defer this because os.Exit doesn't care for defer
	if err := pool.Purge(resource); err != nil {
		log.Fatalf("Could not purge resource: %s", err)
	}

	os.Exit(code)
}

func Test_nack(t *testing.T) {
	var received = make(chan struct{}, 1)
	var initialized = make(chan struct{}, 1)
	var receivedDead = make(chan struct{}, 1)
	var deadInitialized = make(chan struct{}, 1)
	broker.Sink("test-nack").Ready(func() error {
		select {
		case initialized <- struct{}{}:
		default:

		}
		return nil
	}).KeepDead().Retries(0).Requeue(10 * time.Millisecond).Direct("xxx").TransactFunc(func(ctx context.Context, msg amqp.Delivery) error {
		close(received)
		return fmt.Errorf("oops")
	})
	//  let broker create queues
	t.Log("waiting for sink initialization")
	<-initialized

	writer := broker.Publisher().Create()
	t.Log("sending")
	<-writer.Prepare().String("hello").Key("test-nack").Send()

	t.Log("waiting for receive")
	<-received

	broker.Sink("test-nack/dead").Ready(func() error {
		select {
		case deadInitialized <- struct{}{}:
		default:

		}
		return nil
	}).HandlerFunc(func(ctx context.Context, msg amqp.Delivery) {
		receivedDead <- struct{}{}
		t.Log("got dead")
	})
	// let initialize dead reader
	t.Log("waiting for dead queue listener initialization")
	<-deadInitialized

	t.Log("waiting for receive dead message")
	select {
	case <-time.After(5 * time.Second):
		t.Error("didn't get dead message")
	case <-receivedDead:
	}
}
