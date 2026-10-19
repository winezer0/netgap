package logbuffer

import (
	"fmt"
	"sync"
	"time"
)

// Buffer 表示线程安全的日志缓存。
type Buffer struct {
	mu       sync.RWMutex
	capacity int
	lines    []string
}

// New 创建日志缓存对象。
func New(capacity int) *Buffer {
	if capacity < 1 {
		capacity = 200
	}
	return &Buffer{
		capacity: capacity,
		lines:    make([]string, 0, capacity),
	}
}

// Add 添加一条日志。
func (b *Buffer) Add(level, message string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	line := fmt.Sprintf("%s [%s] %s",
		time.Now().Format("15:04:05"), level, message)
	b.lines = append(b.lines, line)
	if len(b.lines) > b.capacity {
		b.lines = append([]string(nil), b.lines[len(b.lines)-b.capacity:]...)
	}
}

// List 返回当前日志快照。
func (b *Buffer) List() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]string, len(b.lines))
	copy(result, b.lines)
	return result
}

// Clear 清空日志缓存。
func (b *Buffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = b.lines[:0]
}
