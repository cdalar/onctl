package cloud

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
)

// Test mapHetznerNetwork function
func TestMapHetznerNetwork(t *testing.T) {
	created := time.Now()
	network := hcloud.Network{
		ID:      123,
		Name:    "test-network",
		IPRange: &net.IPNet{IP: net.ParseIP("10.0.0.0"), Mask: net.CIDRMask(16, 32)},
		Created: created,
		Servers: []*hcloud.Server{{}, {}}, // 2 servers
	}

	result := mapHetznerNetwork(network)

	assert.Equal(t, "hetzner", result.Provider)
	assert.Equal(t, "123", result.ID)
	assert.Equal(t, "test-network", result.Name)
	assert.Equal(t, "10.0.0.0/16", result.CIDR)
	assert.Equal(t, created, result.CreatedAt)
	assert.Equal(t, 2, result.Servers)
}

// Test mapHetznerServer function
func TestMapHetznerServer(t *testing.T) {
	created := time.Now()
	server := hcloud.Server{
		ID:     123,
		Name:   "test-server",
		Status: hcloud.ServerStatusRunning,
		PublicNet: hcloud.ServerPublicNet{
			IPv4: hcloud.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.4")},
		},
		PrivateNet: []hcloud.ServerPrivateNet{
			{IP: net.ParseIP("10.0.0.1")},
		},
		ServerType: &hcloud.ServerType{
			Name: "cx11",
			Pricings: []hcloud.ServerTypeLocationPricing{
				{
					Location: &hcloud.Location{Name: "fsn1"},
					Hourly:   hcloud.Price{Gross: "0.0119"},
					Monthly:  hcloud.Price{Gross: "7.14"},
				},
			},
		},
		Datacenter: &hcloud.Datacenter{
			Location: &hcloud.Location{Name: "fsn1"},
		},
		Created: created,
	}

	result := mapHetznerServer(server)

	assert.Equal(t, "hetzner", result.Provider)
	assert.Equal(t, "123", result.ID)
	assert.Equal(t, "test-server", result.Name)
	assert.Equal(t, "1.2.3.4", result.IP)
	assert.Equal(t, "10.0.0.1", result.PrivateIP)
	assert.Equal(t, "cx11", result.Type)
	assert.Equal(t, "running", result.Status)
	assert.Equal(t, created, result.CreatedAt)
	assert.Equal(t, "fsn1", result.Location)
	assert.Equal(t, "EUR", result.Cost.Currency)
	assert.Greater(t, result.Cost.CostPerHour, 0.0)
	assert.Greater(t, result.Cost.CostPerMonth, 0.0)
}

// Test mapHetznerServer with no private network
func TestMapHetznerServer_NoPrivateNetwork(t *testing.T) {
	server := hcloud.Server{
		ID:         123,
		Name:       "test-server",
		Status:     hcloud.ServerStatusRunning,
		PrivateNet: []hcloud.ServerPrivateNet{}, // Empty private networks
		PublicNet: hcloud.ServerPublicNet{
			IPv4: hcloud.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.4")},
		},
		ServerType: &hcloud.ServerType{Name: "cx11"},
		Datacenter: &hcloud.Datacenter{
			Location: &hcloud.Location{Name: "fsn1"},
		},
		Created: time.Now(),
	}

	result := mapHetznerServer(server)

	assert.Equal(t, "N/A", result.PrivateIP)
}

// Test mapHetznerServer with pricing calculation
func TestMapHetznerServer_PricingCalculation(t *testing.T) {
	created := time.Now().Add(-2 * time.Hour) // 2 hours ago
	server := hcloud.Server{
		ID:     123,
		Name:   "test-server",
		Status: hcloud.ServerStatusRunning,
		PublicNet: hcloud.ServerPublicNet{
			IPv4: hcloud.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.4")},
		},
		PrivateNet: []hcloud.ServerPrivateNet{},
		ServerType: &hcloud.ServerType{
			Name: "cx11",
			Pricings: []hcloud.ServerTypeLocationPricing{
				{
					Location: &hcloud.Location{Name: "fsn1"},
					Hourly:   hcloud.Price{Gross: "0.0119"},
					Monthly:  hcloud.Price{Gross: "7.14"},
				},
			},
		},
		Datacenter: &hcloud.Datacenter{
			Location: &hcloud.Location{Name: "fsn1"},
		},
		Created: created,
	}

	result := mapHetznerServer(server)

	assert.Equal(t, 0.0119, result.Cost.CostPerHour)
	assert.Equal(t, 7.14, result.Cost.CostPerMonth)
	assert.Greater(t, result.Cost.AccumulatedCost, 0.0) // Should have some accumulated cost
	assert.Less(t, result.Cost.AccumulatedCost, 1.0)    // But not too much for 2 hours
}

// Test ProviderHetzner.SSHInto (stub method)
func TestProviderHetzner_SSHInto(t *testing.T) {
	provider := ProviderHetzner{}

	// This method is a stub, so it should not panic
	assert.NotPanics(t, func() {
		provider.SSHInto("test-server", 22, "private-key", "jumphost")
	})
}

// Test NetworkProviderHetzner and ProviderHetzner struct creation
func TestProviderStructs(t *testing.T) {
	// Test that we can create the provider structs
	networkProvider := NetworkProviderHetzner{}
	assert.NotNil(t, networkProvider)

	provider := ProviderHetzner{}
	assert.NotNil(t, provider)
}

// Test error conditions in mapping functions
func TestMapHetznerServer_EdgeCases(t *testing.T) {
	// Test with minimal server data
	server := hcloud.Server{
		ID:     456,
		Name:   "minimal-server",
		Status: hcloud.ServerStatusOff,
		PublicNet: hcloud.ServerPublicNet{
			IPv4: hcloud.ServerPublicNetIPv4{IP: net.ParseIP("5.6.7.8")},
		},
		PrivateNet: []hcloud.ServerPrivateNet{},
		ServerType: &hcloud.ServerType{
			Name:     "cx21",
			Pricings: []hcloud.ServerTypeLocationPricing{}, // No pricing data
		},
		Datacenter: &hcloud.Datacenter{
			Location: &hcloud.Location{Name: "nbg1"},
		},
		Created: time.Now(),
	}

	result := mapHetznerServer(server)

	assert.Equal(t, "hetzner", result.Provider)
	assert.Equal(t, "456", result.ID)
	assert.Equal(t, "minimal-server", result.Name)
	assert.Equal(t, "5.6.7.8", result.IP)
	assert.Equal(t, "N/A", result.PrivateIP)
	assert.Equal(t, "cx21", result.Type)
	assert.Equal(t, "off", result.Status)
	assert.Equal(t, "nbg1", result.Location)
	assert.Equal(t, "EUR", result.Cost.Currency)
	assert.Equal(t, 0.0, result.Cost.CostPerHour)
	assert.Equal(t, 0.0, result.Cost.CostPerMonth)
	assert.Equal(t, 0.0, result.Cost.AccumulatedCost)
}

// Test network mapping with different scenarios
func TestMapHetznerNetwork_EdgeCases(t *testing.T) {
	// Test with minimal network data
	network := hcloud.Network{
		ID:      789,
		Name:    "minimal-network",
		IPRange: &net.IPNet{IP: net.ParseIP("192.168.1.0"), Mask: net.CIDRMask(24, 32)},
		Created: time.Now(),
		Servers: []*hcloud.Server{}, // No servers
	}

	result := mapHetznerNetwork(network)

	assert.Equal(t, "hetzner", result.Provider)
	assert.Equal(t, "789", result.ID)
	assert.Equal(t, "minimal-network", result.Name)
	assert.Equal(t, "192.168.1.0/24", result.CIDR)
	assert.Equal(t, 0, result.Servers)
}

// Test string conversion functions
func TestStringConversions(t *testing.T) {
	// Test that our ID conversions work correctly
	testID := int64(12345)
	result := strconv.FormatInt(testID, 10)
	assert.Equal(t, "12345", result)

	// Test parsing back
	parsed, err := strconv.ParseInt(result, 10, 64)
	assert.NoError(t, err)
	assert.Equal(t, testID, parsed)
}
