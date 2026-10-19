package main

import (
	"os"
	"reflect"
	"testing"
)

func TestParseOptions(t *testing.T) {
	os.Args = []string{"netgap"}
	opts, err := ParseOptions()
	if err != nil {
		t.Errorf("ParseOptions failed: %v", err)
	}
	if opts.Interval != 3 {
		t.Errorf("Expected interval 3, got %d", opts.Interval)
	}
	if opts.LogLevel != "info" {
		t.Errorf("Expected log level 'info', got '%s'", opts.LogLevel)
	}
	if opts.ShutdownTest != false {
		t.Errorf("Expected ShutdownTest false, got %v", opts.ShutdownTest)
	}
	expectedRegions := []string{"湖南", "北京", "上海"}
	if !reflect.DeepEqual(opts.Regions, expectedRegions) {
		t.Errorf("Expected Regions %v, got %v", expectedRegions, opts.Regions)
	}
}

func TestParseOptionsWithShutdownTest(t *testing.T) {
	os.Args = []string{"netgap", "-S"}
	opts, err := ParseOptions()
	if err != nil {
		t.Errorf("ParseOptions with -S failed: %v", err)
	}
	if opts.ShutdownTest != true {
		t.Errorf("Expected ShutdownTest true, got %v", opts.ShutdownTest)
	}
}

func TestParseOptionsWithRecover(t *testing.T) {
	os.Args = []string{"netgap", "-r"}
	opts, err := ParseOptions()
	if err != nil {
		t.Errorf("ParseOptions with -r failed: %v", err)
	}
	if opts.Recover != true {
		t.Errorf("Expected Recover true, got %v", opts.Recover)
	}
}

func TestSplitCommaSeparatedSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "空切片",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "nil切片",
			input:    nil,
			expected: nil,
		},
		{
			name:     "单元素逗号分隔",
			input:    []string{"湖南,北京,上海"},
			expected: []string{"湖南", "北京", "上海"},
		},
		{
			name:     "多元素无逗号",
			input:    []string{"湖南", "北京"},
			expected: []string{"湖南", "北京"},
		},
		{
			name:     "混合逗号和独立元素",
			input:    []string{"湖南,北京", "上海"},
			expected: []string{"湖南", "北京", "上海"},
		},
		{
			name:     "带空格的逗号分隔",
			input:    []string{"湖南, 北京, 上海"},
			expected: []string{"湖南", "北京", "上海"},
		},
		{
			name:     "空字符串元素",
			input:    []string{"湖南,,北京"},
			expected: []string{"湖南", "北京"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitCommaSeparatedSlice(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("splitCommaSeparatedSlice(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
