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
	defaultSink struct {
		retries                int
		expiredMessagesHandler DefaultSinkExpiredHandler
		tooMuchRetries         struct {
			handler   DefaultSinkExpiredHandler
			threshold int64
		}
	}
	urls              []string
	reconnectInterval time.Duration
	connectTimeout    time.Duration
	ctx               context.Context
	logger            Logger
	verboseLogging    bool
}

func Broker(urls ...string) *BrokerConfig {
	cfg := &BrokerConfig{
		urls:              urls,
		reconnectInterval: 5 * time.Second,
		connectTimeout:    20 * time.Second,
		ctx:               context.Background(),
		logger:            log.New(ioutil.Discard, "", 0),
		verboseLogging:    true,
	}
	cfg.defaultSink.retries = 10
	return cfg
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

func (bc *BrokerConfig) Verbose(verbose bool) *BrokerConfig {
	bc.verboseLogging = verbose
	return bc
}

// Default (can be changed in a Sink) maximum retries for sink with TransactHandlers.
// Negative value means no limit. Default is 10.
func (bc *BrokerConfig) Retries(num int) *BrokerConfig {
	bc.defaultSink.retries = num
	return bc
}

// Default handler for Expired handler in a sink
func (bc *BrokerConfig) OnExpired(handler DefaultSinkExpiredHandler) *BrokerConfig {
	bc.defaultSink.expiredMessagesHandler = handler
	return bc
}

// Default handler for TooMuchRetries handler in a sink
func (bc *BrokerConfig) OnTooMuchRetries(threshold int64, handler DefaultSinkExpiredHandler) *BrokerConfig {
	bc.defaultSink.tooMuchRetries.handler = handler
	bc.defaultSink.tooMuchRetries.threshold = threshold
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
	go brk.start()
	return brk
}
