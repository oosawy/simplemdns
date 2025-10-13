package simplemdns

import (
	"sync"
)

type broadcaster[T any] struct {
	mu     sync.Mutex
	subs   []chan T
	closed bool
}

func newBroadcaster[T any]() *broadcaster[T] {
	return &broadcaster[T]{}
}

func (b *broadcaster[T]) subscribe() <-chan T {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan T)
		close(ch)
		return ch
	}

	ch := make(chan T, 1)
	b.subs = append(b.subs, ch)
	return ch
}

func (b *broadcaster[T]) broadcast(msg T) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	for _, ch := range b.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (b *broadcaster[T]) close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	for _, ch := range b.subs {
		close(ch)
	}
	b.subs = nil
	b.closed = true
}
