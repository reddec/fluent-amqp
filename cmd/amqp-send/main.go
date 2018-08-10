package main

import (
	"github.com/jessevdk/go-flags"
	"os"
	"github.com/reddec/fluent-amqp"
	"log"
	"io/ioutil"
	"time"
	"context"
	"github.com/pkg/errors"
	"github.com/google/uuid"
	"fmt"
)

var (
	version = "dev"
)

var config struct {
	URLs          []string          `short:"u" long:"url"              env:"BROKER_URL"         description:"One or more AMQP brokers urls" default:"amqp://guest:guest@localhost" required:"yes"`
	Exchange      string            `short:"e" long:"exchange"         env:"BROKER_EXCHANGE"    description:"Name of AMQP exchange. Can be empty"`
	ExchangeType  string            `short:"k" long:"kind"             env:"BROKER_KIND"        description:"Exchange kind" choice:"direct,topic,fanout" default:"direct"`
	Sign          string            `short:"s" long:"sign-private-key" env:"BROKER_SIGN"        description:"Path to private key to sign"`
	MessageID     string            `short:"i" long:"id"               env:"BROKER_MESSAGED_ID" description:"Message ID. If empty it is generated"`
	ReplyTo       string            `short:"r" long:"reply-to"         env:"BROKER_REPLY_TO"    description:"Reply queue name. Should be used together with correlation-id"`
	CorrelationID string            `short:"c" long:"correlation-id"   env:"BROKER_CORRELATION_ID" description:"Correlation ID in message. Usually used together with reply-to"`
	Headers       map[string]string `short:"h" long:"header"           env:"BROKER_HEADER"       description:"Custom headers"`
	ContentType   string            `short:"t" long:"content-type"     env:"BROKER_CONTENT_TYPE" description:"Custom content type"`
	TTL           time.Duration     `          long:"ttl"              env:"BROKER_TTL"          description:"Time to live of message"`

	Args struct {
		RoutingKey string `positional-arg-name:"routing-key" env:"BROKER_ROUTING_KEY" description:"Routing key or queue name if exchange is empty" required:"yes"`
	} `positional-args:"yes"`

	Interval time.Duration `short:"R" long:"reconnect-interval" env:"BROKER_RECONNECT_INTERVAL" description:"Reconnect timeout" default:"5s"`
	Timeout  time.Duration `short:"T" long:"timeout" env:"BROKER_CONNECT_TIMEOUT" description:"Connect timeout" default:"30s"`

	Quiet   bool `short:"q" long:"quiet" env:"BROKER_QUIET" description:"Suppress all log messages"`
	Version bool `short:"v" long:"version" description:"Print version and exit"`
}

func run() error {
	gctx, cancel := context.WithCancel(context.Background())
	ctx := fluent.SignalContext(gctx)
	broker := fluent.Broker(config.URLs...).Context(ctx).Logger(log.New(os.Stderr, "[broker] ", log.LstdFlags)).Interval(config.Interval).Timeout(config.Timeout).Start()
	defer broker.WaitToFinish()
	defer cancel()
	log.Println("preparing publisher")
	publisherCfg := broker.Publisher()
	if config.Exchange != "" {
		switch config.ExchangeType {
		case "topic":
			publisherCfg = publisherCfg.DefaultTopic(config.Exchange)
		case "direct":
			publisherCfg = publisherCfg.DefaultDirect(config.Exchange)
		case "fanout":
			publisherCfg = publisherCfg.DefaultFanout(config.Exchange)
		default:
			return errors.Errorf("unknown exchange type %v", config.ExchangeType)
		}
	}
	publisherCfg = publisherCfg.DefaultKey(config.Args.RoutingKey)

	if config.Sign != "" {
		log.Println("preparing signer")
		publisherCfg = publisherCfg.Sign(config.Sign)
	}
	writer := publisherCfg.Create()
	log.Println("publisher prepared")
	log.Println("waiting for input data...")

	input := make(chan []byte)
	go func() {
		data, err := ioutil.ReadAll(os.Stdin)
		log.Println("failed read STDIN:", err)
		if err != nil {
			close(input)
		} else {
			input <- data
		}
	}()
	var data []byte

	select {
	case inData, ok := <-input:
		if !ok {
			return errors.New("read STDIN")
		}
		data = inData
	case <-ctx.Done():
		return ctx.Err()
	}

	if config.MessageID == "" {
		config.MessageID = uuid.New().String()
		log.Println("generated message id:", config.MessageID)
	}

	msg := writer.Prepare().Bytes(data).ID(config.MessageID).TTL(config.TTL)
	log.Println("filling properties")
	for k, v := range config.Headers {
		msg = msg.Header(k, v)
	}

	msg = msg.Type(config.ContentType).Reply(config.CorrelationID, config.ReplyTo)
	msg.Send()
	return ctx.Err()
}

func main() {
	parser := flags.NewParser(&config, flags.Default)
	_, err := parser.Parse()
	if config.Version {
		fmt.Println(version)
		return
	}
	if err != nil {
		os.Exit(1)
	}
	if config.Quiet {
		log.SetOutput(ioutil.Discard)
	}
	err = run()
	if err != nil {
		log.Println("failed:", err)
		os.Exit(2)
	}
}
