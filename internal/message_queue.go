package internal

import (
	"container/list"
	"context"
	"sync"
)

// Internal cache for outgoing message (as a workaround for unlimited channels)
type MessageQueue struct {
	lock     sync.Mutex
	messages list.List
	notify   chan struct{}
}

// Get a single message (but not remove) or return an error if context canceled
func (q *MessageQueue) Peek(ctx context.Context) (*Message, error) {
	q.initOnce()
	for {
		q.lock.Lock()
		elem := q.messages.Front()
		if elem != nil {
			q.messages.Remove(elem)
		}
		q.lock.Unlock()
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
}
