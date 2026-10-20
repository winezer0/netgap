package iproute

import "net/netip"

func relevantNetworks(addresses []string, destination, hop string) map[netip.Prefix]bool {
	all, onLink := map[netip.Prefix]bool{}, map[netip.Prefix]bool{}
	gateway, _ := netip.ParseAddr(hop)
	for _, address := range addresses {
		p, err := netip.ParsePrefix(address)
		if err != nil || p.Addr().Is6() != (destination == "::/0") {
			continue
		}
		p = p.Masked()
		all[p] = true
		if p.Contains(gateway) {
			onLink[p] = true
		}
	}
	if len(onLink) > 0 {
		return onLink
	}
	return all
}
