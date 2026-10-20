// Package netwatch maintains a persistent notification source. Each fingerprint
// drains queued events synchronously, including changes that returned to the
// original snapshot before the query completed.
package netwatch

import (
	"errors"
	"sync"
	"time"
)

type Source interface {
	Drain() (uint64, error)
	Close() error
}

type Watcher struct {
	mu         sync.Mutex
	open       func() (Source, error)
	source     Source
	generation uint64
	failure    error
	next       time.Time
	delay      time.Duration
	now        func() time.Time
}

func New(open func() (Source, error)) *Watcher { return &Watcher{open: open, now: time.Now} }

func (w *Watcher) Revision() (uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.source == nil {
		if w.now().Before(w.next) {
			return 0, w.failure
		}
		source, err := w.open()
		if err != nil {
			return w.failed(err)
		}
		w.source = source
		w.generation++ // Rebuilding must invalidate every pre-failure result.
	}
	count, err := w.source.Drain()
	if err != nil {
		return w.failed(err)
	}
	w.generation += count
	w.failure = nil
	w.delay = 0
	return w.generation, nil
}

func (w *Watcher) failed(err error) (uint64, error) {
	if err == nil {
		err = errors.New("network notification source failed")
	}
	if w.source != nil {
		_ = w.source.Close()
		w.source = nil
	}
	w.generation++
	w.failure = err
	if w.delay == 0 {
		w.delay = 500 * time.Millisecond
	} else {
		w.delay *= 2
	}
	if w.delay > 30*time.Second {
		w.delay = 30 * time.Second
	}
	w.next = w.now().Add(w.delay)
	return 0, err
}
