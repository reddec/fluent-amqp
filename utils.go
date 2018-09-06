package fluent

import (
	"context"
	"github.com/streadway/amqp"
	"os"
	"os/signal"
)

func SignalContext(parent context.Context) context.Context {
	if parent == nil {
		parent = context.Background()
	}

	ctx, closer := context.WithCancel(parent)
	go func() {
		c := make(chan os.Signal, 2)
		signal.Notify(c, os.Kill, os.Interrupt)
		for range c {
			closer()
			break
		}
	}()
	return ctx
}

func BoolHeader(msg *amqp.Delivery, param string, def bool) bool {
	if msg.Headers == nil {
		return def
	}
	val, ok := msg.Headers[param]
	if !ok {
		return def
	}
	tVal, ok := val.(bool)
	if !ok {
		return def
	}
	return tVal
}

func StringHeader(msg *amqp.Delivery, param string, def string) string {
	if msg.Headers == nil {
		return def
	}
	val, ok := msg.Headers[param]
	if !ok {
		return def
	}
	tVal, ok := val.(string)
	if !ok {
		return def
	}
	return tVal
}

func IntHeader(msg *amqp.Delivery, param string, def int64) int64 {
	if msg.Headers == nil {
		return def
	}
	val, ok := msg.Headers[param]
	if !ok {
		return def
	}

	switch t := val.(type) {
	case int:
		return int64(t)
	case int8:
		return int64(t)
	case int16:
		return int64(t)
	case int32:
		return int64(t)
	case int64:
		return t
	case uint:
		return int64(t)
	case uint8:
		return int64(t)
	case uint16:
		return int64(t)
	case uint32:
		return int64(t)
	case uint64:
		return int64(t)
	default:
		return def
	}
}

func FloatHeader(msg *amqp.Delivery, param string, def float64) float64 {
	if msg.Headers == nil {
		return def
	}
	val, ok := msg.Headers[param]
	if !ok {
		return def
	}

	switch t := val.(type) {
	case int:
		return float64(t)
	case int8:
		return float64(t)
	case int16:
		return float64(t)
	case int32:
		return float64(t)
	case int64:
		return float64(t)
	case uint:
		return float64(t)
	case uint8:
		return float64(t)
	case uint16:
		return float64(t)
	case uint32:
		return float64(t)
	case uint64:
		return float64(t)
	case float32:
		return float64(t)
	case float64:
		return t
	default:
		return def
	}
}
