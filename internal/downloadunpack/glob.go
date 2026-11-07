package downloadunpack

import (
	"fmt"
	"path"
	"strings"
)

func normalizeSourcePattern(pattern string) (string, error) {
	if strings.ContainsAny(pattern, `/\\`) {
		return "", fmt.Errorf("source pattern must match an asset name, not a path")
	}
	if pattern == "" {
		return "", nil
	}
	if strings.ContainsRune(pattern, '\x00') || hasWindowsDrivePrefix(pattern) {
		return "", fmt.Errorf("source pattern is not a valid asset name pattern")
	}
	if _, err := path.Match(pattern, ""); err != nil {
		return "", fmt.Errorf("invalid glob syntax: %w", err)
	}
	return pattern, nil
}

func normalizeGlobPattern(pattern string, allowRoot bool) (string, error) {
	if pattern == "" {
		return "", nil
	}
	pattern = strings.ReplaceAll(pattern, `\`, "/")
	if strings.ContainsRune(pattern, '\x00') || path.IsAbs(pattern) || hasWindowsDrivePrefix(pattern) {
		return "", fmt.Errorf("pattern must be a relative path")
	}
	for _, component := range strings.Split(pattern, "/") {
		if component == ".." {
			return "", fmt.Errorf("pattern cannot traverse to a parent directory")
		}
	}
	pattern = path.Clean(pattern)
	if pattern == "." {
		if allowRoot {
			return pattern, nil
		}
		return "", fmt.Errorf("pattern must identify a relative path")
	}
	for _, component := range strings.Split(pattern, "/") {
		if component == "" || component == "." || component == ".." {
			return "", fmt.Errorf("pattern contains an invalid path component")
		}
		if component == "**" {
			continue
		}
		if _, err := path.Match(component, ""); err != nil {
			return "", fmt.Errorf("invalid glob syntax in %q: %w", component, err)
		}
	}
	return pattern, nil
}

func matchAssetPattern(pattern, name string) (bool, error) {
	return path.Match(pattern, name)
}

func matchRelativeGlob(pattern, candidate string) (bool, error) {
	patternParts := strings.Split(pattern, "/")
	candidate = strings.ReplaceAll(candidate, `\`, "/")
	candidateParts := strings.Split(candidate, "/")
	return matchGlobParts(patternParts, candidateParts)
}

func matchGlobParts(pattern, candidate []string) (bool, error) {
	type position struct {
		pattern   int
		candidate int
	}
	known := make(map[position]bool)
	results := make(map[position]bool)
	var match func(int, int) (bool, error)
	match = func(patternIndex, candidateIndex int) (bool, error) {
		current := position{pattern: patternIndex, candidate: candidateIndex}
		if known[current] {
			return results[current], nil
		}
		known[current] = true
		if patternIndex == len(pattern) {
			results[current] = candidateIndex == len(candidate)
			return results[current], nil
		}
		if pattern[patternIndex] == "**" {
			matched, err := match(patternIndex+1, candidateIndex)
			if err != nil {
				return false, err
			}
			if matched {
				results[current] = true
				return true, nil
			}
			if candidateIndex < len(candidate) {
				matched, err = match(patternIndex, candidateIndex+1)
				if err != nil {
					return false, err
				}
				results[current] = matched
				return matched, nil
			}
			return false, nil
		}
		if candidateIndex >= len(candidate) {
			return false, nil
		}
		matched, err := path.Match(pattern[patternIndex], candidate[candidateIndex])
		if err != nil || !matched {
			return false, err
		}
		matched, err = match(patternIndex+1, candidateIndex+1)
		if err != nil {
			return false, err
		}
		results[current] = matched
		return matched, nil
	}
	return match(0, 0)
}
