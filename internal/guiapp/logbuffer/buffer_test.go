package logbuffer

import (
	"sync"
	"testing"
)

func TestErrorCountSurvivesLogTrimmingAndClearing(t *testing.T) {
	buffer := New(1)
	buffer.Add("ERROR", "operation failed")
	buffer.Add("INFO", "query succeeded")
	buffer.Add("WARN", "network changed")
	lines, count := buffer.Snapshot()
	if len(lines) != 1 || count != 1 {
		t.Fatalf("trimming lost error count: %v %d", lines, count)
	}
	buffer.Clear()
	lines, count = buffer.Snapshot()
	if len(lines) != 0 || count != 1 {
		t.Fatalf("clearing logs reset session count: %v %d", lines, count)
	}
	buffer.Add("ERROR", "another failure")
	_, count = buffer.Snapshot()
	if count != 2 {
		t.Fatalf("expected two session errors, got %d", count)
	}
}

func TestConcurrentErrorsAreCountedOnce(t *testing.T) {
	buffer := New(5)
	var workers sync.WaitGroup
	for range 100 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			buffer.Add("ERROR", "concurrent error")
		}()
	}
	workers.Wait()
	_, count := buffer.Snapshot()
	if count != 100 {
		t.Fatalf("lost concurrent errors: %d", count)
	}
}

// TestBufferAddAndTrim 验证日志缓存的容量裁剪。
func TestBufferAddAndTrim(t *testing.T) {
	t.Parallel()

	buffer := New(2)
	buffer.Add("INFO", "first")
	buffer.Add("INFO", "second")
	buffer.Add("WARN", "third")

	lines := buffer.List()
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] == "" || lines[1] == "" {
		t.Fatal("expected non-empty lines")
	}
	if lines[0] == lines[1] {
		t.Fatal("expected different log lines")
	}
}

// TestBufferClear 验证日志缓存清空逻辑。
func TestBufferClear(t *testing.T) {
	t.Parallel()

	buffer := New(3)
	buffer.Add("INFO", "message")
	buffer.Clear()

	if len(buffer.List()) != 0 {
		t.Fatal("expected empty buffer after clear")
	}
}
