package internal

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMessageQueue_Get(t *testing.T) {
	queue := MessageQueue{}
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				queue.Put(&Message{})
			}
		}()
	}

	var num int
	background, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	for {
		_, err := queue.Get(background)
		if err != nil {
			break
		}
		num++
	}
	wg.Wait()
	if num != 1000 {
		t.Errorf("%d", num)
	}
}
