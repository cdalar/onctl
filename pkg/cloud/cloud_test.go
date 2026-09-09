package cloud

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVmString(t *testing.T) {
	v := Vm{
		ID:       "123",
		Name:     "test-vm",
		IP:       "1.2.3.4",
		Type:     "t2.micro",
		Status:   "running",
		Location: "us-east-1",
		Provider: "aws",
	}
	s := v.String()
	assert.Contains(t, s, "ID")
	assert.Contains(t, s, "123")
	assert.Contains(t, s, "Name")
	assert.Contains(t, s, "test-vm")
	assert.Contains(t, s, "Provider")
	assert.Contains(t, s, "aws")
}

func TestVmString_Empty(t *testing.T) {
	v := Vm{}
	s := v.String()
	assert.NotEmpty(t, s)
	assert.True(t, strings.HasPrefix(s, "\n"))
}

func TestVmFields(t *testing.T) {
	now := time.Now()
	v := Vm{
		ID:            "i-abc123",
		Name:          "my-vm",
		IP:            "10.0.0.1",
		PrivateIP:     "192.168.1.1",
		Type:          "m5.large",
		Status:        "running",
		Location:      "eu-west-1",
		SSHKeyID:      "key-001",
		SSHPort:       22,
		CloudInitFile: "/tmp/init.sh",
		CreatedAt:     now,
		Provider:      "aws",
		Cost: CostStruct{
			Currency:        "USD",
			CostPerHour:     0.096,
			CostPerMonth:    69.12,
			AccumulatedCost: 1.5,
		},
	}
	assert.Equal(t, "i-abc123", v.ID)
	assert.Equal(t, "my-vm", v.Name)
	assert.Equal(t, "10.0.0.1", v.IP)
	assert.Equal(t, "192.168.1.1", v.PrivateIP)
	assert.Equal(t, 22, v.SSHPort)
	assert.Equal(t, "USD", v.Cost.Currency)
	assert.InDelta(t, 0.096, v.Cost.CostPerHour, 0.0001)
}

func TestVmList(t *testing.T) {
	vl := VmList{
		List: []Vm{
			{ID: "1", Name: "vm1"},
			{ID: "2", Name: "vm2"},
		},
	}
	assert.Len(t, vl.List, 2)
	assert.Equal(t, "1", vl.List[0].ID)
	assert.Equal(t, "vm2", vl.List[1].Name)
}

func TestVmList_Empty(t *testing.T) {
	vl := VmList{}
	assert.Empty(t, vl.List)
}

func TestCostStruct(t *testing.T) {
	c := CostStruct{
		Currency:        "EUR",
		CostPerHour:     0.05,
		CostPerMonth:    36.0,
		AccumulatedCost: 2.5,
	}
	assert.Equal(t, "EUR", c.Currency)
	assert.InDelta(t, 0.05, c.CostPerHour, 0.0001)
	assert.InDelta(t, 36.0, c.CostPerMonth, 0.0001)
	assert.InDelta(t, 2.5, c.AccumulatedCost, 0.0001)
}

func TestProbeTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	assert.True(t, ProbeTCP(ln.Addr().String(), time.Second), "a real listener must probe reachable")

	// 10.255.255.1 is a non-routable address reserved for exactly this
	// (blackholes rather than fast-refusing, so this also exercises the
	// timeout path, not just "connection refused").
	assert.False(t, ProbeTCP("10.255.255.1:22", 100*time.Millisecond))
}

func TestProbeSSHReady(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	var port int
	_, err = fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)

	candidates := []Vm{
		{IP: "127.0.0.1", SSHPort: port},  // reachable
		{IP: "10.255.255.1", SSHPort: 22}, // unreachable (probeTimeout bounds this)
		{IP: ""},                          // not a real candidate -- must be skipped, not probed
	}
	ready := ProbeSSHReady(candidates)
	assert.True(t, ready[0], "index 0 (real listener) must be reported ready")
	assert.False(t, ready[1], "index 1 (unreachable) must not be reported ready")
	assert.False(t, ready[2], "index 2 (no IP) must not be reported ready")
}
