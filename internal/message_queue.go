package internal

import (
	"container/list"
	"context"
	"sync"
)

// Internal cache for outgoing message (as a workaround for unlimited channels)
type MessageQueue struct {
	lock     sync.RWMutex
	messages *list.List
	notify   chan struct{}
}

// Len of message buffer
func (q *MessageQueue) Len() int {
	q.initOnce()
	q.lock.RLock()
	defer q.lock.RUnlock()
	return q.messages.Len()
}

// Get a single message (but not remove) or return an error if context canceled
func (q *MessageQueue) Peek(ctx context.Context) (*Message, error) {
	q.initOnce()
	for {
		q.lock.RLock()
		elem := q.messages.Front()
		q.lock.RUnlock()
		if elem != nil {
			return elem.Value.(*Message), nil
		}
		select {
		case <-q.notify:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// Remove front message
func (q *MessageQueue) Commit() {
	q.initOnce()
	q.lock.Lock()
	front := q.messages.Front()
	if front != nil {
		q.messages.Remove(front)
	}
	q.lock.Unlock()
}

// Put message to unlimited cache
func (q *MessageQueue) Put(msg *Message) {
	q.initOnce()
	q.lock.Lock()
	q.messages.PushBack(msg)
	q.lock.Unlock()
	select {
	case q.notify <- struct{}{}:
	default:

	}
}

func (q *MessageQueue) initOnce() {
	if q.notify != nil {
		return
	}
	q.lock.Lock()
	defer q.lock.Unlock()
	if q.notify != nil {
		return
	}
	q.notify = make(chan struct{}, 1)
	q.messages = list.New()
}
