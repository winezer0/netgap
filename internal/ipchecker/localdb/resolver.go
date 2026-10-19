package localdb

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/winezer0/ipinfo/pkg/iplocate"
	"github.com/winezer0/ipinfo/pkg/iplocate/ip2region"
	"github.com/winezer0/ipinfo/pkg/queryip"
	logging "github.com/winezer0/zaplogs"

	"netgap/assets"
	"netgap/internal/ipchecker/types"
)

const (
	ipv4DatabaseName = "ip2region_v4.xdb"
	ipv6DatabaseName = "ip2region_v6.xdb"
)

var ErrNoLocation = errors.New("IP location is not present in the database")

type databasePaths struct {
	ipv4 string
	ipv6 string
}

// Resolver performs local IP location lookups using paired IPv4 and IPv6 databases.
type Resolver struct {
	mu     sync.RWMutex
	engine *queryip.DBEngine
}

// NewResolver prefers a valid database pair beside the executable, then uses embedded databases.
func NewResolver() (*Resolver, error) {
	if paths, found := findUserDatabasePaths(); found {
		engine, err := openEngine(paths)
		if err == nil {
			logging.Infof("Using user IP databases from %s", filepath.Dir(paths.ipv4))
			return &Resolver{engine: engine}, nil
		}
		logging.Warnf("User IP databases are unavailable, using embedded databases: %v", err)
	}

	paths, err := prepareEmbeddedDatabases()
	if err != nil {
		return nil, err
	}
	engine, err := openEngine(paths)
	if err != nil {
		if writeErr := writeEmbeddedPair(paths); writeErr != nil {
			return nil, fmt.Errorf("repair embedded IP databases: %w", writeErr)
		}
		engine, err = openEngine(paths)
		if err != nil {
			return nil, fmt.Errorf("open embedded IP databases: %w", err)
		}
	}

	logging.Infof("Using embedded IP databases")
	return &Resolver{engine: engine}, nil
}

// Resolve returns local location information for an IP address.
func (r *Resolver) Resolve(ip string) (*types.IPInfo, error) {
	address, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return nil, fmt.Errorf("parse IP address: %w", err)
	}
	address = address.Unmap()

	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.engine == nil {
		return nil, errors.New("IP location resolver is closed")
	}

	result := r.engine.QueryIP(address.String())
	if result == nil || result.IPLocate == nil {
		return nil, ErrNoLocation
	}
	location := result.IPLocate
	if location.Country == "" && location.Province == "" && location.City == "" {
		return nil, ErrNoLocation
	}

	return &types.IPInfo{
		IP:      address.String(),
		Country: location.Country,
		Region:  location.Province,
		City:    location.City,
		Isp:     location.ISP,
	}, nil
}

// Close releases the database searchers.
func (r *Resolver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.engine == nil {
		return nil
	}
	err := r.engine.Close()
	r.engine = nil
	return err
}

func findUserDatabasePaths() (databasePaths, bool) {
	candidates := make([]string, 0, 2)
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), "assets"))
	}
	if workingDirectory, err := os.Getwd(); err == nil {
		candidate := filepath.Join(workingDirectory, "assets")
		if len(candidates) == 0 || !samePath(candidate, candidates[0]) {
			candidates = append(candidates, candidate)
		}
	}

	for _, directory := range candidates {
		info, err := os.Stat(directory)
		if err != nil || !info.IsDir() {
			continue
		}
		return databasePaths{
			ipv4: filepath.Join(directory, ipv4DatabaseName),
			ipv6: filepath.Join(directory, ipv6DatabaseName),
		}, true
	}
	return databasePaths{}, false
}

func prepareEmbeddedDatabases() (databasePaths, error) {
	digest, err := embeddedDatabaseDigest()
	if err != nil {
		return databasePaths{}, err
	}
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		cacheRoot = os.TempDir()
	}
	directory := filepath.Join(cacheRoot, "netgap", "ip2region", digest)
	paths := databasePaths{
		ipv4: filepath.Join(directory, ipv4DatabaseName),
		ipv6: filepath.Join(directory, ipv6DatabaseName),
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return databasePaths{}, fmt.Errorf("create embedded IP database cache: %w", err)
	}
	if err := ensureEmbeddedFile(ipv4DatabaseName, paths.ipv4); err != nil {
		return databasePaths{}, err
	}
	if err := ensureEmbeddedFile(ipv6DatabaseName, paths.ipv6); err != nil {
		return databasePaths{}, err
	}
	return paths, nil
}

func embeddedDatabaseDigest() (string, error) {
	hash := sha256.New()
	for _, name := range []string{ipv4DatabaseName, ipv6DatabaseName} {
		file, err := assets.Files.Open(name)
		if err != nil {
			return "", fmt.Errorf("open embedded IP database %s: %w", name, err)
		}
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", fmt.Errorf("hash embedded IP database %s: %w", name, copyErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("close embedded IP database %s: %w", name, closeErr)
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func ensureEmbeddedFile(name, path string) error {
	fileInfo, err := fs.Stat(assets.Files, name)
	if err != nil {
		return fmt.Errorf("stat embedded IP database %s: %w", name, err)
	}
	localInfo, err := os.Stat(path)
	if err == nil && localInfo.Size() == fileInfo.Size() {
		return nil
	}
	return writeEmbeddedFile(name, path)
}

func writeEmbeddedPair(paths databasePaths) error {
	if err := writeEmbeddedFile(ipv4DatabaseName, paths.ipv4); err != nil {
		return err
	}
	return writeEmbeddedFile(ipv6DatabaseName, paths.ipv6)
}

func writeEmbeddedFile(name, path string) error {
	source, err := assets.Files.Open(name)
	if err != nil {
		return fmt.Errorf("open embedded IP database %s: %w", name, err)
	}
	defer source.Close()

	temporary, err := os.CreateTemp(filepath.Dir(path), name+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary IP database %s: %w", name, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if _, err := io.Copy(temporary, source); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("extract embedded IP database %s: %w", name, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("flush embedded IP database %s: %w", name, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary IP database %s: %w", name, err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replace cached IP database %s: %w", name, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("install embedded IP database %s: %w", name, err)
	}
	return nil
}

func openEngine(paths databasePaths) (*queryip.DBEngine, error) {
	ipv4Engine, err := ip2region.NewIP2Region(iplocate.IPv4VersionNo, paths.ipv4)
	if err != nil {
		return nil, fmt.Errorf("open IPv4 database: %w", err)
	}
	ipv6Engine, err := ip2region.NewIP2Region(iplocate.IPv6VersionNo, paths.ipv6)
	if err != nil {
		ipv4Engine.Close()
		return nil, fmt.Errorf("open IPv6 database: %w", err)
	}
	return &queryip.DBEngine{
		IPv4Engine: ipv4Engine,
		IPv6Engine: ipv6Engine,
	}, nil
}

func samePath(first, second string) bool {
	firstAbs, firstErr := filepath.Abs(first)
	secondAbs, secondErr := filepath.Abs(second)
	return firstErr == nil && secondErr == nil && strings.EqualFold(firstAbs, secondAbs)
}
