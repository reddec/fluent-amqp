package fluent

import (
	"context"
	"io/ioutil"
	"log"
	"os"
	"time"
)

type Logger interface {
	Println(items ...interface{})
}

type BrokerConfig struct {
	defaultSinkRetriesCount int
	urls                    []string
	reconnectInterval       time.Duration
	connectTimeout          time.Duration
	ctx                     context.Context
	logger                  Logger
}

func Broker(urls ...string) *BrokerConfig {
	return &BrokerConfig{
		defaultSinkRetriesCount: 10,
		urls:                    urls,
		reconnectInterval:       5 * time.Second,
		connectTimeout:          20 * time.Second,
		ctx:                     context.Background(),
		logger:                  log.New(ioutil.Discard, "", 0),
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

// Default (can be changed in a Sink) maximum retries for sink with TransactHandlers.
// Negative value means no limit. Default is 10.
func (bc *BrokerConfig) Retries(num int) *BrokerConfig {
	bc.defaultSinkRetriesCount = num
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
		config:              *bc,
		refreshHandlers:     make(chan struct{}, 1),
		done:                make(chan struct{}),
		defaultRetriesCount: 10,
	}
	go brk.serve()
	return brk
}
