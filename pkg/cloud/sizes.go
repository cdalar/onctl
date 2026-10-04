package cloud

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

var (
	_ SizeLister = ProviderHetzner{}
	_ SizeLister = ProviderOvh{}
)

// ListSizes lists Hetzner's server types available in the configured
// location, with that location's prices. Deprecated types, and types not
// offered there, are left out.
func (p ProviderHetzner) ListSizes() ([]CloudSize, error) {
	types, err := p.Client.ServerType.All(context.TODO())
	if err != nil {
		return nil, err
	}
	var out []CloudSize
	for _, t := range types {
		if t.IsDeprecated() || !hetznerOfferedIn(t, p.Config.Location) {
			continue
		}
		size := CloudSize{
			Name:      t.Name,
			VCPU:      t.Cores,
			MemoryMiB: int(t.Memory * 1024),
			DiskGiB:   t.Disk,
			Arch:      string(t.Architecture),
			Default:   t.Name == p.Config.VMType,
		}
		for _, pr := range t.Pricings {
			if pr.Location != nil && pr.Location.Name == p.Config.Location {
				size.HourlyPrice = formatPrice(pr.Hourly.Gross, hetznerCurrency(pr.Hourly.Currency), 4)
				size.MonthlyPrice = formatPrice(pr.Monthly.Gross, hetznerCurrency(pr.Monthly.Currency), 2)
			}
		}
		out = append(out, size)
	}
	sortSizes(out)
	return out, nil
}

// hetznerOfferedIn reports whether t can be created in location: listed
// there as available and not deprecated. A type that lists no locations
// (an older API response) counts as offered everywhere; so does every
// type when no location is configured.
func hetznerOfferedIn(t *hcloud.ServerType, location string) bool {
	if location == "" || len(t.Locations) == 0 {
		return true
	}
	for _, l := range t.Locations {
		if l.Location != nil && l.Location.Name == location {
			return l.Available && !l.IsDeprecated()
		}
	}
	return false
}

// hetznerCurrency is the currency Hetzner prices a server type in. Its
// API carries the currency only on /pricing, not on server types, and
// Hetzner bills in euros (the same assumption as List's cost column).
func hetznerCurrency(c string) string {
	if c == "" {
		return "EUR"
	}
	return c
}

// formatPrice turns an API amount ("0.0080000000") into "0.0080 EUR".
func formatPrice(amount, currency string, decimals int) string {
	f, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return ""
	}
	if currency == "" {
		return fmt.Sprintf("%.*f", decimals, f)
	}
	return fmt.Sprintf("%.*f %s", decimals, f, currency)
}

// ListSizes lists OVH's flavors available in the configured region. The
// API publishes no prices.
func (p ProviderOvh) ListSizes() ([]CloudSize, error) {
	var flavors []ovhFlavor
	if err := p.Client.Get(p.projectPath("/flavor"), &flavors); err != nil {
		return nil, fmt.Errorf("listing flavors: %w", err)
	}
	var out []CloudSize
	for _, f := range flavors {
		if f.Region != p.Config.Region || !f.Available {
			continue
		}
		out = append(out, CloudSize{
			Name:      f.Name,
			VCPU:      f.VCPUs,
			MemoryMiB: f.RAM, // OVH reports RAM in MB
			DiskGiB:   f.Disk,
			Default:   f.Name == p.Config.VMType,
		})
	}
	sortSizes(out)
	return out, nil
}

// sortSizes orders sizes smallest first: by architecture, then vCPUs,
// memory and name.
func sortSizes(sizes []CloudSize) {
	sort.SliceStable(sizes, func(i, j int) bool {
		a, b := sizes[i], sizes[j]
		if a.Arch != b.Arch {
			return a.Arch < b.Arch
		}
		if a.VCPU != b.VCPU {
			return a.VCPU < b.VCPU
		}
		if a.MemoryMiB != b.MemoryMiB {
			return a.MemoryMiB < b.MemoryMiB
		}
		return a.Name < b.Name
	})
}
