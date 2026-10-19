//go:build linux

package detector

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"
)

func getProxyFromSystem() *ProxyConfig {
	mode, err := runGsettings("org.gnome.system.proxy", "mode")
	if err != nil {
		return nil
	}
	if strings.Trim(mode, "'\" \n\t") != "manual" {
		return nil
	}
	host, err := runGsettings("org.gnome.system.proxy.http", "host")
	if err != nil {
		return nil
	}
	host = strings.Trim(host, "'\" \n\t")
	if host == "" {
		return nil
	}
	portText, err := runGsettings("org.gnome.system.proxy.http", "port")
	if err != nil {
		return nil
	}
	portText = strings.Trim(portText, "'\" \n\t")
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 {
		return nil
	}
	return &ProxyConfig{
		Type:    "port",
		Address: host,
		Port:    strconv.Itoa(port),
		Enabled: true,
	}
}

func runGsettings(schema, key string) (string, error) {
	cmd := exec.Command("gsettings", "get", schema, key)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}

