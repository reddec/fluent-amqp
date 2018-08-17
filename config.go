package fluent

import (
	"time"
	"context"
	"log"
	"io/ioutil"
	"os"
)

type Logger interface {
	Println(items ...interface{})
}

type BrokerConfig struct {
	urls              []string
	reconnectInterval time.Duration
	connectTimeout    time.Duration
	ctx               context.Context
	logger            Logger
}

func Broker(urls ...string) *BrokerConfig {
	return &BrokerConfig{
		urls:              urls,
		reconnectInterval: 5 * time.Second,
		connectTimeout:    20 * time.Second,
		ctx:               context.Background(),
		logger:            log.New(ioutil.Discard, "", 0),
	}
}

func (bc *BrokerConfig) Interval(tm time.Duration) *BrokerConfig {
	bc.reconnectInterval = tm
	return bc
}

func (bc *BrokerConfig) Timeout(tm time.Duration) *BrokerConfig {
	bc.connectTimeout = tm
	return bc
}

func (bc *BrokerConfig) Logger(logger Logger) *BrokerConfig {
	bc.logger = logger
	return bc
}

func (bc *BrokerConfig) StdLogger(prefix string) *BrokerConfig {
	return bc.Logger(log.New(os.Stderr, prefix, log.LstdFlags))
}

func (bc *BrokerConfig) Context(ctx context.Context) *BrokerConfig {
	bc.ctx = ctx
	return bc
}

func (bc *BrokerConfig) Start() *Server {
	brk := &Server{
		config:          *bc,
		refreshHandlers: make(chan struct{}, 1),
		done:            make(chan struct{}),
	}
	go brk.serve()
	return brk
}
