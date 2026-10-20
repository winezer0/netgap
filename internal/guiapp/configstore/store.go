package configstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultAppName  = "netgapgui"
	defaultLanguage = "zh"
	defaultInterval = 3
)

// Settings 表示 GUI 配置文件内容。
type Settings struct {
	Language           string   `json:"language" yaml:"language"`
	Interval           int      `json:"interval" yaml:"interval"`
	KeywordInput       string   `json:"keywordInput" yaml:"keyword_input"`
	Regions            []string `json:"regions" yaml:"regions"`
	APIEnabled         []string `json:"apiEnabled" yaml:"api_enabled"`
	CustomAPIURL       string   `json:"customApiUrl" yaml:"custom_api_url"`
	CloseAction        string   `json:"closeAction" yaml:"close_action"`
	RemoveRouteOnMatch bool     `json:"removeRouteOnMatch" yaml:"remove_route_on_match"`
}

// Store 表示 GUI 配置存储对象。
type Store struct {
	appName string
	path    string
}

// NewStore 创建 GUI 配置存储对象。
func NewStore(appName string) (*Store, error) {
	if strings.TrimSpace(appName) == "" {
		appName = defaultAppName
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(configDir, appName)
	return NewStoreWithPath(appName, filepath.Join(dir, appName+".yaml")), nil
}

// NewStoreWithPath 创建指定路径的配置存储对象。
func NewStoreWithPath(appName, path string) *Store {
	if strings.TrimSpace(appName) == "" {
		appName = defaultAppName
	}
	return &Store{
		appName: appName,
		path:    path,
	}
}

// Path 返回当前配置文件路径。
func (s *Store) Path() string {
	return s.path
}

// Load 加载配置文件，文件不存在时返回默认值。
func (s *Store) Load() (*Settings, error) {
	content, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultSettings(), nil
		}
		return nil, err
	}
	settings := *DefaultSettings()
	if err := yaml.Unmarshal(content, &settings); err != nil {
		return nil, err
	}
	return NormalizeSettings(&settings), nil
}

// Save 保存配置到 YAML 文件。
func (s *Store) Save(settings *Settings) error {
	normalized := NormalizeSettings(settings)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	content, err := yaml.Marshal(normalized)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, content, 0o644)
}

// DefaultSettings 返回默认配置。
func DefaultSettings() *Settings {
	return &Settings{
		Language:           defaultLanguage,
		Interval:           defaultInterval,
		Regions:            []string{"湖南", "北京", "广东"},
		APIEnabled:         []string{"ipconfig", "ipify", "seeip", "ipapi", "pcjson", "ip2location"},
		CloseAction:        "tray",
		RemoveRouteOnMatch: true,
	}
}

// NormalizeSettings 规范化 GUI 配置内容。
func NormalizeSettings(settings *Settings) *Settings {
	if settings == nil {
		return DefaultSettings()
	}
	result := &Settings{
		Language:           normalizeLanguage(settings.Language),
		Interval:           normalizeInterval(settings.Interval),
		KeywordInput:       strings.TrimSpace(settings.KeywordInput),
		Regions:            normalizeRegions(settings.Regions),
		APIEnabled:         normalizeAPIEnabled(settings.APIEnabled),
		CustomAPIURL:       normalizeCustomAPIURL(settings.CustomAPIURL),
		CloseAction:        normalizeCloseAction(settings.CloseAction),
		RemoveRouteOnMatch: settings.RemoveRouteOnMatch,
	}
	if len(result.Regions) == 0 {
		result.Regions = DefaultSettings().Regions
	}
	if result.CustomAPIURL == "" {
		result.APIEnabled = removeCustomAPI(result.APIEnabled)
	}
	if len(result.APIEnabled) == 0 {
		result.APIEnabled = DefaultSettings().APIEnabled
	}
	return result
}

func normalizeCloseAction(action string) string {
	if strings.EqualFold(strings.TrimSpace(action), "quit") {
		return "quit"
	}
	return "tray"
}

func normalizeLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "en":
		return "en"
	default:
		return defaultLanguage
	}
}

func normalizeInterval(interval int) int {
	if interval == 0 {
		return defaultInterval
	}
	if interval < 1 {
		return 1
	}
	if interval > 60 {
		return 60
	}
	return interval
}

func normalizeRegions(regions []string) []string {
	result := make([]string, 0, len(regions))
	seen := make(map[string]struct{})
	for _, region := range regions {
		trimmed := strings.TrimSpace(region)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func normalizeAPIEnabled(apiEnabled []string) []string {
	result := make([]string, 0, len(apiEnabled))
	seen := make(map[string]struct{})
	for _, api := range apiEnabled {
		trimmed := strings.TrimSpace(api)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func normalizeCustomAPIURL(url string) string {
	return strings.TrimSpace(url)
}

func removeCustomAPI(apiEnabled []string) []string {
	result := make([]string, 0, len(apiEnabled))
	for _, api := range apiEnabled {
		if api != "customip" {
			result = append(result, api)
		}
	}
	return result
}
