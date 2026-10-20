package netwatch

import (
	"errors"
	"testing"
	"time"
)

type fakeSource struct {
	count  uint64
	err    error
	closed int
}

func (s *fakeSource) Drain() (uint64, error) { n := s.count; s.count = 0; return n, s.err }
func (s *fakeSource) Close() error           { s.closed++; return nil }

func TestFailureBackoffAndRecovery(t *testing.T) {
	now := time.Unix(100, 0)
	attempts := 0
	first, second := &fakeSource{}, &fakeSource{}
	watcher := New(func() (Source, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("open failure")
		}
		if attempts == 2 {
			return first, nil
		}
		return second, nil
	})
	watcher.now = func() time.Time { return now }
	if _, err := watcher.Revision(); err == nil {
		t.Fatal("open failure hidden")
	}
	for range 10 {
		if _, err := watcher.Revision(); err == nil {
			t.Fatal("backoff failure hidden")
		}
	}
	if attempts != 1 {
		t.Fatal("retried during backoff")
	}
	now = now.Add(time.Second)
	before, err := watcher.Revision()
	if err != nil {
		t.Fatal(err)
	}
	first.count = 2 // A -> B -> A: identical snapshots must still invalidate.
	after, err := watcher.Revision()
	if err != nil || after != before+2 {
		t.Fatal("transient events lost", after, err)
	}
	first.err = errors.New("queue overflow")
	if _, err := watcher.Revision(); err == nil || first.closed != 1 {
		t.Fatal("lost source retained", err)
	}
	now = now.Add(time.Second)
	recovered, err := watcher.Revision()
	if err != nil || recovered <= after {
		t.Fatal("stale generation after rebuild", err)
	}
}

func TestRetryDelayBounded(t *testing.T) {
	now := time.Unix(100, 0)
	w := New(func() (Source, error) { return nil, errors.New("unavailable") })
	w.now = func() time.Time { return now }
	for range 20 {
		_, _ = w.Revision()
		if w.delay > 30*time.Second {
			t.Fatal(w.delay)
		}
		now = w.next
	}
	if w.delay != 30*time.Second {
		t.Fatal(w.delay)
	}
}
