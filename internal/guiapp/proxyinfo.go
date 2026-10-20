package guiapp

import (
	"context"

	"netgap/internal/detector"
)

type ProxyConfigInfo struct {
	Enabled bool   `json:"enabled"`
	Type    string `json:"type"`
	Address string `json:"address"`
	Port    string `json:"port"`
	Source  string `json:"source"`
}

func (s *Service) GetProxyConfigInfo() (*ProxyConfigInfo, error) {
	proxy, source, err := detector.DetectProxyWithSource(s.operationContext())
	if err != nil {
		return nil, err
	}
	if proxy == nil {
		return &ProxyConfigInfo{Enabled: false, Source: source}, nil
	}
	return &ProxyConfigInfo{
		Enabled: proxy.Enabled,
		Type:    proxy.Type,
		Address: proxy.Address,
		Port:    proxy.Port,
		Source:  source,
	}, nil
}

// operationContext ties bound UI operations to the application lifecycle.
// A service created outside Wails can still be used before Startup.
func (s *Service) operationContext() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}
