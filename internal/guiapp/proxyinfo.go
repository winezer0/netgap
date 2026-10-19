package guiapp

import (
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
	proxy, source, err := detector.DetectProxyWithSource()
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
