package iproute

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"
)

type RouteBackupSummary struct {
	ID         string    `json:"id"`
	SavedAt    time.Time `json:"savedAt"`
	RouteCount int       `json:"routeCount"`
	IPv4Count  int       `json:"ipv4Count"`
	IPv6Count  int       `json:"ipv6Count"`
	Interfaces []string  `json:"interfaces"`
}

type RouteBackupHistory struct {
	Records      []RouteBackupSummary `json:"records"`
	InvalidLines int                  `json:"invalidLines"`
}

type RouteBackupRoute struct {
	Saved   RouteEntry  `json:"saved"`
	Current *RouteEntry `json:"current,omitempty"`
	State   string      `json:"state"` // present, missing, conflict, unavailable
	Reason  string      `json:"reason,omitempty"`
}

type RouteBackupDetail struct {
	Summary RouteBackupSummary `json:"summary"`
	Routes  []RouteBackupRoute `json:"routes"`
}

type RouteBackupRestoreResult struct {
	Restored       int      `json:"restored"`
	AlreadyPresent int      `json:"alreadyPresent"`
	Replaced       int      `json:"replaced"`
	Skipped        int      `json:"skipped"`
	Failed         int      `json:"failed"`
	Failures       []string `json:"failures"`
	Complete       bool     `json:"complete"`
}

type routeBackupTask struct {
	ID        string       `json:"id"`
	StartedAt time.Time    `json:"startedAt"`
	Routes    []RouteEntry `json:"routes"`
}

func ListRouteBackups() (RouteBackupHistory, error) {
	records, invalid, err := readRouteBackupRecords()
	if err != nil {
		return RouteBackupHistory{}, err
	}
	result := RouteBackupHistory{Records: []RouteBackupSummary{}, InvalidLines: invalid}
	for i := len(records) - 1; i >= 0; i-- {
		result.Records = append(result.Records, records[i].summary)
	}
	return result, nil
}

func GetRouteBackupDetail(ctx context.Context, id string) (*RouteBackupDetail, error) {
	release, err := acquireRouteOperation(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	entry, err := findRouteBackup(id)
	if err != nil {
		return nil, err
	}
	current, err := snapshotBackend.list(ctx)
	if err != nil {
		return nil, err
	}
	detail := &RouteBackupDetail{Summary: entry.summary}
	for _, saved := range entry.record.Routes {
		item := RouteBackupRoute{Saved: saved, State: "missing"}
		if err := snapshotBackend.validate(ctx, []RouteEntry{saved}); err != nil {
			item.State, item.Reason = "unavailable", err.Error()
		} else {
			for i := range current {
				if sameRoute(saved, current[i]) {
					item.State, item.Current = "present", routeCopy(&current[i])
					break
				}
				if sameRouteTarget(saved, current[i]) {
					item.State, item.Current = "conflict", routeCopy(&current[i])
					break
				}
			}
		}
		detail.Routes = append(detail.Routes, item)
	}
	return detail, nil
}

func RestoreRouteBackup(ctx context.Context, id string, replaceConflicts bool) (*RouteBackupRestoreResult, error) {
	release, err := acquireRouteOperation(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	entry, err := findRouteBackup(id)
	if err != nil {
		return nil, err
	}
	taskFile := routeBackupTaskPath()
	task, err := loadRouteBackupTask(taskFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read pending history restore: %w", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		routes := append([]RouteEntry(nil), entry.record.Routes...)
		sort.SliceStable(routes, func(i, j int) bool { return routes[i].Store == "ActiveStore" && routes[j].Store == "PersistentStore" })
		task = &routeBackupTask{ID: id, StartedAt: time.Now().UTC(), Routes: routes}
		if err := saveRouteBackupTask(taskFile, task); err != nil {
			return nil, err
		}
	} else if task.ID != id {
		return nil, errors.New("another historical route restore is incomplete; retry or resolve it first")
	}
	result := &RouteBackupRestoreResult{}
	var pending []RouteEntry
	for _, saved := range task.Routes {
		if err := ctx.Err(); err != nil {
			pending = append(pending, saved)
			result.fail(err.Error())
			continue
		}
		if err := snapshotBackend.validate(ctx, []RouteEntry{saved}); err != nil {
			result.Skipped++
			result.Failures = append(result.Failures, fmt.Sprintf("%s on %s: %v", saved.Destination, saved.InterfaceAlias, err))
			continue
		}
		current, err := snapshotBackend.list(ctx)
		if err != nil {
			pending = append(pending, saved)
			result.fail(err.Error())
			continue
		}
		var exact, conflict *RouteEntry
		for i := range current {
			if sameRoute(saved, current[i]) {
				exact = routeCopy(&current[i])
				break
			}
			if sameRouteTarget(saved, current[i]) {
				conflict = routeCopy(&current[i])
				break
			}
		}
		if exact != nil {
			result.AlreadyPresent++
			continue
		}
		if conflict != nil {
			if !replaceConflicts {
				return nil, fmt.Errorf("route conflict: %s on %s currently uses gateway %s, backup uses %s", saved.Destination, saved.InterfaceAlias, conflict.NextHop, saved.NextHop)
			}
			if err := snapshotBackend.validate(ctx, []RouteEntry{*conflict}); err != nil {
				result.Skipped++
				result.Failures = append(result.Failures, fmt.Sprintf("conflicting route on %s is no longer safe to replace: %v", saved.InterfaceAlias, err))
				continue
			}
			if err := snapshotBackend.remove(ctx, *conflict); err != nil {
				pending = append(pending, saved)
				result.fail(err.Error())
				continue
			}
			if err := snapshotBackend.restore(ctx, saved, saved.CapturedAt); err != nil {
				pending = append(pending, saved)
				result.fail(err.Error())
				continue
			}
			result.Replaced++
			continue
		}
		if err := snapshotBackend.restore(ctx, saved, saved.CapturedAt); err != nil {
			pending = append(pending, saved)
			result.fail(err.Error())
			continue
		}
		result.Restored++
	}
	task.Routes = pending
	if len(pending) == 0 {
		if err := os.Remove(taskFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
		result.Complete = result.Skipped == 0 && result.Failed == 0
		return result, nil
	}
	if err := saveRouteBackupTask(taskFile, task); err != nil {
		return result, err
	}
	result.Complete = false
	return result, nil
}

func (r *RouteBackupRestoreResult) fail(message string) {
	r.Failed++
	r.Failures = append(r.Failures, message)
}

type parsedRouteBackup struct {
	record  routeBackupRecord
	summary RouteBackupSummary
}

func readRouteBackupRecords() ([]parsedRouteBackup, int, error) {
	f, err := os.Open(historyPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	var records []parsedRouteBackup
	invalid := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var record routeBackupRecord
		if err := json.Unmarshal(line, &record); err != nil || !validRouteBackupRecord(record) {
			invalid++
			continue
		}
		sum := sha256.Sum256(line)
		summary := RouteBackupSummary{ID: hex.EncodeToString(sum[:]), SavedAt: record.SavedAt, RouteCount: len(record.Routes), Interfaces: []string{}}
		interfaces := map[string]bool{}
		for _, r := range record.Routes {
			if r.Destination == "0.0.0.0/0" {
				summary.IPv4Count++
			} else if r.Destination == "::/0" {
				summary.IPv6Count++
			}
			name := r.InterfaceAlias
			if name == "" {
				name = r.InterfaceGUID
			}
			if name != "" {
				interfaces[name] = true
			}
		}
		for name := range interfaces {
			summary.Interfaces = append(summary.Interfaces, name)
		}
		sort.Strings(summary.Interfaces)
		records = append(records, parsedRouteBackup{record: record, summary: summary})
	}
	if err := scanner.Err(); err != nil {
		return nil, invalid, err
	}
	return records, invalid, nil
}

func validRouteBackupRecord(record routeBackupRecord) bool {
	if record.Version != snapshotVersion || record.SavedAt.IsZero() || len(record.Routes) == 0 {
		return false
	}
	for _, route := range record.Routes {
		if (route.Destination != "0.0.0.0/0" && route.Destination != "::/0") || route.InterfaceGUID == "" || route.InterfaceIndex == 0 || (route.Store != "ActiveStore" && route.Store != "PersistentStore" && route.Store != "Linux" && route.Store != "Darwin") {
			return false
		}
	}
	return true
}

func findRouteBackup(id string) (parsedRouteBackup, error) {
	records, _, err := readRouteBackupRecords()
	if err != nil {
		return parsedRouteBackup{}, err
	}
	for _, record := range records {
		if record.summary.ID == id {
			return record, nil
		}
	}
	return parsedRouteBackup{}, errors.New("route backup record not found")
}

func sameRouteTarget(a, b RouteEntry) bool {
	return a.InterfaceGUID == b.InterfaceGUID && a.Network == b.Network && a.CompartmentID == b.CompartmentID && a.Destination == b.Destination && a.Store == b.Store
}

func routeCopy(r *RouteEntry) *RouteEntry { c := *r; return &c }
func routeBackupTaskPath() string         { return GetCacheFilePath() + ".history-restore.json" }
func loadRouteBackupTask(file string) (*routeBackupTask, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var t routeBackupTask
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, err
	}
	return &t, nil
}
func saveRouteBackupTask(file string, task *routeBackupTask) error {
	b, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	return atomicSnapshotWrite(file, b)
}
