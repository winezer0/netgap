package guiapp

import (
	"reflect"
	"testing"

	"netgap/internal/guiapp/configstore"
)

// TestCloneSettings 验证配置克隆逻辑。
func TestCloneSettings(t *testing.T) {
	t.Parallel()

	input := &configstore.Settings{
		Language:     "zh",
		Interval:     3,
		KeywordInput: "湖南",
		Regions:      []string{"湖南", "北京"},
	}
	output := cloneSettings(input)
	output.Regions[0] = "广东"

	if input.Regions[0] != "湖南" {
		t.Fatal("expected clone to copy regions")
	}
}

// TestFilterRegion 验证区域移除逻辑。
func TestFilterRegion(t *testing.T) {
	t.Parallel()

	output := filterRegion([]string{"湖南", "北京", "广东"}, "北京")
	expected := []string{"湖南", "广东"}
	if !reflect.DeepEqual(output, expected) {
		t.Fatalf("unexpected output: %#v", output)
	}
}

// TestStatusTexts 验证状态文本映射。
func TestStatusTexts(t *testing.T) {
	t.Parallel()

	if runningText(true) != "running" || runningText(false) != "stopped" {
		t.Fatal("unexpected running text")
	}
	if permissionText(true) != "admin" || permissionText(false) != "user" {
		t.Fatal("unexpected permission text")
	}
}
