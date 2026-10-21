//go:build windows

package iproute

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"netgap/internal/detector"

	"golang.org/x/sys/windows"
)

type windowsRouteBackend struct{}

var getCurrentThreadCompartmentID = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetCurrentThreadCompartmentId")

// Never merge new native records into a legacy backup that this backend cannot
// restore. Refuse before any mutation and preserve the original snapshot bytes.
func (windowsRouteBackend) validateSnapshot(snapshot *routeSnapshot) error {
	defaultCompartment, compartmentErr := currentWindowsCompartmentID()
	if compartmentErr != nil {
		return fmt.Errorf("read current Windows network compartment: %w", compartmentErr)
	}
	for i := range snapshot.Routes {
		row := &snapshot.Routes[i]
		protocol, protocolErr := strconv.ParseUint(row.Protocol, 10, 16)
		publish, publishErr := strconv.ParseUint(row.Publish, 10, 8)
		_, networkErr := windows.GUIDFromString(row.Network)
		if protocolErr != nil || protocol == 0 || publishErr != nil || publish > 2 || networkErr != nil {
			return errors.New("Windows recovery snapshot is incompatible with the native backend; retained unchanged; restore with the original version first")
		}
		// Earlier native snapshots did not persist compartment IDs. The routes
		// were enumerated in the current thread's compartment, so use that value.
		if row.CompartmentID == 0 {
			row.CompartmentID = defaultCompartment
		}
	}
	return nil
}

func currentWindowsCompartmentID() (uint32, error) {
	if err := getCurrentThreadCompartmentID.Find(); err != nil {
		return 0, err
	}
	id, _, callErr := getCurrentThreadCompartmentID.Call()
	if id == 0 {
		if callErr != windows.ERROR_SUCCESS {
			return 0, callErr
		}
		return 0, errors.New("Windows returned compartment ID 0")
	}
	return uint32(id), nil
}

func lockSnapshotFile(file string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(file + ".lock")
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("another process may be changing routes: %w", err)
	}
	return func() { windows.CloseHandle(handle) }, nil
}

func newRouteBackend() routeBackend { return windowsRouteBackend{} }

// Reuse the local MI application/session, avoiding repeated provider startup.
// Route mutations remain serialized by the common snapshot operation lock.
var routeMI struct {
	sync.Mutex
	client *miClient
}

func routeClient(ctx context.Context) (*miClient, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	routeMI.Lock()
	defer routeMI.Unlock()
	if routeMI.client != nil {
		return routeMI.client, nil
	}
	c, err := openMI()
	if err == nil {
		routeMI.client = c
	}
	return c, err
}

func windowsBindings(ctx context.Context) (map[uint32]RouteEntry, error) {
	size := uint32(15000)
	for attempt := 0; attempt < 4; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		buffer := make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, 0, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buffer[0])), &size)
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			continue
		}
		if err != nil {
			return nil, err
		}
		bindings := map[uint32]RouteEntry{}
		for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buffer[0])); a != nil; a = a.Next {
			b := RouteEntry{InterfaceIndex: a.IfIndex, InterfaceAlias: windows.UTF16PtrToString(a.FriendlyName), InterfaceGUID: windows.BytePtrToString(a.AdapterName), Network: a.NetworkGuid.String()}
			for u := a.FirstUnicastAddress; u != nil; u = u.Next {
				b.Addresses = append(b.Addresses, fmt.Sprintf("%s/%d", u.Address.IP(), u.OnLinkPrefixLength))
			}
			sort.Strings(b.Addresses)
			bindings[a.IfIndex] = b
			if a.Ipv6IfIndex != a.IfIndex {
				v6 := b
				v6.InterfaceIndex = a.Ipv6IfIndex
				bindings[a.Ipv6IfIndex] = v6
			}
		}
		return bindings, nil
	}
	return nil, errors.New("adapter table kept changing while capturing bindings")
}

func bindWindowsRoute(row *RouteEntry, bindings map[uint32]RouteEntry) error {
	if row.Store != "ActiveStore" && row.Store != "PersistentStore" {
		return errors.New("unsupported Windows route store")
	}
	if row.Destination != "0.0.0.0/0" && row.Destination != "::/0" {
		return errors.New("saved route is not a default route")
	}
	hop, err := netip.ParseAddr(row.NextHop)
	if err != nil || hop.Is6() != (row.Destination == "::/0") {
		return errors.New("invalid saved default-route next hop")
	}
	if row.Metric > 65535 {
		return errors.New("saved route metric is outside the provider range")
	}
	compartment, err := currentWindowsCompartmentID()
	if err != nil {
		return err
	}
	if row.CompartmentID != 0 && row.CompartmentID != compartment {
		return errors.New("saved route belongs to a different Windows network compartment; refusing cross-compartment mutation")
	}
	for _, b := range bindings {
		if !strings.EqualFold(b.InterfaceGUID, row.InterfaceGUID) {
			continue
		}
		if b.Network != row.Network {
			return fmt.Errorf("%w: adapter network identity changed", detector.ErrStateChanged)
		}
		saved, current := relevantNetworks(row.Addresses, row.Destination, row.NextHop), relevantNetworks(b.Addresses, row.Destination, row.NextHop)
		for network := range saved {
			if current[network] {
				row.InterfaceIndex = b.InterfaceIndex
				row.InterfaceAlias = b.InterfaceAlias
				if row.CompartmentID == 0 {
					row.CompartmentID = compartment
				}
				return nil
			}
		}
		return fmt.Errorf("%w: relevant route subnet changed", detector.ErrStateChanged)
	}
	return fmt.Errorf("%w: saved adapter no longer exists", detector.ErrStateChanged)
}

func readMIRoute(i *miInstance) (RouteEntry, error) {
	var r RouteEntry
	var failures []error
	getString := func(name string) string { v, err := miGetString(i, name); failures = append(failures, err); return v }
	getNumber := func(name string, typ uint32) uint32 {
		v, err := miGetNumber(i, name, typ)
		failures = append(failures, err)
		return v
	}
	r.Destination, r.NextHop = getString("DestinationPrefix"), getString("NextHop")
	r.InterfaceIndex, r.Metric = getNumber("InterfaceIndex", miUint32), getNumber("RouteMetric", miUint16)
	r.CompartmentID = getNumber("CompartmentId", miUint32)
	r.Protocol, r.Publish = fmt.Sprint(getNumber("Protocol", miUint16)), fmt.Sprint(getNumber("Publish", miUint8))
	var err error
	r.ValidTicks, err = miGetTicks(i, "ValidLifetime")
	failures = append(failures, err)
	r.PreferredTicks, err = miGetTicks(i, "PreferredLifetime")
	failures = append(failures, err)
	return r, errors.Join(failures...)
}

const windowsDefaultsQuery = "SELECT * FROM MSFT_NetRoute WHERE DestinationPrefix = '0.0.0.0/0' OR DestinationPrefix = '::/0'"

func (windowsRouteBackend) list(ctx context.Context) ([]RouteEntry, error) {
	c, err := routeClient(ctx)
	if err != nil {
		return nil, err
	}
	bindings, err := windowsBindings(ctx)
	if err != nil {
		return nil, err
	}
	var rows []RouteEntry
	for _, store := range []string{"ActiveStore", "PersistentStore"} {
		err = c.query(ctx, store, windowsDefaultsQuery, func(i *miInstance) error {
			r, err := readMIRoute(i)
			if err != nil {
				return err
			}
			b, ok := bindings[r.InterfaceIndex]
			if !ok {
				return fmt.Errorf("interface %d no longer exists", r.InterfaceIndex)
			}
			r.Store, r.InterfaceAlias, r.InterfaceGUID, r.Network, r.Addresses = store, b.InterfaceAlias, b.InterfaceGUID, b.Network, b.Addresses
			rows = append(rows, r)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("enumerate %s default routes: %w", store, err)
		}
	}
	return rows, ctx.Err()
}

func (windowsRouteBackend) validate(ctx context.Context, rows []RouteEntry) error {
	bindings, err := windowsBindings(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := bindWindowsRoute(&row, bindings); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// Match parsed identities, rather than interpolating stored values into WQL.
// MI owns a result instance until the next pull; mutations consume it in place.
func windowsMatching(ctx context.Context, row RouteEntry, visit func(*miClient, *miInstance, RouteEntry) error) (int, error) {
	c, err := routeClient(ctx)
	if err != nil {
		return 0, err
	}
	bindings, err := windowsBindings(ctx)
	if err != nil {
		return 0, err
	}
	if err := bindWindowsRoute(&row, bindings); err != nil {
		return 0, err
	}
	count := 0
	err = c.query(ctx, row.Store, windowsDefaultsQuery, func(i *miInstance) error {
		current, err := readMIRoute(i)
		if err != nil {
			return err
		}
		if current.InterfaceIndex != row.InterfaceIndex || current.CompartmentID != row.CompartmentID || current.Destination != row.Destination || current.NextHop != row.NextHop {
			return nil
		}
		count++
		if visit != nil {
			return visit(c, i, current)
		}
		return nil
	})
	return count, err
}

func windowsAttributesMatch(a, b RouteEntry) bool {
	return a.Metric == b.Metric && a.Protocol == b.Protocol && a.Publish == b.Publish
}

// Enumerate once per store, rather than querying all default routes again for
// each target. Every mutation still checks the saved target and current binding.
func (windowsRouteBackend) removeBatch(ctx context.Context, rows []RouteEntry) error {
	c, err := routeClient(ctx)
	if err != nil {
		return err
	}
	bindings, err := windowsBindings(ctx)
	if err != nil {
		return err
	}
	key := func(r RouteEntry) string {
		return fmt.Sprintf("%d|%d|%s|%s", r.CompartmentID, r.InterfaceIndex, r.Destination, r.NextHop)
	}
	targets := map[string]map[string]RouteEntry{"PersistentStore": {}, "ActiveStore": {}}
	for _, row := range rows {
		if err := bindWindowsRoute(&row, bindings); err != nil {
			return err
		}
		targets[row.Store][key(row)] = row
	}
	var failures []error
	for _, store := range []string{"PersistentStore", "ActiveStore"} {
		if len(targets[store]) == 0 {
			continue
		}
		err := c.query(ctx, store, windowsDefaultsQuery, func(i *miInstance) error {
			current, err := readMIRoute(i)
			if err != nil {
				return err
			}
			row, ok := targets[store][key(current)]
			if !ok {
				return nil
			}
			if !windowsAttributesMatch(current, row) {
				return fmt.Errorf("%w: route changed before batch removal", detector.ErrStateChanged)
			}
			bindings, err := windowsBindings(ctx)
			if err != nil {
				return err
			}
			if err := bindWindowsRoute(&row, bindings); err != nil {
				return err
			}
			if err := c.delete(ctx, store, i); err != nil {
				failures = append(failures, fmt.Errorf("remove %s default route on interface %d: %w", store, row.InterfaceIndex, err))
			}
			return ctx.Err()
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("batch removal in %s: %w", store, err))
			break
		}
		if ctx.Err() != nil {
			break
		}
	}
	return errors.Join(failures...)
}

func (windowsRouteBackend) remove(ctx context.Context, row RouteEntry) error {
	_, err := windowsMatching(ctx, row, func(c *miClient, i *miInstance, current RouteEntry) error {
		if !windowsAttributesMatch(current, row) {
			return fmt.Errorf("%w: route changed before removal", detector.ErrStateChanged)
		}
		bindings, err := windowsBindings(ctx)
		if err != nil {
			return err
		}
		if err := bindWindowsRoute(&row, bindings); err != nil {
			return err
		}
		return c.delete(ctx, row.Store, i)
	})
	return err
}

// Journal the active side effects of persistent creation before any mutation.
func (b windowsRouteBackend) prepareRestore(ctx context.Context, snapshot *routeSnapshot) error {
	current, err := b.list(ctx)
	if err != nil {
		return err
	}
	for i := 0; i < len(snapshot.Routes); i++ {
		persistent := snapshot.Routes[i]
		if persistent.Store != "PersistentStore" {
			continue
		}
		exists := false
		for _, row := range current {
			if sameRouteIdentity(row, persistent) {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		active := persistent
		active.Store = "ActiveStore"
		active.CreatedByRecovery, active.ActiveBefore = nil, nil
		var before *RouteEntry
		for _, row := range current {
			if sameRouteIdentity(row, active) {
				copyRow := row
				before = &copyRow
				break
			}
		}
		index := -1
		for j, row := range snapshot.Routes {
			if sameRouteIdentity(row, active) {
				index = j
				break
			}
		}
		if before != nil && (index < 0 || (!sameRoute(*before, snapshot.Routes[index]) && !sameRoute(*before, active))) {
			return errors.New("active route conflicts with persistent recovery; no route overwritten")
		}
		snapshot.Routes[i].ActiveBefore = before
		if index >= 0 {
			if snapshot.Routes[index].CreatedByRecovery == nil {
				snapshot.Routes[index].CreatedByRecovery = &active
			}
		} else {
			cleanup := active
			cleanup.CreatedByRecovery = &active
			cleanup.RemoveAfterRestore = true
			snapshot.Routes = append(snapshot.Routes, cleanup)
		}
	}
	return nil
}

func routeLifetimeParam(name, ticks string, saved time.Time) (miParameter, error) {
	return routeLifetimeParamAt(name, ticks, saved, time.Now())
}

func routeLifetimeParamAt(name, ticks string, saved, now time.Time) (miParameter, error) {
	value, err := strconv.ParseUint(ticks, 10, 64)
	if err != nil {
		return miParameter{}, err
	}
	const infinite = uint64(42949672950000000)
	if value >= infinite {
		value = infinite
	} else {
		elapsed := now.Sub(saved) / 100
		if elapsed < 0 {
			elapsed = 0
		}
		if uint64(elapsed) >= value {
			value = 0
		} else {
			value -= uint64(elapsed)
		}
	}
	// NetTCPIP accepts whole-second route lifetimes. Subtracting elapsed
	// time in ticks otherwise produces a fractional CIM interval rejected by
	// the provider. Round down so recovery never extends a saved lifetime.
	seconds := value / 10000000
	if seconds == 0 && name == "ValidLifetime" {
		return miParameter{}, errors.New("saved route lifetime expired; reconnect to reacquire a route")
	}
	p := miParameter{name: name, typ: miDatetime}
	parts := (*[9]uint32)(unsafe.Pointer(&p.value))
	parts[1], parts[2], parts[3], parts[4] = uint32(seconds/86400), uint32(seconds%86400/3600), uint32(seconds%3600/60), uint32(seconds%60)
	return p, nil
}

func windowsCreateParameters(row RouteEntry, saved time.Time) ([]miParameter, error) {
	protocol, err := strconv.ParseUint(row.Protocol, 10, 16)
	if err != nil || protocol == 0 {
		return nil, errors.New("invalid saved route protocol")
	}
	publish, err := strconv.ParseUint(row.Publish, 10, 8)
	if err != nil || publish > 2 {
		return nil, errors.New("invalid saved route publish mode")
	}
	now := time.Now()
	valid, err := routeLifetimeParamAt("ValidLifetime", row.ValidTicks, saved, now)
	if err != nil {
		return nil, err
	}
	preferred, err := routeLifetimeParamAt("PreferredLifetime", row.PreferredTicks, saved, now)
	if err != nil {
		return nil, err
	}
	family := uint32(windows.AF_INET)
	if row.Destination == "::/0" {
		family = windows.AF_INET6
	}
	// Follow the system CDXML ByInterfaceIndex set: alias is an alternative
	// selector. Omit optional inputs instead of supplying empty defaults.
	params := []miParameter{
		miNumberParam("AddressFamily", miUint16, family),
		miStringParam("DestinationPrefix", row.Destination),
		miNumberParam("InterfaceIndex", miUint32, row.InterfaceIndex),
		miStringParam("NextHop", row.NextHop), valid, preferred,
		miNumberParam("Protocol", miUint16, uint32(protocol)),
		miNumberParam("Publish", miUint8, uint32(publish)),
		miNumberParam("RouteMetric", miUint16, row.Metric),
	}
	switch row.Store {
	case "ActiveStore":
		params = append(params, miStringParam("PolicyStore", "ActiveStore"))
	case "PersistentStore": // Default creation updates both stores; journaled by prepareRestore.
	default:
		return nil, errors.New("unsupported Windows route store")
	}
	return params, nil
}

func (b windowsRouteBackend) restore(ctx context.Context, row RouteEntry, saved time.Time) error {
	var existing []RouteEntry
	_, err := windowsMatching(ctx, row, func(_ *miClient, _ *miInstance, r RouteEntry) error { existing = append(existing, r); return nil })
	if err != nil {
		return err
	}
	if row.RemoveAfterRestore {
		if row.CreatedByRecovery == nil {
			return errors.New("generated route cleanup has no journal")
		}
		for _, r := range existing {
			if !windowsAttributesMatch(r, *row.CreatedByRecovery) {
				return errors.New("generated active route changed; refusing cleanup")
			}
		}
		cleanup := row
		cleanup.Metric, cleanup.Protocol, cleanup.Publish = row.CreatedByRecovery.Metric, row.CreatedByRecovery.Protocol, row.CreatedByRecovery.Publish
		if err := b.remove(ctx, cleanup); err != nil {
			return err
		}
		count, err := windowsMatching(ctx, row, nil)
		if err != nil {
			return err
		}
		if count != 0 {
			return errors.New("generated route cleanup was not confirmed")
		}
		return nil
	}
	if len(existing) > 0 {
		allMatch := true
		for _, r := range existing {
			allMatch = allMatch && windowsAttributesMatch(r, row)
		}
		if allMatch {
			return nil
		}
		if row.Store != "ActiveStore" || row.CreatedByRecovery == nil {
			return errors.New("existing route conflicts with recovery snapshot")
		}
		for _, r := range existing {
			if !windowsAttributesMatch(r, *row.CreatedByRecovery) {
				return errors.New("generated active route changed; refusing replacement")
			}
		}
		generated := row
		generated.Metric, generated.Protocol, generated.Publish = row.CreatedByRecovery.Metric, row.CreatedByRecovery.Protocol, row.CreatedByRecovery.Publish
		if err := b.remove(ctx, generated); err != nil {
			return err
		}
	}
	bindings, err := windowsBindings(ctx)
	if err != nil {
		return err
	}
	if err := bindWindowsRoute(&row, bindings); err != nil {
		return err
	}
	params, err := windowsCreateParameters(row, saved)
	if err != nil {
		return err
	}
	if row.Store == "PersistentStore" {
		active := row
		active.Store = "ActiveStore"
		_, err = windowsMatching(ctx, active, func(_ *miClient, _ *miInstance, r RouteEntry) error {
			if !windowsAttributesMatch(r, row) && (row.ActiveBefore == nil || !windowsAttributesMatch(r, *row.ActiveBefore)) {
				return errors.New("active route changed before persistent creation")
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	c, err := routeClient(ctx)
	if err != nil {
		return err
	}
	createErr := c.create(ctx, params)
	if createErr != nil {
		createErr = fmt.Errorf("create default route %s via %s on interface %s (%d), compartment %d, store %s: %w", row.Destination, row.NextHop, row.InterfaceAlias, row.InterfaceIndex, row.CompartmentID, row.Store, createErr)
	}
	// Persistent creation may report an active duplicate after persisting it.
	// Only exact attributes in the requested store can confirm restoration.
	count, verifyErr := windowsMatching(ctx, row, func(_ *miClient, _ *miInstance, r RouteEntry) error {
		if !windowsAttributesMatch(r, row) {
			return errors.New("restored route attributes differ from recovery snapshot")
		}
		return nil
	})
	if verifyErr != nil || count == 0 {
		return errors.Join(createErr, verifyErr, errors.New("route restoration was not confirmed"))
	}
	return ctx.Err()
}
