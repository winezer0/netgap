package configstore

import (
	"path/filepath"
	"reflect"
	"testing"
)

// TestNormalizeSettings 验证配置规范化逻辑。
func TestNormalizeSettings(t *testing.T) {
	t.Parallel()

	settings := NormalizeSettings(&Settings{
		Language:     "EN",
		Interval:     99,
		KeywordInput: "  湖南  ",
		Regions:      []string{"湖南", "", "北京", "湖南"},
	})

	if settings.Language != "en" {
		t.Fatalf("expected en, got %s", settings.Language)
	}
	if settings.Interval != 60 {
		t.Fatalf("expected interval 60, got %d", settings.Interval)
	}
	if settings.KeywordInput != "湖南" {
		t.Fatalf("expected keyword 湖南, got %s", settings.KeywordInput)
	}
	expectedRegions := []string{"湖南", "北京"}
	if !reflect.DeepEqual(settings.Regions, expectedRegions) {
		t.Fatalf("unexpected regions: %#v", settings.Regions)
	}
}

// TestStoreSaveLoad 验证配置文件保存与加载。
func TestStoreSaveLoad(t *testing.T) {
	t.Parallel()

	store := NewStoreWithPath("netgapgui", filepath.Join(t.TempDir(), "netgapgui.yaml"))
	input := &Settings{
		Language:     "zh",
		Interval:     5,
		KeywordInput: "广东",
		Regions:      []string{"广东", "深圳"},
		APIEnabled:   []string{"pcjson", "ip2location", "customip"},
		CustomAPIURL: "http://38.55.192.13:8080/ip?token=ipinfoapitoken",
		CloseAction:  "tray",
	}

	if err := store.Save(input); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	output, err := store.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if !reflect.DeepEqual(output, input) {
		t.Fatalf("unexpected settings: %#v", output)
	}
}

// TestStoreLoadDefault 验证默认配置加载。
func TestStoreLoadDefault(t *testing.T) {
	t.Parallel()

	store := NewStoreWithPath("netgapgui", filepath.Join(t.TempDir(), "missing.yaml"))
	settings, err := store.Load()
	if err != nil {
		t.Fatalf("load default failed: %v", err)
	}
	if settings.Interval != 3 {
		t.Fatalf("expected default interval 3, got %d", settings.Interval)
	}
	if settings.Language != "zh" {
		t.Fatalf("expected default language zh, got %s", settings.Language)
	}
	if settings.CloseAction != "tray" {
		t.Fatalf("expected default close action tray, got %s", settings.CloseAction)
	}
	if !settings.RemoveRouteOnMatch {
		t.Fatal("expected route removal on match to be enabled by default")
	}
	if len(settings.Regions) == 0 {
		t.Fatal("expected default regions")
	}
}
