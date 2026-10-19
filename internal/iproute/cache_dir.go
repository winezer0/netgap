package iproute

import "strings"

var cacheDirOverride string

func SetCacheDir(path string) {
	cacheDirOverride = strings.TrimSpace(path)
}

