package releasebot

import (
	"sync"
)

// asyncQueue delivers messages in order from its own goroutine; push never blocks.
type asyncQueue struct {
	mu    sync.Mutex
	items []string
	wake  chan struct{}

	// closed once every queued message was delivered after close
	closed bool
	done   chan struct{}
}

func newAsyncQueue(deliver func(string)) *asyncQueue {
	q := &asyncQueue{wake: make(chan struct{}, 1), done: make(chan struct{})}
	go func() {
		defer close(q.done)
		for {
			q.mu.Lock()
			if len(q.items) == 0 {
				closed := q.closed
				q.mu.Unlock()
				if closed {
					return
				}
				<-q.wake
				continue
			}
			msg := q.items[0]
			q.items = q.items[1:]
			q.mu.Unlock()
			deliver(msg)
		}
	}()

	return q
}

// push queues msg; after close nothing consumes the queue, so it is dropped.
func (q *asyncQueue) push(msg string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.items = append(q.items, msg)
	q.signal()
}

// close stops the queue and waits until the queued messages are delivered (each post has its own
// timeout), so a run's last messages go out before it reports its end.
func (q *asyncQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.signal()
	<-q.done
}

func (q *asyncQueue) signal() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}
