package logbuffer

import "testing"

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
